package DecisionBrain

import (
	"AIxVuln/llm"
	"AIxVuln/misc"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var systemPrompt = `You are the decision-making brain, responsible for organizing various agents to accomplish the overall vulnerability discovery task. Before completing the overall task, you need to maintain fragmented exploit points and schedule various agents to drive the combination of these fragmented exploit points into an attack chain that can fulfill the overall vulnerability discovery task (the attack chain is the ultimate goal).  
Some specific terms:  
1. 'exploitIdea': A single fragmented exploit point  
2. 'exploitChain': An attack chain composed of one or more 'exploitIdea's  
These agents execute tasks within containers, and the source code of the project under audit is mapped to '/sourceCodeDir' in the containers. The agents have already been informed that the source code directory is '/sourceCodeDir'. When starting operations, analysis, verification, and report-writing agents, there is no need to specify the source code path. If you need to understand the source code content, you can schedule analysis-type agents to gather information.  
Agents may generate irrelevant events that wake you up. When you feel no action is currently required and need to wait for the digital humans to continue running, call 'Tool-Wait'. The system will wake you when something changes.  
You MUST call 'Tool-FinishTask' when you believe all objectives have been met. CRITICAL: Before calling 'Tool-FinishTask', you MUST verify that ALL agents have completed (ACTIVE_AGENT_COUNT must be 0 in agent_runtime_summary). If any agent is still running, call 'Tool-Wait' instead and wait for them to finish. The system will reject your FinishTask call if agents are still active. The system will ask the user for confirmation before actually stopping. If the user wants to continue, you will receive their reply. You must NEVER stop on your own without calling 'Tool-FinishTask' — always use this tool to end the task.  
These agents and tools can run concurrently. When using tools to run them, consider efficiency. For example, while the operations agent is setting up the environment, the analysis agent can simultaneously perform code analysis tasks because both can access '/sourceCodeDir'. When you need to run them concurrently, return multiple tool calls simultaneously. Before setting up the environment and starting the discovery task, you can run them first to collect project source code information and then make decisions.  
Regarding the use of 'Ops'-type agents: You must clearly tell them whether to perform a setup task or an operations task, and then specify the task content. If it is 'OpsEnvScoutAgent', inform it of all the environmental information you know.  
Regarding the use of 'Analyze'-type agents: Clearly tell them whether they need to perform a regular code analysis task or an 'exploitIdea' discovery task.  
Regarding the use of 'Verifier'-type agents: The prerequisite is that the environment must already be deployed; otherwise, they will wait indefinitely for deployment. This type of agent can only be scheduled when 'target environment information' is not empty. You only need to pass the exploit_idea_id (e.g. 'E.0') or exploit_chain_id (e.g. 'C.0') — the agent will automatically fetch the full exploit data internally. Do NOT paste the full exploit JSON into the arguments. After it completes verification, you will see the status of the 'exploitIdea' or 'exploitChain' change in the list. In most cases, you do not need to actively call it, as it will run automatically after the analyzer submits an 'exploitIdea' or after you perform an exploit point combination operation. Call it only when you need to re-verify.  
Regarding the use of 'Report'-type agents: You only need to pass the exploit_idea_id or exploit_chain_id plus the reportType — the agent will automatically fetch the full exploit data (including runtime evidence and PoC) internally. Do NOT paste the full exploit JSON into the arguments. You can optionally include extra notes in task_content.  
Discovery strategy recommendations:  
1. To verify after discovering exploitable points, generally use Ops-type agents to set up the environment or collect testing information from existing test environments. While setting up or gathering information, you can start analysis-type agents to analyze exploitable points.  
2. First, collect various exploitable points of the project under audit (referred to as 'ExploitIdea' in the overall framework). Code analysis agents will gather information such as exploit conditions, achievable exploit effects, and suggestions for expanding the exploit.  
3. Pay attention to the status of 'exploitIdea's. Focus on exploit points with the status "exploitable." If these exploit points can be combined into an exploit chain, call 'SynthesizeChainTool' to combine them into an 'exploitChain'. An 'exploitChain' can consist of one or more 'exploitIdea's.  
4. If the 'exploitChain' is successfully exploited, schedule the report agent to write the report.  

**CRITICAL — Persistent Mining Strategy:**
Your primary objective is THOROUGH vulnerability discovery. Do NOT stop mining prematurely. Follow these rules:
- **Multi-round analysis**: After each round of analysis agents completes, review the exploit_idea_list. If the discovered exploitIdeas only cover a narrow set of vulnerability types or code modules, you MUST schedule additional analysis agents to explore OTHER areas. For example:
  * Different vulnerability categories: SQL injection, XSS, SSRF, file upload, deserialization, authentication bypass, privilege escalation, information disclosure, command injection, path traversal, etc.
  * Different code modules: controllers, API endpoints, middleware, authentication logic, file handling, database queries, admin panels, etc.
- **Minimum coverage expectation**: Before considering finishing, you should have attempted to analyze at least 3-5 different vulnerability categories relevant to the project's tech stack. If you have fewer than 3 exploitIdeas, you almost certainly have NOT done enough analysis.
- **Iterative deepening**: When an analysis agent finishes with few or no findings in one area, assign another agent to explore a DIFFERENT area rather than giving up. Each agent should be given a SPECIFIC and DISTINCT focus (e.g. "focus on authentication and session management vulnerabilities" or "focus on file upload and path traversal vulnerabilities").
- **Concurrent analysis**: Run multiple analysis agents in parallel with different focus areas to maximize coverage and efficiency.
- **Do NOT finish early**: The fact that one or two analysis rounds found nothing does not mean the project is secure. It may mean you need to look in different places or from different angles.

Important:  
- Do not repeatedly call the same agent to perform exactly the same task. If you see it running in the status information but producing no results, this is normal—simply wait for it to continue.  
- Do not insist on having analysis agents directly discover 'exploitChain's. If they cannot directly discover the target 'exploitChain', have them first discover related 'exploitIdea's and then combine them into an 'exploitChain'.  
- Rules for using 'Ops'-type agents when setting up the environment: When testing using an already running test environment, call 'Agent-Ops-OpsEnvScoutAgent'; if setting up the environment from scratch, use other 'Ops'-type agents.
- CRITICAL: Check 'ENV_READY' in agent_runtime_summary and the 'env' section BEFORE scheduling any Ops agent. If ENV_READY is true and the env section contains valid environment information (URLs, ports, credentials, etc.), the environment has ALREADY been set up — do NOT schedule any Ops agent to build it again. Repeated environment setup wastes resources and may break the existing environment. Only schedule Ops agents for environment tasks when ENV_READY is false and the env section says "not generated".

Digital Human Team:
Each tool you call (Agent-Ops-*, Agent-Analyze-*, Agent-Verifier-*, Agent-Report-*) is backed by a real digital human with a name, gender, and personality. The mapping is provided in the 'DigitalHumanRoster' section of your context. When communicating with the user via UserMessage, you MUST refer to digital humans by their persona name (e.g. "已安排温舒然进行代码分析" instead of "已启动分析代理"). Never use technical terms like "代理", "Agent", "运维代理", "分析代理" etc. in UserMessage — always use the digital human's actual name. You are the team leader managing these digital humans.
You have a 'Tool-SendMessageToDigitalHuman' tool that lets you send instructions to any digital human by persona name, regardless of whether they are busy or idle.
IMPORTANT: <UserMessage> only notifies the USER — digital humans will NOT receive it. If you want to actually communicate with a digital human (e.g. urge them, give follow-up instructions, ask a question), you MUST call 'Tool-SendMessageToDigitalHuman'. Simply mentioning a digital human's name in <UserMessage> does NOT deliver anything to them.

UserMessage (notify the user):
Use <UserMessage>...</UserMessage> tags to communicate with the user. The content will be shown in the user's chat panel. Use it for:
- Replying to the user when you receive a '[Team Chat]' message
- Proactively notifying the user of important progress at key milestones (e.g. "已安排张伟进行环境搭建", "已让李娜开始代码审计", "温舒然发现了SQL注入线索", "所有任务已完成")
If there is nothing to tell the user, do NOT include a UserMessage tag.`

// ContextBreakdownSection represents the token size of one section in the brain context.
type ContextBreakdownSection struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
}

