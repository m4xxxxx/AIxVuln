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

type GeneralCommonAgent struct {
	memory       llm.Memory
	client       *toolCalling.ToolManager
	task         *taskManager.Task
	id           string
	state        string
	stateHandler func(string)
	profile      AgentProfile
	taskChan     chan TaskAssignment
}

type GeneralCommonAgentConfig struct {
	TaskContent string `json:"task_content"`
}

func (c *GeneralCommonAgent) GetTask() *taskManager.Task {
	return c.task
}

func (c *GeneralCommonAgent) GetMemory() llm.Memory {
	return c.memory
}

func (c *GeneralCommonAgent) GetId() string {
	return c.id
}

func (c *GeneralCommonAgent) SetId(id string) {
	c.id = id
}

func (c *GeneralCommonAgent) GetState() string {
	return c.state
}

func (c *GeneralCommonAgent) GetProfile() AgentProfile {
	return c.profile
}

func (c *GeneralCommonAgent) SetProfile(p AgentProfile) {
	c.profile = p
}

func (c *GeneralCommonAgent) SetState(state string) {
	c.state = state
	if c.stateHandler != nil {
		c.stateHandler(state)
	}
}

func (c *GeneralCommonAgent) SetMemory(m llm.Memory) {
	c.memory = m
}

func (c *GeneralCommonAgent) SetKeyMessage(k map[string][]interface{}) {
	c.memory.SetKeyMessage(k, c.task.GetTaskId())
}

func (c *GeneralCommonAgent) SetEnvInfo(k map[string]interface{}) {
	c.task.SetEnvInfo(k)
}

func (c *GeneralCommonAgent) AssignTask(assignment TaskAssignment) {
	c.taskChan <- assignment
}

func NewGeneralCommonAgent(task *taskManager.Task, argsJson string) (Agent, error) {
	task.SetAgentName("GeneralCommonAgent")
	systemPrompt := `You are a general-purpose digital human used for miscellaneous technical tasks.
You can perform mixed tasks including code reading, lightweight environment operations, script execution, and repository collaboration.

When the task involves GitHub repository operations (issues, PRs, workflow checks, discussions, file updates, code search, etc.), prefer using GitHub Copilot MCP tools first if available.
If MCP tools are unavailable or fail, provide a fallback plan and continue using local tools.

Always keep actions goal-oriented and avoid unnecessary broad scans. Use targeted reads and precise commands.
` + CommonSystemPrompt()

	var memory llm.Memory
	if task.GetMemory() == nil {
		memory = llm.NewContextManager("general")
		memory.SetEventHandler(task.GetEventHandler())
		task.SetMemory(memory)
	} else {
		memory = task.GetMemory()
	}

	b := BuildAgentWithMemory(task, memory, systemPrompt, GeneralToolFactories())
	b.Client.RegisterHandlers(toolCalling.GetGitHubCopilotMCPToolHandlers()...)

	agent := GeneralCommonAgent{
		memory:   b.Memory,
		client:   b.Client,
		task:     task,
		state:    "Not Running",
		taskChan: make(chan TaskAssignment, 1),
	}
	agent.SetId(agent.Name() + "-" + uuid.New().String())
	return &agent, nil
}

func (c *GeneralCommonAgent) StartTask(ctx context.Context) *StartResp {
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

func (c *GeneralCommonAgent) executeTask(ctx context.Context, assignment TaskAssignment) *StartResp {
	config := &GeneralCommonAgentConfig{}
	if err := json.Unmarshal([]byte(assignment.ArgsJson), config); err != nil {
		return &StartResp{Err: err}
	}
	taskContent := config.TaskContent
	if taskContent == "" {
		return &StartResp{Err: fmt.Errorf("task_content is required")}
	}

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
	model := misc.GetConfigValueDefault("general", "MODEL", misc.GetConfigValueRequired("main_setting", "MODEL"))
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
		assistantMessage, toolMessage, err := c.client.ToolCallRequestWithLabel(ctx, llm.GetResponsesClient("general", "main_setting"), msgList, model, c.Name(), c.profile.PersonaName, c.task.GetProjectName())
		if err != nil {
			c.memory.UnlockForLLM()
			return &StartResp{Err: err}
		}
		c.task.EmitAgentFeed(c.GetId(), "AgentMessage", map[string]interface{}{
			"role":    assistantMessage.Role,
			"content": assistantMessage.Content,
		})
		misc.Debug("[%s] 通用者响应: %s 消息大小: %d", c.profile.PersonaName, assistantMessage.Content, c.GetMemory().GetMsgSize(c.task.GetTaskId()))

		eventLog = eventLog + "assistant: " + assistantMessage.Content + "\n"
		index := 0
		if len(assistantMessage.ToolCalls) > 0 {
			for _, tool := range assistantMessage.ToolCalls {
				misc.Debug("通用者tool：%s -- %s", tool.Name, tool.Arguments)
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
		c.memory.AddMessage(&llm.MessageX{Msg: assistantMessage, Shared: false, ContextId: c.task.GetTaskId()})
		if len(toolMessage) > 0 {
			if s, ok := extractAgentFinishSummary(toolMessage); ok {
				summary = s
				for _, message := range toolMessage {
					c.memory.AddMessage(&llm.MessageX{Msg: message, Shared: false, ContextId: c.task.GetTaskId()})
				}
				c.memory.UnlockForLLM()
				break
			}
			for _, message := range toolMessage {
				c.memory.AddMessage(&llm.MessageX{Msg: message, Shared: false, ContextId: c.task.GetTaskId()})
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
		if err := c.memory.CompressIfNeeded(llm.GetResponsesClient("general", "main_setting"), model); err != nil {
			misc.Debug("%s: memory compress error: %s", c.Name(), err.Error())
		}
	}
	return &StartResp{Err: nil, Memory: c.memory, EvnInfo: c.task.GetEnvInfo(), Summary: summary}
}

func (c *GeneralCommonAgent) Name() string {
	return "GeneralCommonAgent"
}

func (c *GeneralCommonAgent) SetStateHandler(f func(string)) {
	c.stateHandler = f
}
