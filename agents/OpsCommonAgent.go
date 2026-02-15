package agents

import (
	"AIxVuln/llm"
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"AIxVuln/toolCalling"
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type OpsCommonAgent struct {
	memory       llm.Memory
	client       *toolCalling.ToolManager
	task         *taskManager.Task
	id           string
	state        string
	stateHandler func(string)
	profile      AgentProfile
	taskChan     chan TaskAssignment
}
type OpsCommonAgentConfig struct {
	TaskContent string `json:"task_content"`
}

func (c *OpsCommonAgent) GetTask() *taskManager.Task {
	return c.task
}

func (c *OpsCommonAgent) GetMemory() llm.Memory {
	return c.memory
}
func (c *OpsCommonAgent) GetId() string {
	return c.id
}
func (c *OpsCommonAgent) SetId(id string) {
	c.id = id
}
func (c *OpsCommonAgent) GetState() string {
	return c.state
}

func (c *OpsCommonAgent) GetProfile() AgentProfile {
	return c.profile
}

func (c *OpsCommonAgent) SetProfile(p AgentProfile) {
	c.profile = p
}
func (c *OpsCommonAgent) SetState(state string) {
	c.state = state
	if c.stateHandler != nil {
		c.stateHandler(state)
	}
}
func (c *OpsCommonAgent) SetMemory(m llm.Memory) {
	c.memory = m
}
func (c *OpsCommonAgent) SetKeyMessage(k map[string][]interface{}) {
	c.memory.SetKeyMessage(k, c.task.GetTaskId())
}
func (c *OpsCommonAgent) SetEnvInfo(k map[string]interface{}) {
	c.task.SetEnvInfo(k)
}

func (c *OpsCommonAgent) AssignTask(assignment TaskAssignment) {
	c.taskChan <- assignment
}

func NewOpsCommonAgent(task *taskManager.Task, argsJson string) (Agent, error) {
	task.SetAgentName("OpsCommonAgent")
	systemPrompt := `You are an operations AI assistant responsible for utilizing various tools to manage the testing environment, specifically focusing on setting up and maintaining the target environment. For setup tasks, the specific requirements are as follows:
If there are middleware configuration files, such as .user.ini, .htaccess, etc., the relevant functionality must be enabled to ensure support for these configurations.
Direct execution of Docker commands is prohibited. All operations must be performed through DockerxxxTool. All containers share the '/sourceCodeDir' directory. To transfer files between containers, simply copy the files to the '/sourceCodeDir' directory.
When setting up a PHP system, first start the web service and try accessing it to confirm whether database, administrator username, password, and other configurations can be completed directly via the installation guide on the webpage. If there is no installation guide, manually configure this information.
For tools that require the webPort parameter, you must first confirm which port the web service is running on before passing it in. The PHP environment uses Apache2, which defaults to port 80. You may not use this default port, but after passing the specified webPort, you must configure it to the port number designated by webPort.
When encountering compatibility issues where the environment version does not support the project, do not attempt to modify the source code. Instead, promptly switch to a compatible environment version. For example, if PHP7 is started but the project contains syntax not supported by PHP7, decisively launch a new environment with the specified required version. When you start a new container due to version switching, if the old container is no longer in use, you need to call DockerRemoveTool to delete the old container.
Do not insert any data into MySQL for initialization unless you have repeatedly attempted and confirmed that the system installation cannot proceed via the web interface.
If you need to execute commands in the environment started by RunxxxEnvTool, you must use DockerExecTool instead of RunCommandTool. Although their '/sourceCodeDir' directories are mutually mapped, they do not run in the same container.
When calling EnvSaveTool, all URL and IP information must not use 127.0.0.1, localhost, etc., but rather the real WEB environment container IP.
**Setup Task Guidelines:**
1. Install the source code project: First, start the environment using the RunxxxEnvTool as the preferred method, ensuring the web environment is set up within a tool that supports the designated webPort. Then, visit the project page to verify whether the service port matches the webPort specified when starting the environment (if applicable). If they do not match, modify the service port accordingly. If an installation guide page is provided, follow the instructions to complete installation and initialization. If no guide page is available, analyze the installation process independently. You can set up an account and password as needed.
2. If the login process in the environment includes a CAPTCHA, modify the source code to disable the CAPTCHA login feature.
3. Analyze the routing access method and provide three successfully accessed routing examples.
4. Log into the system and obtain a valid COOKIE.
5. Call EnvSaveTool to save key information. When calling EnvSaveTool, all URL and IP information must not use 127.0.0.1, localhost, etc., but rather the real WEB environment container IP.
For other operations and maintenance tasks, simply complete them according to the task content.` + CommonSystemPrompt()
	var memory llm.Memory
	if task.GetMemory() == nil {
		memory = llm.NewContextManager("ops")
		memory.SetEventHandler(task.GetEventHandler())
		task.SetMemory(memory)
	} else {
		memory = task.GetMemory()
	}
	b := BuildAgentWithMemory(task, memory, systemPrompt, OpsToolFactories())
	agent := OpsCommonAgent{memory: b.Memory, client: b.Client, task: task, state: "Not Running", taskChan: make(chan TaskAssignment, 1)}
	agent.SetId(agent.Name() + "-" + uuid.New().String())
	return &agent, nil
}

func (c *OpsCommonAgent) StartTask(ctx context.Context) *StartResp {
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

func (c *OpsCommonAgent) executeTask(ctx context.Context, assignment TaskAssignment) *StartResp {
	config := &OpsCommonAgentConfig{}
	if err := json.Unmarshal([]byte(assignment.ArgsJson), config); err != nil {
		return &StartResp{Err: err}
	}
	taskContent := config.TaskContent
	tl := []map[string]string{{"TaskContent": taskContent}}
	c.task.SetTaskList(tl)

	c.memory.AddMessage(&llm.MessageX{
		Msg:       llm.Message{Role: llm.RoleUser, Content: "[New Task Assigned]\n" + taskContent},
		Shared:    false,
		ContextId: c.task.GetTaskId(),
	})

	c.SetState("Running")
	defer func() { c.SetState("Done") }()
	if len(c.task.GetTaskList()) < 2 {
		c.client.RemoveTool("TaskListTool")
	}
	var summary string
	model := misc.GetConfigValueDefault("ops", "MODEL", misc.GetConfigValueRequired("main_setting", "MODEL"))
	for {
		select {
		case <-ctx.Done():
			return &StartResp{Err: ctx.Err()}
		default:
		}
		var eventLog string
		c.memory.UnlockForLLM()
		msgList := c.memory.GetContext(c.task.GetTaskId())
		if msgList == nil {
			c.memory.LockForLLM()
			return &StartResp{Err: fmt.Errorf("agent task not set")}
		}
		c.memory.LockForLLM()
		debugLastMessages(c.profile.PersonaName, msgList)
		assistantMessage, toolMessage, err := c.client.ToolCallRequestWithLabel(ctx, llm.GetResponsesClient("ops", "main_setting"), msgList, model, c.Name(), c.profile.PersonaName, c.task.GetProjectName())
		if err != nil {
			c.memory.UnlockForLLM()
			return &StartResp{Err: err}
		}
		c.task.EmitAgentFeed(c.GetId(), "AgentMessage", map[string]interface{}{
			"role":    assistantMessage.Role,
			"content": assistantMessage.Content,
		})
		misc.Debug("[%s] 运维者响应: %s 消息大小: %d", c.profile.PersonaName, assistantMessage.Content, c.GetMemory().GetMsgSize(c.task.GetTaskId()))

		eventLog = eventLog + "assistant: " + assistantMessage.Content + "\n"
		var index = 0
		if len(assistantMessage.ToolCalls) > 0 {
			for _, tool := range assistantMessage.ToolCalls {
				misc.Debug("运维者tool： %s -- %s", tool.Name, tool.Arguments)
				c.task.EmitAgentFeed(c.GetId(), "AgentToolCall", map[string]interface{}{
					"stage":      "call",
					"toolCallID": tool.ID,
					"name":       tool.Name,
					"arguments":  tool.Arguments,
				})
				eventLog = eventLog + "ToolCalling: " + tool.Name + " args: " + tool.Arguments + "\n"
				eventLog = eventLog + "ToolResult: " + toolMessage[index].Content + "\n"
				c.task.EmitAgentFeed(c.GetId(), "AgentToolCall", map[string]interface{}{
					"stage":      "result",
					"toolCallID": tool.ID,
					"name":       tool.Name,
					"result":     toolMessage[index].Content,
				})
				index++
			}
		}
		_ = c.task.EventLog(eventLog)
		msg := &llm.MessageX{Msg: assistantMessage, Shared: false, ContextId: c.task.GetTaskId()}
		c.memory.AddMessage(msg)
		if len(toolMessage) > 0 {
			if s, ok := extractAgentFinishSummary(toolMessage); ok {
				summary = s
				for _, message := range toolMessage {
					msgTool := &llm.MessageX{Msg: message, Shared: false, ContextId: c.task.GetTaskId()}
					c.memory.AddMessage(msgTool)
				}
				c.memory.UnlockForLLM()
				break
			}
			for _, message := range toolMessage {
				msgTool := &llm.MessageX{Msg: message, Shared: false, ContextId: c.task.GetTaskId()}
				c.memory.AddMessage(msgTool)
			}
		} else {
			if c.memory.HasPendingUserMessage() {
				misc.Debug("%s: pending user message detected, continuing loop", c.Name())
				continue
			}
			misc.Debug("%s: 空响应（无工具调用），发送提醒继续", c.Name())
			c.memory.UnlockForLLM()
			c.memory.AddMessage(&llm.MessageX{Msg: llm.Message{Role: llm.RoleUser, Content: "Please continue your task. If you have finished, call AgentFinishTool with a summary."}, Shared: false, ContextId: c.task.GetTaskId()})
			continue
		}
		if err := c.memory.CompressIfNeeded(llm.GetResponsesClient("ops", "main_setting"), model); err != nil {
			misc.Debug("%s: memory compress error: %s", c.Name(), err.Error())
		}
	}
	return &StartResp{Err: nil, Memory: c.memory, EvnInfo: c.task.GetEnvInfo(), Summary: summary}
}

func (c *OpsCommonAgent) Name() string {
	return "OpsCommonAgent"
}
func (c *OpsCommonAgent) SetStateHandler(f func(string)) {
	c.stateHandler = f
}