// ContextBreakdown holds the per-section token breakdown of the brain context.
type ContextBreakdown struct {
	Sections   []ContextBreakdownSection `json:"sections"`
	Total      int                       `json:"total"`
	MaxContext int                       `json:"max_context"`
}

type BrainMemory struct {
	systemPrompt            string
	memory                  []llm.Turn
	mu                      sync.RWMutex
	keyMessage              map[string][]interface{} // 重要的信息，这个信息永远不会被覆盖
	maxHistory              int                      // 历史对话记录最大token数
	compressedSummary       string                   // 压缩后的历史摘要（用于保留关键信息）
	taskContent             string
	eventHandler            func(string, string, int)
	agentRuntimeInfoJson    string
	envInfoJson             string
	exploitIdeaListJson     string
	exploitChainListJson    string
	containerInfoList       string
	noAgentRunning          bool
	userPromptHash          string
	msgSize                 int // 消息大小
	lastUserPrompt          string
	lastPanelSectionHash    map[string]string
	lastPanelSectionContent map[string]string
	seenAgentRuntime        map[string]bool
	contextBreakdown        ContextBreakdown
}

func NewBrainMemory() *BrainMemory {
	return &BrainMemory{
		memory:                  make([]llm.Turn, 0),
		maxHistory:              misc.GetMaxContext("decision"),
		systemPrompt:            systemPrompt,
		keyMessage:              make(map[string][]interface{}),
		lastPanelSectionHash:    map[string]string{},
		lastPanelSectionContent: map[string]string{},
		seenAgentRuntime:        map[string]bool{},
	}
}
func (br *BrainMemory) SaveMemoryToFile(filename string) error {
	memoryInfoJson, _ := json.Marshal(br)
	err := os.WriteFile(filename, memoryInfoJson, 0644)
	return err
}

