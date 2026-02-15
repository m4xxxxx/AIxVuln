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

type OpsEnvScoutAgent struct {
	memory       llm.Memory
	client       *toolCalling.ToolManager
	task         *taskManager.Task
	id           string
	state        string
	stateHandler func(string)
	config       *OpsEnvScoutAgentConfig
	profile      AgentProfile
	taskChan     chan TaskAssignment
}
type OpsEnvScoutAgentConfig struct {
	TaskContent   string `json:"task_content"`
	EnvBaseConfig string `json:"env_base_config"`
}

func (c *OpsEnvScoutAgent) GetTask() *taskManager.Task {
	return c.task
}

func (c *OpsEnvScoutAgent) GetMemory() llm.Memory {
	return c.memory
}
func (c *OpsEnvScoutAgent) GetId() string {
	return c.id
}
func (c *OpsEnvScoutAgent) SetId(id string) {
	c.id = id
}
func (c *OpsEnvScoutAgent) GetState() string {
	return c.state
}

func (c *OpsEnvScoutAgent) GetProfile() AgentProfile {
	return c.profile
}

func (c *OpsEnvScoutAgent) SetProfile(p AgentProfile) {
	c.profile = p
}
func (c *OpsEnvScoutAgent) SetState(state string) {
	c.state = state
	if c.stateHandler != nil {
		c.stateHandler(state)
	}
}
func (c *OpsEnvScoutAgent) SetMemory(m llm.Memory) {
	c.memory = m
}
func (c *OpsEnvScoutAgent) SetKeyMessage(k map[string][]interface{}) {
	c.memory.SetKeyMessage(k, c.task.GetTaskId())
}
func (c *OpsEnvScoutAgent) SetEnvInfo(k map[string]interface{}) {
	c.task.SetEnvInfo(k)
}

func (c *OpsEnvScoutAgent) AssignTask(assignment TaskAssignment) {
	c.taskChan <- assignment
}

func NewOpsEnvScoutAgent(task *taskManager.Task, argsJson string) (Agent, error) {
	task.SetAgentName("OpsEnvScoutAgent")
	systemPrompt := `You are an operations AI assistant. Your main task is to operate within the provided existing WEB testing environment and collect the data required for security research tasks.You may be given SSH connection credentials or a Docker container ID. You need to use tools to operate these environments and complete tasks.**Direct execution of Docker commands is prohibited**. All operations must be performed through DockerxxxTool tools.You must complete the following foundational tasks before proceeding with any other assigned tasks.
1. **CAPTCHA Handling**: If the WEB system's login process includes a CAPTCHA, modify the source code to **disable the CAPTCHA login feature**.
2. **Route Analysis**: Analyze the route access methods and provide **three examples of successfully accessed routes**.
3. **System Login**: Log in to the system and obtain a **valid COOKIE**. If no password is provided, you need to retrieve or reset the password within the environment.
4. **Save environment information**: Be sure to call EnvSaveTool to save critical information, and submit credentials using a valid Cookie obtained after login.
Keep summaries concise. Simply use EnvSaveTool to save the environment information before the end of each task.
The IP used in the collected information such as URLs and IPs cannot be 127.0.0.1. When a container is provided, check /etc/hosts to determine the externally accessible IP address.` + CommonSystemPrompt()
	var memory llm.Memory
	if task.GetMemory() == nil {
		memory = llm.NewContextManager("ops")
		memory.SetEventHandler(task.GetEventHandler())
		task.SetMemory(memory)
	} else {
		memory = task.GetMemory()
	}
	tools := OpsToolFactories()
	tools = append(tools, func(task *taskManager.Task) toolCalling.ToolHandler { return toolCalling.NewRunSSHCommandTool(task) })
	b := BuildAgentWithMemory(task, memory, systemPrompt, tools)
	agent := OpsEnvScoutAgent{memory: b.Memory, client: b.Client, task: task, state: "Not Running", taskChan: make(chan TaskAssignment, 1)}
	agent.SetId(agent.Name() + "-" + uuid.New().String())
	return &agent, nil
}

func (c *OpsEnvScoutAgent) StartTask(ctx context.Context) *StartResp {
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

func (c *OpsEnvScoutAgent) executeTask(ctx context.Context, assignment TaskAssignment) *StartResp {
	config := &OpsEnvScoutAgentConfig{}
	if err := json.Unmarshal([]byte(assignment.ArgsJson), config); err != nil {
		return &StartResp{Err: err}
	}
	c.config = config
	taskContent := config.TaskContent
	tl := []map[string]string{{"TaskContent": taskContent}}
	c.task.SetTaskList(tl)

	if config.EnvBaseConfig != "" {
		c.memory.AddKeyMessage(&llm.EnvMessageX{
			Key:       "TargetEnvironmentInformation",
			Content:   config.EnvBaseConfig,
			AppendEnv: false,
		})
	}

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

func (c *OpsEnvScoutAgent) Name() string {
	return "OpsEnvScoutAgent"
}
func (c *OpsEnvScoutAgent) SetStateHandler(f func(string)) {
	c.stateHandler = f
}
