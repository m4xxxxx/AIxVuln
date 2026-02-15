package agents

import (
	"AIxVuln/llm"
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"AIxVuln/toolCalling"
	"context"
	"fmt"

	"github.com/google/uuid"
)

type ProjectOverviewAgent struct {
	memory       llm.Memory
	client       *toolCalling.ToolManager
	task         *taskManager.Task
	id           string
	state        string
	stateHandler func(string)
	profile      AgentProfile
	taskChan     chan TaskAssignment
}

func (c *ProjectOverviewAgent) GetTask() *taskManager.Task {
	return c.task
}
func (c *ProjectOverviewAgent) GetMemory() llm.Memory {
	return c.memory
}
func (c *ProjectOverviewAgent) GetId() string {
	return c.id
}
func (c *ProjectOverviewAgent) SetId(id string) {
	c.id = id
}
func (c *ProjectOverviewAgent) GetState() string {
	return c.state
}
func (c *ProjectOverviewAgent) GetProfile() AgentProfile {
	return c.profile
}
func (c *ProjectOverviewAgent) SetProfile(p AgentProfile) {
	c.profile = p
}
func (c *ProjectOverviewAgent) SetState(state string) {
	c.state = state
	if c.stateHandler != nil {
		c.stateHandler(state)
	}
}
func (c *ProjectOverviewAgent) SetMemory(m llm.Memory) {
	c.memory = m
}
func (c *ProjectOverviewAgent) SetKeyMessage(k map[string][]interface{}) {
	c.memory.SetKeyMessage(k, c.task.GetTaskId())
}
func (c *ProjectOverviewAgent) SetEnvInfo(k map[string]interface{}) {
	c.task.SetEnvInfo(k)
}
func (c *ProjectOverviewAgent) Name() string {
	return "ProjectOverviewAgent"
}
func (c *ProjectOverviewAgent) SetStateHandler(f func(string)) {
	c.stateHandler = f
}

func (c *ProjectOverviewAgent) AssignTask(assignment TaskAssignment) {
	c.taskChan <- assignment
}

func NewProjectOverviewAgent(task *taskManager.Task, argsJson string) (Agent, error) {
	task.SetAgentName("ProjectOverviewAgent")
	systemPrompt := `You are a Project Overview Analyst. Your job is to quickly scan a source code project and produce a concise, structured overview.

You MUST complete the following steps:
1. Use DetectLanguageTool to identify the programming language(s).
2. Use ListSourceCodeTreeTool to get the project directory structure.
3. Based on the directory structure, use ReadLinesFromFileTool to read key configuration files (e.g. composer.json, package.json, pom.xml, build.gradle, requirements.txt, go.mod, Gemfile, Cargo.toml, etc.) and key entry-point files to identify the framework and tech stack.
4. Produce a final summary in the following structured format (in Chinese):

## 项目概览
- **编程语言**: (e.g. PHP, Java , Python , Go )
- **Web框架**: (e.g. Laravel 8, Spring Boot 2.7, Django 4.2, Gin 1.9)
- **数据库**: (e.g. MySQL, PostgreSQL, SQLite, Redis — inferred from config files or dependencies)
- **其他依赖/技术栈**: (e.g. Composer, npm, Maven, Docker, Redis, RabbitMQ, etc.)
- **项目结构特征**: (e.g. MVC pattern, microservices, monolith, API-only, etc.)
- **入口文件**: (e.g. index.php, Application.java, main.py, main.go)
- **备注**: (any other notable observations, e.g. uses ORM, has migration files, has test suite, etc.)

Keep the overview concise (under 500 characters for each field). Do NOT perform any security analysis — only identify the tech stack.
When you are done, call **AgentFinishTool** with the structured overview as the summary parameter to formally complete your task.
**Token Efficiency**: Do NOT output any free-form text, thinking process, or commentary. Your response must contain ONLY tool calls. Reason silently and express results through tool calls only.`

	memory := llm.NewContextManager("overview")
	memory.SetEventHandler(task.GetEventHandler())
	task.SetMemory(memory)
	tl := []map[string]string{{"TaskContent": "Scan the project source code and produce a structured overview."}}
	task.SetTaskList(tl)
	b := BuildAgentWithMemory(task, memory, systemPrompt, OverviewToolFactories())
	agent := ProjectOverviewAgent{memory: b.Memory, client: b.Client, task: task, state: "Not Running", taskChan: make(chan TaskAssignment, 1)}
	agent.SetId(agent.Name() + "-" + uuid.New().String())
	return &agent, nil
}

func (c *ProjectOverviewAgent) StartTask(ctx context.Context) *StartResp {
	for {
		var assignment TaskAssignment
		select {
		case <-ctx.Done():
			return &StartResp{Err: ctx.Err()}
		case assignment = <-c.taskChan:
		}
		resp := c.executeTask(ctx, assignment)
		if assignment.DoneCb != nil {
			assignment.DoneCb(resp)
		}
		if ctx.Err() != nil {
			return resp
		}
	}
}

func (c *ProjectOverviewAgent) executeTask(ctx context.Context, assignment TaskAssignment) *StartResp {
	taskContent := "Scan the project source code and produce a structured overview."
	c.memory.AddMessage(&llm.MessageX{
		Msg:       llm.Message{Role: llm.RoleUser, Content: "[New Task Assigned]\n" + taskContent},
		Shared:    false,
		ContextId: c.task.GetTaskId(),
	})

	c.SetState("Running")
	defer func() { c.SetState("Done") }()
	c.client.RemoveTool("TaskListTool")
	var summary string
	model := misc.GetConfigValueDefault("overview", "MODEL", misc.GetConfigValueRequired("main_setting", "MODEL"))
	for {
		select {
		case <-ctx.Done():
			return &StartResp{Err: ctx.Err()}
		default:
		}
		msgList := c.memory.GetContext(c.task.GetTaskId())
		if msgList == nil {
			return &StartResp{Err: fmt.Errorf("agent task not set")}
		}
		assistantMessage, toolMessage, err := c.client.ToolCallRequestWithLabel(ctx, llm.GetResponsesClient("overview", "main_setting"), msgList, model, c.Name(), "ProjectOverview", c.task.GetProjectName())
		if err != nil {
			return &StartResp{Err: err}
		}
		misc.Debug("[ProjectOverview] 响应: %s", assistantMessage.Content)
		msg := &llm.MessageX{Msg: assistantMessage, Shared: false, ContextId: c.task.GetTaskId()}
		c.memory.AddMessage(msg)
		if len(toolMessage) > 0 {
			if s, ok := extractAgentFinishSummary(toolMessage); ok {
				summary = s
				for _, message := range toolMessage {
					msgTool := &llm.MessageX{Msg: message, Shared: false, ContextId: c.task.GetTaskId()}
					c.memory.AddMessage(msgTool)
				}
				break
			}
			for _, message := range toolMessage {
				msgTool := &llm.MessageX{Msg: message, Shared: false, ContextId: c.task.GetTaskId()}
				c.memory.AddMessage(msgTool)
			}
		} else {
			misc.Debug("[ProjectOverview] 空响应，发送提醒继续")
			c.memory.AddMessage(&llm.MessageX{Msg: llm.Message{Role: llm.RoleUser, Content: "Please continue your task. If you have finished, call AgentFinishTool with a summary."}, Shared: false, ContextId: c.task.GetTaskId()})
			continue
		}
	}
	return &StartResp{Err: nil, Memory: c.memory, Summary: summary}
}