func (br *BrainMemory) LoadMemoryFromFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var saved BrainMemory
	if err := json.Unmarshal(content, &saved); err != nil {
		return err
	}
	br.mu.Lock()
	defer br.mu.Unlock()
	if saved.systemPrompt != "" {
		br.systemPrompt = saved.systemPrompt
	} else if br.systemPrompt == "" {
		br.systemPrompt = systemPrompt
	}
	br.memory = saved.memory
	if saved.keyMessage != nil {
		br.keyMessage = saved.keyMessage
	} else if br.keyMessage == nil {
		br.keyMessage = make(map[string][]interface{})
	}
	br.maxHistory = saved.maxHistory
	br.compressedSummary = saved.compressedSummary
	br.taskContent = saved.taskContent
	br.agentRuntimeInfoJson = saved.agentRuntimeInfoJson
	br.envInfoJson = saved.envInfoJson
	br.exploitIdeaListJson = saved.exploitIdeaListJson
	br.exploitChainListJson = saved.exploitChainListJson
	br.containerInfoList = saved.containerInfoList
	br.noAgentRunning = saved.noAgentRunning
	br.userPromptHash = saved.userPromptHash
	br.msgSize = saved.msgSize
	br.lastUserPrompt = saved.lastUserPrompt
	if saved.lastPanelSectionHash != nil {
		br.lastPanelSectionHash = saved.lastPanelSectionHash
	} else if br.lastPanelSectionHash == nil {
		br.lastPanelSectionHash = map[string]string{}
	}
	if saved.lastPanelSectionContent != nil {
		br.lastPanelSectionContent = saved.lastPanelSectionContent
	} else if br.lastPanelSectionContent == nil {
		br.lastPanelSectionContent = map[string]string{}
	}
	if saved.seenAgentRuntime != nil {
		br.seenAgentRuntime = saved.seenAgentRuntime
	} else if br.seenAgentRuntime == nil {
		br.seenAgentRuntime = map[string]bool{}
	}
	return nil
}

func (br *BrainMemory) SetEventHandler(f func(string, string, int)) {
	br.eventHandler = f
}

func (br *BrainMemory) SetNoAgentRunning(v bool) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.noAgentRunning = v
}
func (br *BrainMemory) GetLastHash() string {
	br.mu.RLock()
	defer br.mu.RUnlock()
	return br.userPromptHash
}
func (br *BrainMemory) UpdateAgentRuntimeInfo(rJson string) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.agentRuntimeInfoJson = rJson
}

func (br *BrainMemory) SetTaskContent(taskContent string) {
	br.taskContent = taskContent
}

func (br *BrainMemory) AddMessage(x llm.Message) {
	if len(x.Content) > misc.GetMessageMaximum() {
		x.Content = x.Content[:misc.GetMessageMaximum()] + " ...... (The text exceeds the maximum length of " + strconv.Itoa(misc.GetMessageMaximum()) + " characters and cannot be sent to the LLM—)."
	}
	br.mu.Lock()
	defer br.mu.Unlock()
	// Tool messages are appended to the last turn (which should be an assistant
	// turn with tool_calls). All other messages start a new turn.
	if x.Role == llm.RoleTool && len(br.memory) > 0 {
		last := &br.memory[len(br.memory)-1]
		if last.Role() == llm.RoleAssistant && last.HasToolCalls() {
			last.Messages = append(last.Messages, x)
		} else {
			br.memory = append(br.memory, llm.Turn{Messages: []llm.Message{x}})
		}
	} else {
		br.memory = append(br.memory, llm.Turn{Messages: []llm.Message{x}})
	}
	// Invalidate hash so Start() loop detects the new message and doesn't skip it.
	if x.Role == llm.RoleUser {
		br.userPromptHash = ""
	}
}

