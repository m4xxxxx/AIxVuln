package agents

import (
	"AIxVuln/llm"
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"AIxVuln/toolCalling"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

type AnalyzeCommonAgent struct {
	memory       llm.Memory
	client       *toolCalling.ToolManager
	task         *taskManager.Task
	id           string
	config       *AnalyzeCommonAgentConfig
	state        string
	stateHandler func(string)
	profile      AgentProfile
	taskChan     chan TaskAssignment
}

type AnalyzeCommonAgentConfig struct {
	ExploitIdeaMaxCount string `json:"exploitIdeaMaxCount"`
	TaskContent         string `json:"task_content"`
}

func (c *AnalyzeCommonAgent) GetTask() *taskManager.Task {
	return c.task
}
func (c *AnalyzeCommonAgent) GetMemory() llm.Memory {
	return c.memory
}
func (c *AnalyzeCommonAgent) GetId() string {
	return c.id
}
func (c *AnalyzeCommonAgent) SetId(id string) {
	c.id = id
}
func (c *AnalyzeCommonAgent) GetState() string {
	return c.state
}

func (c *AnalyzeCommonAgent) GetProfile() AgentProfile {
	return c.profile
}

func (c *AnalyzeCommonAgent) SetProfile(p AgentProfile) {
	c.profile = p
}
func (c *AnalyzeCommonAgent) SetState(state string) {
	c.state = state
	if c.stateHandler != nil {
		c.stateHandler(state)
	}
}
func (c *AnalyzeCommonAgent) SetMemory(m llm.Memory) {
	c.memory = m
}
func (c *AnalyzeCommonAgent) SetEnvInfo(k map[string]interface{}) {
	c.task.SetEnvInfo(k)
}
func (c *AnalyzeCommonAgent) SetKeyMessage(k map[string][]interface{}) {
	c.memory.SetKeyMessage(k, c.task.GetTaskId())
}

func (c *AnalyzeCommonAgent) AssignTask(assignment TaskAssignment) {
	c.taskChan <- assignment
}

func NewAnalyzeCommonAgent(task *taskManager.Task, argsJson string) (Agent, error) {
	task.SetAgentName("AnalyzeCommonAgent")
	systemPrompt := `You are a Code Analyst
Some specific terms: 1. 'exploitIdea' – a single fragmented exploitation point 2. 'exploitChain' – an exploitation chain composed of one or more 'exploitIdea'.
Your job is to complete given 'exploitIdea' mining or code analysis tasks by safely reading code using A+ tools (search + targeted file reads). The task will clearly specify whether you need to perform a mining task or a code analysis task.
Do NOT read the entire codebase. Start with the ListSourceCodeTreeTool, then the SearchFileContentsByRegexTool, and then use the ReadLinesFromFileTool to read files in small slices.
Prefer evidence with file paths and line numbers.
If you discover an 'exploitIdea', you need to call the IssueCandidateExploitIdeaTool to report it.
IMPORTANT — Avoid duplicate mining: Check the 'DiscoveredExploitIdeas' section in your key_info. It contains a list of vulnerabilities already discovered by you or other analysts (id, title, type, file, route, state). Do NOT submit an exploitIdea that targets the same file + route + vulnerability type combination as one already listed. Focus your efforts on finding NEW, undiscovered vulnerabilities.
` + CommonSystemPrompt()
	var memory llm.Memory
	if task.GetMemory() == nil {
		memory = llm.NewContextManager("analyze")
		memory.SetEventHandler(task.GetEventHandler())
		task.SetMemory(memory)
	} else {
		memory = task.GetMemory()
	}
	b := BuildAgentWithMemory(task, memory, systemPrompt, AnalyzeToolFactories())
	agent := AnalyzeCommonAgent{memory: b.Memory, client: b.Client, task: task, state: "Not Running", taskChan: make(chan TaskAssignment, 1)}
	agent.SetId(agent.Name() + "-" + uuid.New().String())
	return &agent, nil
}

func (c *AnalyzeCommonAgent) StartTask(ctx context.Context) *StartResp {
	for {
		// Wait for a task assignment or context cancellation.
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
		// If context was cancelled during execution, exit the outer loop.
		if ctx.Err() != nil {
			return resp
		}
	}
}

func (c *AnalyzeCommonAgent) executeTask(ctx context.Context, assignment TaskAssignment) *StartResp {
	config := &AnalyzeCommonAgentConfig{}
	if err := json.Unmarshal([]byte(assignment.ArgsJson), config); err != nil {
		return &StartResp{Err: err}
	}
	c.config = config

	// Per-task quota state must be reset on every assignment.
	c.task.ResetExploitIdeaSubmitted()
	c.task.SetExploitIdeaQuota(0)

	// Inject task content as a user message.
	taskContent := config.TaskContent
	if maxCountText := strings.TrimSpace(config.ExploitIdeaMaxCount); maxCountText != "" {
		maxCount, err := strconv.Atoi(maxCountText)
		if err == nil && maxCount > 0 {
			c.task.SetExploitIdeaQuota(maxCount)
		} else {
			misc.Debug("%s: invalid exploitIdeaMaxCount=%q, fallback to unlimited per-task quota", c.Name(), config.ExploitIdeaMaxCount)
		}
		taskContent += fmt.Sprintf("\nYou can submit a maximum of **%s NEW ExploitIdea** in THIS task (this is your per-task quota, independent of previously discovered ones). Once this limit is reached, immediately stop mining and call AgentFinishTool.", config.ExploitIdeaMaxCount)
	}
	tl := []map[string]string{{"TaskContent": taskContent}}
	c.task.SetTaskList(tl)

	c.memory.AddMessage(&llm.MessageX{
		Msg:       llm.Message{Role: llm.RoleUser, Content: "[New Task Assigned]\n" + taskContent},
		Shared:    false,
		ContextId: c.task.GetTaskId(),
	})

	c.SetState("Running")
	defer func() {
		c.SetState("Done")
	}()
	if len(c.task.GetTaskList()) < 2 {
		c.client.RemoveTool("TaskListTool")
	}
	var summary string
	model := misc.GetConfigValueDefault("analyze", "MODEL", misc.GetConfigValueRequired("main_setting", "MODEL"))
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
		assistantMessage, toolMessage, err := c.client.ToolCallRequestWithLabel(ctx, llm.GetResponsesClient("analyze", "main_setting"), msgList, model, c.Name(), c.profile.PersonaName, c.task.GetProjectName())
		if err != nil {
			c.memory.UnlockForLLM()
			return &StartResp{Err: err}
		}
		c.task.EmitAgentFeed(c.GetId(), "AgentMessage", map[string]interface{}{
			"role":    assistantMessage.Role,
			"content": assistantMessage.Content,
		})
		misc.Debug("[%s] 分析者响应: %s 消息大小: %d", c.profile.PersonaName, assistantMessage.Content, c.GetMemory().GetMsgSize(c.task.GetTaskId()))
		eventLog = eventLog + "assistant: " + assistantMessage.Content + "\n"
		var index = 0
		if len(assistantMessage.ToolCalls) > 0 {
			for _, tool := range assistantMessage.ToolCalls {
				misc.Debug("分析者tool：%s -- %s", tool.Name, tool.Arguments)
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
			// Check if AgentFinishTool was called.
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
			// Empty response with no tool calls — do NOT exit, nudge the LLM to continue.
			misc.Debug("%s: 空响应（无工具调用），发送提醒继续", c.Name())
			c.memory.UnlockForLLM()
			c.memory.AddMessage(&llm.MessageX{Msg: llm.Message{Role: llm.RoleUser, Content: "Please continue your task. If you have finished, call AgentFinishTool with a summary."}, Shared: false, ContextId: c.task.GetTaskId()})
			continue
		}
		if err := c.memory.CompressIfNeeded(llm.GetResponsesClient("analyze", "main_setting"), model); err != nil {
			misc.Debug("%s: memory compress error: %s", c.Name(), err.Error())
		}
	}
	return &StartResp{Err: nil, Memory: c.memory, EvnInfo: c.task.GetEnvInfo(), Summary: summary}
}

func (c *AnalyzeCommonAgent) Name() string {
	return "AnalyzeCommonAgent"
}

func (c *AnalyzeCommonAgent) SetStateHandler(f func(string)) {
	c.stateHandler = f
}
