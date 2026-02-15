package agents

import (
	"AIxVuln/llm"
	"AIxVuln/misc"
	"AIxVuln/taskManager"
	"AIxVuln/toolCalling"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

type VerifierCommonAgent struct {
	memory       llm.Memory
	client       *toolCalling.ToolManager
	task         *taskManager.Task
	id           string
	state        string
	stateHandler func(string)
	profile      AgentProfile
	taskChan     chan TaskAssignment
}
type VerifierCommonAgentConfig struct {
	TaskContent    string `json:"task_content"`
	ExploitIdeaId  string `json:"exploit_idea_id"`
	ExploitChainId string `json:"exploit_chain_id"`
}

func (c *VerifierCommonAgent) GetTask() *taskManager.Task {
	return c.task
}

func (c *VerifierCommonAgent) GetMemory() llm.Memory {
	return c.memory
}
func (c *VerifierCommonAgent) GetId() string {
	return c.id
}
func (c *VerifierCommonAgent) SetId(id string) {
	c.id = id
}
func (c *VerifierCommonAgent) GetState() string {
	return c.state
}

func (c *VerifierCommonAgent) GetProfile() AgentProfile {
	return c.profile
}

func (c *VerifierCommonAgent) SetProfile(p AgentProfile) {
	c.profile = p
}
func (c *VerifierCommonAgent) SetState(state string) {
	c.state = state
	if c.stateHandler != nil {
		c.stateHandler(state)
	}
}
func (c *VerifierCommonAgent) SetEnvInfo(k map[string]interface{}) {
	c.task.SetEnvInfo(k)
}
func (c *VerifierCommonAgent) SetMemory(m llm.Memory) {
	c.memory = m
}

func (c *VerifierCommonAgent) SetKeyMessage(k map[string][]interface{}) {
	c.memory.SetKeyMessage(k, c.task.GetTaskId())
}

func (c *VerifierCommonAgent) AssignTask(assignment TaskAssignment) {
	c.taskChan <- assignment
}

func NewVerifierCommonAgent(task *taskManager.Task, argsJson string) (Agent, error) {
	task.SetAgentName("VerifierCommonAgent")
	systemPrompt := `Your task is to verify whether an 'exploitIdea' or 'exploitChain' is indeed exploitable and to provide a specific, step-by-step PoC (Proof of Concept) with expected results.
Some specific terms:  
1. 'exploitIdea' – a single fragmented exploitation point  
2. 'exploitChain' – an exploitation chain composed of one or more 'exploitIdea'. Both carry an "Idea" attribute to describe the exploitation or combined exploitation approach. Your task will clearly indicate whether you need to verify an 'exploitChain' or an 'exploitIdea'. In the following text, both will be referred to as exploit candidates (some are standalone vulnerabilities, some are chain components).
**Key Verification Threshold**: You **must never** mark a candidate as "verified" based solely on static code analysis.  
The actual environment verification target comes from the login information section in the key information, which uses the database data from the database information. You can read database content using the RunSQLTool.
The user will specify the exploit candidate currently requiring your verification. You should focus solely on verifying that candidate.  
For multi-step or complex requests, do not use curl; instead, use the RunPythonCodeTool. The Requests library is already installed in the system.
After completing the verification (regardless of success or failure), you must:
- Collect runtime evidence (please write in Chinese, using Markdown format, and be as detailed as possible), including complete HTTP request packets and the actual target's HTTP response packets.
- Write a Python attack script.
- If verifying an 'exploitIdea', call **SubmitExploitIdeaTool** to submit the verification results.
- If verifying an 'exploitChain', call **SubmitExploitChainTool** to submit the verification results, passing the runtime evidence and attack script as parameters.
- After submission, it will be reviewed. If the tool returns a failure, it will include the reason for the failed review and suggestions for improvement. When your submitted evidence fails the review, follow the suggestions to make improvements. If improvements cannot be made, resubmit the verification as failed.
**CRITICAL — Submit to the correct tool**: Check your task description carefully. If your task says "Verify this exploitChain", you MUST call **SubmitExploitChainTool** (NOT SubmitExploitIdeaTool). If your task says "Verify this exploitIdea", you MUST call **SubmitExploitIdeaTool** (NOT SubmitExploitChainTool). An ExploitChain may contain ExploitIdea references — do NOT submit results to SubmitExploitIdeaTool when your assignment is to verify an ExploitChain. Submitting to the wrong tool means your verification result is lost.
**Acceptable evidence for SubmitExploitIdeaTool includes**:
- Exact HTTP request URL + status code + key response headers + minimized response body snippet showing impact;
- Or accurate PoC standard output/error output containing unique markers;
- Or excerpts from server logs related to the exploitation attempt.
If it is an exploitIdea verification task, and it can achieve the effect described in the 'harm' field under the assumption that the conditions specified in the 'condition' field are satisfied, it may also be considered as verification passed, but runtime evidence and a PoC are still required.
**Mandatory Requirement**: As the verifier, you **must not** modify any content within the '/sourceCodeDir' directory.
**Authentication/Cookie Reuse**: If loginInfo contains a valid session cookie after administrator login, you **should** directly reuse it to access endpoints restricted to administrators (by sending it as a Cookie request header), rather than attempting to log in again.
**Environment/Route Reuse**: If routeInfo contains routing rules and valid URL examples, you **must** reuse those URLs/rules to make requests, rather than guessing routes.
You **do not need** to discover any new vulnerabilities, only to verify the specified one.
If you need to view logs in the target container, you can use DockerDirScanTool to list files and then DockerFileReadTool to read log files.
**You must provide conclusive runtime evidence that the exploit candidate is exploitable**. Methods such as simulating SQL execution, simulating code execution, speculating exploitability, or confirming solely through static code analysis **must never** be considered as evidence. If it is not feasible to obtain evidence through viable methods, you may submit it as a failed verification.
There are some tools in the '/data' directory, such as password dictionaries, phpggc, etc.
Pay attention to the 'WebEnvInfo' field in the key information—this is the target environment you need to verify.
` + CommonSystemPrompt()
	var memory llm.Memory
	if task.GetMemory() == nil {
		memory = llm.NewContextManager("verifier")
		memory.SetEventHandler(task.GetEventHandler())
		task.SetMemory(memory) // 不设置记忆体的话Agent将在超出历史记录限制之后不记得启动过的环境信息
	} else {
		memory = task.GetMemory()
	}
	b := BuildAgentWithMemory(task, memory, systemPrompt, VerifierToolFactories())
	agent := VerifierCommonAgent{memory: b.Memory, client: b.Client, task: task, state: "Not Running", taskChan: make(chan TaskAssignment, 1)}
	agent.SetId(agent.Name() + "-" + uuid.New().String())
	return &agent, nil
}

func (c *VerifierCommonAgent) StartTask(ctx context.Context) *StartResp {
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

func (c *VerifierCommonAgent) executeTask(ctx context.Context, assignment TaskAssignment) *StartResp {
	// Reset memory: compress previous session into a summary before starting new task.
	model := misc.GetConfigValueDefault("verifier", "MODEL", misc.GetConfigValueRequired("main_setting", "MODEL"))
	if err := c.memory.ResetMemoryWithSummary(llm.GetResponsesClient("verifier", "main_setting"), model); err != nil {
		misc.Debug("%s: ResetMemoryWithSummary error (non-fatal): %s", c.Name(), err.Error())
	}

	config := &VerifierCommonAgentConfig{}
	if err := json.Unmarshal([]byte(assignment.ArgsJson), config); err != nil {
		return &StartResp{Err: err}
	}

	// Build task content: fetch full exploit data by ID if provided.
	taskContent := strings.TrimSpace(config.TaskContent)
	if eid := strings.TrimSpace(config.ExploitIdeaId); eid != "" {
		getter := c.task.GetExploitIdeaGetter()
		if getter == nil {
			return &StartResp{Err: fmt.Errorf("exploit idea getter not configured")}
		}
		idea, err := getter(eid)
		if err != nil {
			return &StartResp{Err: fmt.Errorf("failed to fetch exploitIdea %s: %w", eid, err)}
		}
		ideaJson, _ := json.Marshal(idea)
		taskContent = fmt.Sprintf("Verify this exploitIdea: %s", string(ideaJson))
		if extra := strings.TrimSpace(config.TaskContent); extra != "" {
			taskContent += "\n\nAdditional instructions: " + extra
		}
	} else if cid := strings.TrimSpace(config.ExploitChainId); cid != "" {
		getter := c.task.GetExploitChainGetter()
		if getter == nil {
			return &StartResp{Err: fmt.Errorf("exploit chain getter not configured")}
		}
		chain, err := getter(cid)
		if err != nil {
			return &StartResp{Err: fmt.Errorf("failed to fetch exploitChain %s: %w", cid, err)}
		}
		chainJson, _ := json.Marshal(chain)
		taskContent = fmt.Sprintf("Verify this exploitChain: %s", string(chainJson))
		if extra := strings.TrimSpace(config.TaskContent); extra != "" {
			taskContent += "\n\nAdditional instructions: " + extra
		}
	}
	if taskContent == "" {
		return &StartResp{Err: fmt.Errorf("either exploit_idea_id, exploit_chain_id, or task_content is required")}
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
		assistantMessage, toolMessage, err := c.client.ToolCallRequestWithLabel(ctx, llm.GetResponsesClient("verifier", "main_setting"), msgList, model, c.Name(), c.profile.PersonaName, c.task.GetProjectName())
		if err != nil {
			c.memory.UnlockForLLM()
			return &StartResp{Err: err}
		}
		c.task.EmitAgentFeed(c.GetId(), "AgentMessage", map[string]interface{}{
			"role":    assistantMessage.Role,
			"content": assistantMessage.Content,
		})
		misc.Debug("[%s] 验证者响应: %s 消息大小: %d", c.profile.PersonaName, assistantMessage.Content, c.GetMemory().GetMsgSize(c.task.GetTaskId()))
		eventLog = eventLog + "assistant: " + assistantMessage.Content + "\n"
		var index = 0
		if len(assistantMessage.ToolCalls) > 0 {
			for _, tool := range assistantMessage.ToolCalls {
				misc.Debug("验证者tool：%s -- %s", tool.Name, tool.Arguments)
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
		if err := c.memory.CompressIfNeeded(llm.GetResponsesClient("verifier", "main_setting"), model); err != nil {
			misc.Debug("%s: memory compress error: %s", c.Name(), err.Error())
		}
	}
	return &StartResp{Err: nil, Memory: c.memory, EvnInfo: c.task.GetEnvInfo(), Summary: summary}
}

func (c *VerifierCommonAgent) Name() string {
	return "VerifierCommonAgent"
}

func (c *VerifierCommonAgent) SetStateHandler(f func(string)) {
	c.stateHandler = f
}