const brainMemorySummaryPrefix = "[Memory Summary]"

func (br *BrainMemory) shouldCompressLocked() bool {
	if br.maxHistory <= 0 {
		return false
	}
	return llm.TurnsSize(br.memory) > br.maxHistory
}

// snapshotCompressionCandidateLocked splits memory into old (to be summarized)
// and recent (to keep). Recent turns are selected by size budget: we keep as
// many recent turns as fit within half of maxHistory, ensuring the kept portion
// won't immediately trigger another compression.
func (br *BrainMemory) snapshotCompressionCandidateLocked() (string, []llm.Message, []llm.Turn, bool) {
	if len(br.memory) <= 2 {
		return "", nil, nil, false
	}
	budget := br.maxHistory / 2
	if budget < 4096 {
		budget = 4096
	}
	// Walk backwards to find how many recent turns fit in the budget.
	kept := 0
	size := 0
	for i := len(br.memory) - 1; i >= 0; i-- {
		s := br.memory[i].Size()
		if size+s > budget && kept >= 4 {
			break
		}
		size += s
		kept++
	}
	if kept >= len(br.memory) {
		return "", nil, nil, false
	}
	splitIdx := len(br.memory) - kept
	oldFlat := llm.FlattenTurns(br.memory[:splitIdx])
	recentTurns := make([]llm.Turn, kept)
	copy(recentTurns, br.memory[splitIdx:])
	return br.compressedSummary, oldFlat, recentTurns, true
}

func (br *BrainMemory) CompressIfNeeded(cli llm.Client, model string) error {
	if cli == nil {
		return errors.New("nil openai client")
	}

	br.mu.Lock()
	need := br.shouldCompressLocked()
	br.mu.Unlock()
	if !need {
		return nil
	}

	var existingSummary string
	var oldMessages []llm.Message
	var recentTurns []llm.Turn
	var ok bool
	br.mu.Lock()
	existingSummary, oldMessages, recentTurns, ok = br.snapshotCompressionCandidateLocked()
	br.mu.Unlock()
	if !ok {
		return nil
	}

	newSummary, err := br.summarizeWithLLM(cli, model, existingSummary, oldMessages)
	if err != nil {
		// Fallback: LLM compression failed. Force-drop old turns to prevent
		// unbounded memory growth. Keep only the recent turns.
		misc.Debug("BrainMemory.CompressIfNeeded: LLM summarization failed (%s), falling back to hard truncation", err.Error())
		br.mu.Lock()
		defer br.mu.Unlock()
		_, _, recentFallback, okFallback := br.snapshotCompressionCandidateLocked()
		if okFallback {
			fallbackMsg := brainMemorySummaryPrefix + "\n(Automatic compression failed. Older conversation history has been discarded to stay within context limits.)"
			if br.compressedSummary != "" {
				fallbackMsg += "\nPrevious summary:\n" + br.compressedSummary
			}
			summaryTurn := llm.Turn{Messages: []llm.Message{{Role: llm.RoleUser, Content: fallbackMsg}}}
			br.memory = append([]llm.Turn{summaryTurn}, recentFallback...)
		}
		return err
	}

	br.mu.Lock()
	defer br.mu.Unlock()
	br.compressedSummary = newSummary
	summaryTurn := llm.Turn{Messages: []llm.Message{br.buildSummaryMessageLocked()}}
	br.memory = append([]llm.Turn{summaryTurn}, recentTurns...)
	return nil
}

func (br *BrainMemory) buildSummaryMessageLocked() llm.Message {
	content := brainMemorySummaryPrefix + "\n(This summary was automatically generated by the system to compress earlier conversation history. It is NOT a message you wrote — treat it as reference context.)"
	if br.compressedSummary != "" {
		content = content + "\n" + br.compressedSummary
	}
	return llm.Message{Role: llm.RoleUser, Content: content}
}

func (br *BrainMemory) isCompressedLocked() bool {
	if br.compressedSummary != "" {
		return true
	}
	if len(br.memory) > 0 && len(br.memory[0].Messages) > 0 && strings.HasPrefix(br.memory[0].Messages[0].Content, brainMemorySummaryPrefix) {
		return true
	}
	return false
}

func (br *BrainMemory) IsCompressed() bool {
	br.mu.Lock()
	defer br.mu.Unlock()
	return br.isCompressedLocked()
}

func (br *BrainMemory) summarizeWithLLM(cli llm.Client, model string, existingSummary string, msgs []llm.Message) (string, error) {
	// Build a budget-aware representation of messages to summarize.
	// We allow up to ~60KB of message content for the summarization prompt.
	const maxPromptBytes = 60000
	const maxPerAssistant = 2000
	const maxPerTool = 400
	const maxPerUser = 1500

	trimmed := make([]map[string]string, 0, len(msgs))
	totalBytes := 0
	for _, m := range msgs {
		c := m.Content
		limit := maxPerUser
		switch m.Role {
		case llm.RoleAssistant:
			limit = maxPerAssistant
		case llm.RoleTool:
			limit = maxPerTool
		}
		if len(c) > limit {
			c = c[:limit] + " ...[truncated]"
		}
		if totalBytes+len(c) > maxPromptBytes {
			// Budget exhausted — skip remaining older messages.
			break
		}
		totalBytes += len(c)
		trimmed = append(trimmed, map[string]string{"role": m.Role, "content": c})
	}
	js, _ := json.Marshal(trimmed)

	sys := "You are a memory compression agent. Summarize the provided chat history into a concise, loss-minimizing memory.\n" +
		"Rules:\n" +
		"- Preserve: all decisions made, task assignments to digital humans (who was assigned what), discovered vulnerabilities and their IDs, exploit ideas/chains and their states, file paths, environment info (URLs/ports/credentials), API/tool usage constraints, and any invariants.\n" +
		"- For tool call results: keep the conclusion/outcome, discard verbose raw output.\n" +
		"- Keep it compact: use short sections and bullet points.\n" +
		"- Do NOT include code blocks.\n" +
		"- Output in plain text (Chinese is OK)."

	user := "Existing summary (may be empty):\n" + existingSummary + "\n\n" +
		"New messages to merge into summary (JSON array of {role,content}):\n" + string(js)

	ms := []llm.Message{
		{Role: llm.RoleSystem, Content: sys},
		{Role: llm.RoleUser, Content: user},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resp, err := llm.RequestLLM(cli, ctx, model, ms, nil)
	if err != nil {
		return "", err
	}
	if resp.Content == "" {
		return "", errors.New("empty summarization response")
	}
	return resp.Content, nil
}

func (br *BrainMemory) GetContentSize(start int) int {
	return llm.TurnsSize(br.memory[start:])
}

func (br *BrainMemory) AddKeyMessage(key string, value interface{}, isAppend bool) {
	br.mu.Lock()
	defer br.mu.Unlock()
	if isAppend {
		existing, exists := br.keyMessage[key]
		if !exists {
			br.keyMessage[key] = []interface{}{value}
		}
		br.keyMessage[key] = append(existing, value)
	} else {
		br.keyMessage[key] = []interface{}{value}
	}
}

func (br *BrainMemory) SetSystemPrompt(x string) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.systemPrompt = x
}

func (br *BrainMemory) SetEnvInfo(xJson string) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.envInfoJson = xJson
}
func (br *BrainMemory) SetExploitIdeaList(xJson string) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.exploitIdeaListJson = xJson
}
func (br *BrainMemory) SetExploitChainList(xJson string) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.exploitChainListJson = xJson
}

func (br *BrainMemory) SetContainerListInfo(xJson string) {
	br.mu.Lock()
	defer br.mu.Unlock()
	br.containerInfoList = xJson
}

func (br *BrainMemory) GetMsgSize() int {
	br.mu.RLock()
	defer br.mu.RUnlock()
	return br.msgSize
}

func (br *BrainMemory) GetContext() []llm.Message {
	br.mu.Lock()
	defer br.mu.Unlock()
	if len(br.systemPrompt) == 0 {
		br.systemPrompt = systemPrompt
	}
	flat := llm.FlattenTurns(br.memory)
	messages := make([]llm.Message, 0, len(flat)+3)
	messages = append(messages, llm.Message{
		Role:    llm.RoleSystem,
		Content: br.systemPrompt,
	})
	userPrompt := ""
	if br.taskContent != "" {
		// agent_runtime: truncate RUNTask/RUNSummary for agents that have already been shown to LLM.
		agentRuntime := br.agentRuntimeInfoJson
		activeIDs := make([]string, 0)
		doneIDs := make([]string, 0)
		if agentRuntime != "" {
			if br.seenAgentRuntime == nil {
				br.seenAgentRuntime = map[string]bool{}
			}
			var runs []map[string]interface{}
			if err := json.Unmarshal([]byte(agentRuntime), &runs); err == nil {
				for _, one := range runs {
					id, _ := one["AgentID"].(string)
					st := ""
					if s, ok := one["RunState"].(string); ok {
						st = s
					}
					stLower := strings.ToLower(strings.TrimSpace(st))
					if id != "" {
						if stLower == "done" || stLower == "completed" || stLower == "success" {
							doneIDs = append(doneIDs, id)
						} else {
							activeIDs = append(activeIDs, id)
						}
					}
				}

				truncate5120 := func(s string) string {
					b := []byte(s)
					if len(b) > 5120 {
						return string(b[:5120]) + " ...[truncated]"
					}
					return s
				}
				for _, one := range runs {
					id, _ := one["AgentID"].(string)
					if id == "" {
						continue
					}
					if br.seenAgentRuntime[id] {
						if v, ok := one["RUNTask"].(string); ok {
							one["RUNTask"] = truncate5120(v)
						}
						if v, ok := one["RUNSummary"].(string); ok {
							one["RUNSummary"] = truncate5120(v)
						}
					}
					// Mark as seen after first inclusion in the panel context.
					br.seenAgentRuntime[id] = true
				}
				if js, err := json.Marshal(runs); err == nil {
					agentRuntime = string(js)
				}
			}
		}

		js1, _ := json.Marshal(br.keyMessage)
		env := "Environment information not generated, Ops-type Agent not running or task not yet completed."
		if br.envInfoJson != "" {
			env = br.envInfoJson
		}
		exploitIdeaList := "ExploitIdea not generated. No analysis-type Agent has been launched or the Agent has not produced any results yet."
		if br.exploitIdeaListJson != "" {
			type exploitIdeaBrief struct {
				ExploitIdeaId string `json:"exploitIdeaId"`
				Title         string `json:"title,omitempty"`
				State         string `json:"state"`
				Harm          string `json:"harm"`
			}
			var raw []map[string]interface{}
			if err := json.Unmarshal([]byte(br.exploitIdeaListJson), &raw); err == nil {
				briefs := make([]exploitIdeaBrief, 0, len(raw))
				for _, item := range raw {
					b := exploitIdeaBrief{}
					if v, ok := item["exploitIdeaId"].(string); ok {
						b.ExploitIdeaId = v
					}
					if v, ok := item["state"].(string); ok {
						b.State = v
					}
					if v, ok := item["harm"].(string); ok {
						b.Harm = v
					}
					if ep, ok := item["exploit_point"].(map[string]interface{}); ok {
						if t, ok := ep["title"].(string); ok {
							b.Title = t
						}
					}
					briefs = append(briefs, b)
				}
				brief, _ := json.Marshal(briefs)
				exploitIdeaList = string(brief) + "\n[Notice] Use Tool-GetExploitIdeaByIdTool with exploitIdeaId to view full details."
			} else {
				exploitIdeaList = br.exploitIdeaListJson
			}
		}
		cinfo := "No containers are currently running."
		if br.containerInfoList != "" {
			cinfo = br.containerInfoList
		}
		ecinfo := "There is currently no attack chain information."
		if br.exploitChainListJson != "" {
			// Compact chain list: only keep chain-level state + idea IDs, not full nested ideas.
			var chains []map[string]interface{}
			if err := json.Unmarshal([]byte(br.exploitChainListJson), &chains); err == nil {
				type chainBrief struct {
					Idea     string   `json:"idea"`
					State    string   `json:"state"`
					IdeaIDs  []string `json:"exploit_idea_ids"`
					Evidence string   `json:"evidence,omitempty"`
				}
				briefs := make([]chainBrief, 0, len(chains))
				for _, ch := range chains {
					cb := chainBrief{}
					if v, ok := ch["idea"].(string); ok {
						cb.Idea = v
					}
					if v, ok := ch["state"].(string); ok {
						cb.State = v
					}
					if v, ok := ch["evidence"].(map[string]interface{}); ok {
						if e, ok := v["Evidence"].(string); ok && e != "" {
							if len(e) > 200 {
								e = e[:200] + "...[truncated]"
							}
							cb.Evidence = e
						}
					}
					if ideas, ok := ch["exploit_idea"].([]interface{}); ok {
						for _, idea := range ideas {
							if m, ok := idea.(map[string]interface{}); ok {
								if id, ok := m["exploitIdeaId"].(string); ok {
									cb.IdeaIDs = append(cb.IdeaIDs, id)
								}
							}
						}
					}
					briefs = append(briefs, cb)
				}
				if js, err := json.Marshal(briefs); err == nil {
					ecinfo = string(js)
				}
			} else {
				ecinfo = br.exploitChainListJson
			}
		}

		envReady := br.envInfoJson != ""
		envWarning := ""
		if envReady {
			envWarning = "\n[WARNING] ENV_READY=true — The test environment has ALREADY been set up successfully. Do NOT schedule any Ops agent to build the environment again. See the 'env' section below for details.\n"
		}
		agentSummary := fmt.Sprintf(
			"agent_runtime_summary:\nNO_AGENT_RUNNING_FLAG: %t\nACTIVE_AGENT_COUNT: %d\nACTIVE_AGENT_IDS: %s\nDONE_AGENT_COUNT: %d\nDONE_AGENT_IDS: %s\nENV_READY: %t%s\n[IMPORTANT] Do NOT infer agent status by skimming agent_runtime JSON. Always trust agent_runtime_summary above.\n",
			br.noAgentRunning,
			len(activeIDs),
			strings.Join(activeIDs, ","),
			len(doneIDs),
			strings.Join(doneIDs, ","),
			envReady,
			envWarning,
		)

		// Build the content-only prompt for hash (excludes current_time so time changes don't trigger unnecessary wakeups).
		contentPrompt := fmt.Sprintf("Overall Goal:\n%s\n\n%s\nagent_runtime:\n%s\n\nenv:\n%s\n\nexploit_idea_list:\n%s\n\ncontainers:\n%s\n\nexploit_chain_list:\n%s\n\nkey_info:\n%s\n", br.taskContent, agentSummary, agentRuntime, env, exploitIdeaList, cinfo, ecinfo, string(js1))
		if br.noAgentRunning {
			contentPrompt = contentPrompt + "\n[ALL AGENTS IDLE] No digital human is currently running. Review the exploit_idea_list and exploit_chain_list above. Ask yourself:\n1. Have I covered enough vulnerability categories for this project's tech stack?\n2. Are there code modules or attack surfaces I haven't explored yet?\n3. Are there exploitIdeas that need verification or combination into chains?\nIf the answer to any of these is YES, schedule more agents NOW. Only call Tool-FinishTask if you are confident the analysis is thorough. If you do not initiate any tool calls, the system will consider the task ended."
		}
		// Include memory length in hash so new chat messages always change the hash.
		hashInput := fmt.Sprintf("mem_len:%d\n%s", len(br.memory), contentPrompt)
		hash := fmt.Sprintf("%x", md5.Sum([]byte(hashInput)))
		// Prepend current_time to the actual prompt sent to the brain.
		userPrompt = fmt.Sprintf("current_time: %s\n\n%s", time.Now().Format("2006-01-02 15:04:05"), contentPrompt)

		if br.userPromptHash != hash {
			// Build precise changed_sections by comparing per-section hashes.
			if br.lastPanelSectionHash == nil {
				br.lastPanelSectionHash = map[string]string{}
			}
			if br.lastPanelSectionContent == nil {
				br.lastPanelSectionContent = map[string]string{}
			}
			currentSectionHash := map[string]string{
				"agent_runtime":      fmt.Sprintf("%x", md5.Sum([]byte(agentRuntime))),
				"env":                fmt.Sprintf("%x", md5.Sum([]byte(env))),
				"exploit_idea_list":  fmt.Sprintf("%x", md5.Sum([]byte(exploitIdeaList))),
				"containers":         fmt.Sprintf("%x", md5.Sum([]byte(cinfo))),
				"exploit_chain_list": fmt.Sprintf("%x", md5.Sum([]byte(ecinfo))),
				"key_info":           fmt.Sprintf("%x", md5.Sum([]byte(string(js1)))),
			}
			currentSectionContent := map[string]string{
				"agent_runtime":      agentRuntime,
				"env":                env,
				"exploit_idea_list":  exploitIdeaList,
				"containers":         cinfo,
				"exploit_chain_list": ecinfo,
				"key_info":           string(js1),
			}

			changed := make([]string, 0, 6)
			sectionOrder := []string{"agent_runtime", "exploit_idea_list", "exploit_chain_list", "env", "containers", "key_info"}
			for _, k := range sectionOrder {
				prev, ok := br.lastPanelSectionHash[k]
				if !ok || prev != currentSectionHash[k] {
					changed = append(changed, k)
				}
			}
			changedJSON, _ := json.Marshal(changed)
			delta := fmt.Sprintf("[StatusPanel Delta]\nchanged_sections=%s\n", string(changedJSON))

			br.lastUserPrompt = userPrompt
			br.lastPanelSectionHash = currentSectionHash
			br.lastPanelSectionContent = currentSectionContent
			userPrompt = delta + "\n" + userPrompt
			//misc.Debug("状态面板变动：\n%s", userPrompt)
			br.userPromptHash = hash
		}
	} else {
		return nil
	}
	messages = append(messages, llm.Message{
		Role:    llm.RoleUser,
		Content: userPrompt,
	})
	messages = append(messages, flat...)
	messages = llm.SanitizeToolCallMessages(messages)

	// Hard limit: if total tokens exceed maxHistory, drop oldest conversation
	// messages (index 2+) until under limit. Keeps system prompt (0) and status panel (1).
	if br.maxHistory > 0 {
		for llm.CountMessagesTokens(messages) > br.maxHistory && len(messages) > 3 {
			dropEnd := 3
			if messages[2].Role == llm.RoleAssistant && len(messages[2].ToolCalls) > 0 {
				ids := make(map[string]bool)
				for _, tc := range messages[2].ToolCalls {
					ids[tc.ID] = true
				}
				for dropEnd < len(messages) && messages[dropEnd].Role == llm.RoleTool && ids[messages[dropEnd].ToolCallID] {
					dropEnd++
				}
			}
			messages = append(messages[:2], messages[dropEnd:]...)
		}
	}

	// Re-surface unanswered user chat messages at the end of the message list.
	// Without this, chat messages get buried under tool calls and eventually
	// compressed away, causing the brain to silently ignore user input.
	var unanswered []string
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == llm.RoleAssistant && strings.Contains(m.Content, "<UserMessage>") {
			// Found an assistant reply with UserMessage — all earlier chat messages are answered.
			break
		}
		if m.Role == llm.RoleUser && (strings.Contains(m.Content, "[Team Chat from ") || strings.Contains(m.Content, "[BrainMessage from ")) {
			unanswered = append(unanswered, m.Content)
		}
	}
	if len(unanswered) > 0 {
		// Reverse to preserve chronological order.
		for i, j := 0, len(unanswered)-1; i < j; i, j = i+1, j-1 {
			unanswered[i], unanswered[j] = unanswered[j], unanswered[i]
		}
		reminder := "[REMINDER] The user sent you the following chat message(s) that you have NOT yet replied to. You MUST reply using <UserMessage>your reply</UserMessage> NOW:\n"
		for _, msg := range unanswered {
			reminder += "\n" + msg + "\n"
		}
		messages = append(messages, llm.Message{
			Role:    llm.RoleUser,
			Content: reminder,
		})
	}

	br.msgSize = llm.CountMessagesTokens(messages)

	// Compute per-section token breakdown for UI display.
	sections := []ContextBreakdownSection{
		{Name: "system_prompt", Tokens: llm.CountTokens(br.systemPrompt)},
		{Name: "task_goal", Tokens: llm.CountTokens(br.taskContent)},
	}
	if br.lastPanelSectionContent != nil {
		for _, k := range []string{"agent_runtime", "exploit_idea_list", "exploit_chain_list", "env", "containers", "key_info"} {
			if c, ok := br.lastPanelSectionContent[k]; ok {
				sections = append(sections, ContextBreakdownSection{Name: k, Tokens: llm.CountTokens(c)})
			}
		}
	}
	// Conversation history tokens = total - system prompt - status panel
	panelTokens := 0
	for _, s := range sections {
		panelTokens += s.Tokens
	}
	historyTokens := br.msgSize - panelTokens
	if historyTokens < 0 {
		historyTokens = 0
	}
	sections = append(sections, ContextBreakdownSection{Name: "conversation_history", Tokens: historyTokens})
	br.contextBreakdown = ContextBreakdown{
		Sections:   sections,
		Total:      br.msgSize,
		MaxContext: br.maxHistory,
	}

	return messages
}

// GetContextBreakdown returns the latest per-section token breakdown.
func (br *BrainMemory) GetContextBreakdown() ContextBreakdown {
	br.mu.RLock()
	defer br.mu.RUnlock()
	return br.contextBreakdown
}
