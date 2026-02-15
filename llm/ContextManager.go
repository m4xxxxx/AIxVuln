package llm

import (
	"AIxVuln/misc"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ContextManager struct {
	systemPrompt        string
	extraSystemPrompt   string
	memory              []Turn
	mu                  sync.RWMutex
	envMessage          map[string][]interface{} // 重要的信息，这个信息永远不会被覆盖
	maxHistory          int                      // 历史对话记录最大token数
	compressedSummary   string                   // LLM-generated summary of older turns
	taskList            []map[string]string
	msgSize             int // 消息大小
	llmBusy             bool       // true while agent is in an LLM request
	llmCond             *sync.Cond // signaled when llmBusy becomes false
	waitingWriters      int        // number of AddMessage(user) goroutines waiting to inject
	writerDone          *sync.Cond // signaled when a waiting writer finishes injection
	eventHandler        func(string, string, int)
	teamMessageHandler  func(senderName string, msg string)
	userMessageHandler  func(senderCtxId string, msg string)
	brainMessageHandler func(senderCtxId string, msg string)
}

func NewContextManager(configSections ...string) *ContextManager {
	cm := &ContextManager{
		memory:     make([]Turn, 0),
		maxHistory: misc.GetMaxContext(configSections...),
		envMessage: make(map[string][]interface{}),
	}
	cm.llmCond = sync.NewCond(&cm.mu)
	cm.writerDone = sync.NewCond(&cm.mu)
	return cm
}
func (cm *ContextManager) AddContextManager(id string, contextManager *ContextManager) {

}
func (cm *ContextManager) SaveMemoryToFile(filename string) error {
	memoryInfoJson, _ := json.Marshal(cm)
	err := os.WriteFile(filename, memoryInfoJson, 0644)
	return err
}

func (cm *ContextManager) SetEventHandler(f func(string, string, int)) {
	cm.eventHandler = f
}

func (cm *ContextManager) LoadMemoryFromFile(filename string) error {
	content, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	var saved ContextManager
	if err := json.Unmarshal(content, &saved); err != nil {
		return err
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.systemPrompt = saved.systemPrompt
	cm.memory = saved.memory
	cm.envMessage = saved.envMessage
	cm.maxHistory = saved.maxHistory
	cm.taskList = saved.taskList
	cm.msgSize = saved.msgSize
	return nil
}

func (cm *ContextManager) SetMemory(memory []Message) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.memory = BuildTurns(memory)
}
func (cm *ContextManager) SetKeyMessage(env map[string][]interface{}, id string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.envMessage = env
}
func (cm *ContextManager) GetKeyMessage(id string) map[string][]interface{} {
	return cm.envMessage
}

// 清除历史记忆，但是保留关键信息
func (cm *ContextManager) ClearMemory() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.memory = make([]Turn, 0)
}

func (cm *ContextManager) GetType() string {
	return "ContextManager"
}
func (cm *ContextManager) GetMsgSize(id string) int {
	return cm.msgSize
}
func (cm *ContextManager) GetMaxHistory() int {
	return cm.maxHistory
}

func (cm *ContextManager) AddMessage(x *MessageX) {
	if len(x.Msg.Content) > misc.GetMessageMaximum() {
		x.Msg.Content = x.Msg.Content[:misc.GetMessageMaximum()] + " ...... (The text exceeds the maximum length of " + strconv.Itoa(misc.GetMessageMaximum()) + " characters and cannot be sent to the LLM—)."
	}
	cm.mu.Lock()

	// If this is a user message (injected via @-mention) and the agent is
	// currently in an LLM request, block until the request finishes so the
	// message is guaranteed to be included in the next LLM context.
	if x.Msg.Role == RoleUser && cm.llmBusy {
		cm.waitingWriters++
		misc.Debug("AddMessage: user message blocked, waiting for LLM to finish (waitingWriters=%d)", cm.waitingWriters)
		for cm.llmBusy {
			cm.llmCond.Wait()
		}
		misc.Debug("AddMessage: LLM finished, injecting user message now")
		// Note: waitingWriters is decremented after the message is written (below).
	}

	// Tool messages are appended to the last turn (which should be an assistant
	// turn with tool_calls). All other messages start a new turn.
	if x.Msg.Role == RoleTool && len(cm.memory) > 0 {
		last := &cm.memory[len(cm.memory)-1]
		if last.Role() == RoleAssistant && last.HasToolCalls() {
			last.Messages = append(last.Messages, x.Msg)
		} else {
			cm.memory = append(cm.memory, Turn{Messages: []Message{x.Msg}})
		}
	} else {
		cm.memory = append(cm.memory, Turn{Messages: []Message{x.Msg}})
	}

	// If this writer was waiting, decrement counter and signal UnlockForLLM.
	if x.Msg.Role == RoleUser && cm.waitingWriters > 0 {
		cm.waitingWriters--
		if cm.waitingWriters == 0 {
			cm.writerDone.Broadcast()
		}
	}

	// Truncate old tool results inside older turns to save space.
	cm.trimOldToolTurns()

	// NOTE: old turns are no longer simply truncated here.
	// Instead, call CompressIfNeeded() from the agent loop to
	// summarize old turns via LLM before discarding them.

	// Detect <TeamMessage>, <UserMessage>, <BrainMessage> in assistant replies.
	var teamHandler func(string, string)
	var userHandler func(string, string)
	var brainHandler func(string, string)
	var teamMsg, userMsg, brainMsg string
	if x.Msg.Role == RoleAssistant {
		teamMsg = ExtractTag(x.Msg.Content, "TeamMessage")
		if teamMsg != "" && cm.teamMessageHandler != nil {
			teamHandler = cm.teamMessageHandler
		}
		userMsg = ExtractTag(x.Msg.Content, "UserMessage")
		if userMsg != "" && cm.userMessageHandler != nil {
			userHandler = cm.userMessageHandler
		}
		brainMsg = ExtractTag(x.Msg.Content, "BrainMessage")
		if brainMsg != "" && cm.brainMessageHandler != nil {
			brainHandler = cm.brainMessageHandler
		}
	}
	cm.mu.Unlock()

	// Call handlers outside the lock to avoid deadlock.
	if teamHandler != nil {
		teamHandler(x.ContextId, teamMsg)
	}
	if userHandler != nil {
		userHandler(x.ContextId, userMsg)
	}
	if brainHandler != nil {
		brainHandler(x.ContextId, brainMsg)
	}
}

// ExtractTag extracts content between <tagName>...</tagName> from text.
// Also handles malformed tags where the opening '<' is missing (e.g. "UserMessage>..." instead of "<UserMessage>...").
func ExtractTag(text string, tagName string) string {
	open := "<" + tagName + ">"
	close := "</" + tagName + ">"
	start := strings.Index(text, open)
	if start < 0 {
		// Fallback: try without leading '<' (LLM sometimes omits it).
		openBroken := tagName + ">"
		start = strings.Index(text, openBroken)
		if start < 0 {
			return ""
		}
		start += len(openBroken)
	} else {
		start += len(open)
	}
	// Try standard closing tag first, then broken closing tag without '<'.
	end := strings.Index(text[start:], close)
	if end < 0 {
		closeBroken := "/" + tagName + ">"
		end = strings.Index(text[start:], closeBroken)
	}
	if end < 0 {
		// No closing tag — take everything after the opening tag.
		return strings.TrimSpace(text[start:])
	}
	return strings.TrimSpace(text[start : start+end])
}

func (cm *ContextManager) SetTeamMessageHandler(f func(senderName string, msg string)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.teamMessageHandler = f
}

func (cm *ContextManager) SetUserMessageHandler(f func(senderCtxId string, msg string)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.userMessageHandler = f
}

func (cm *ContextManager) SetBrainMessageHandler(f func(senderCtxId string, msg string)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.brainMessageHandler = f
}

func (cm *ContextManager) GetContentSize(start int) int {
	return TurnsSize(cm.memory[start:])
}

func (cm *ContextManager) SetTaskList(x *TaskListX) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.taskList = x.TaskList
}

func (cm *ContextManager) AddKeyMessage(x *EnvMessageX) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if x.AppendEnv {
		existing, exists := cm.envMessage[x.Key]
		if !exists {
			cm.envMessage[x.Key] = []interface{}{x.Content}
		}
		cm.envMessage[x.Key] = append(existing, x.Content)
	} else {
		cm.envMessage[x.Key] = []interface{}{x.Content}
	}
	if !x.Submit {
		cm.eventHandler("记忆体", x.jsonEncode(), 0)
		x.Submit = true
	}
}

func (cm *ContextManager) SetSystemPrompt(x *SystemPromptX) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.systemPrompt = x.SystemPrompt
}

func (cm *ContextManager) SetExtraSystemPrompt(prompt string, contextId string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.extraSystemPrompt = prompt
}

func (cm *ContextManager) GetContext(id string) []Message {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	if len(cm.systemPrompt) == 0 {
		return nil
	}
	systemPrompt := cm.systemPrompt
	if strings.TrimSpace(cm.extraSystemPrompt) != "" {
		systemPrompt = systemPrompt + "\n\n" + cm.extraSystemPrompt
	}
	flat := FlattenTurns(cm.memory)
	messages := make([]Message, 0, len(flat)+3)
	messages = append(messages, Message{
		Role:    RoleSystem,
		Content: systemPrompt,
	})
	userPrompt := ""
	if cm.taskList != nil {
		if len(cm.taskList) == 1 {
			js1, _ := json.Marshal(cm.envMessage)
			userPrompt = (cm.taskList)[0]["TaskContent"] + "\nthis is the recorded useful information: \n" + string(js1)
		} else {
			js, _ := json.Marshal(cm.taskList)
			js1, _ := json.Marshal(cm.envMessage)
			userPrompt = "The current subtask list and their statuses are as follows: \n" + string(js) + "\nYou need to call the TaskListTool to confirm after completing or discarding each subtask, after which the subtask list will automatically update.\nthis is the recorded useful information: \n" + string(js1)
		}
	} else {
		return nil
	}
	messages = append(messages, Message{
		Role:    RoleUser,
		Content: userPrompt,
	})
	messages = append(messages, flat...)
	messages = SanitizeToolCallMessages(messages)

	// Hard limit: if total tokens exceed maxHistory, drop oldest conversation
	// messages (index 2+) until under limit. Keeps system prompt (0) and task prompt (1).
	if cm.maxHistory > 0 {
		for CountMessagesTokens(messages) > cm.maxHistory && len(messages) > 3 {
			// Remove the oldest conversation message (index 2).
			// Must respect tool-call pairing: if msg[2] is an assistant with tool_calls,
			// drop it and all following tool-result messages that belong to it.
			dropEnd := 3 // drop at least one message
			if messages[2].Role == RoleAssistant && len(messages[2].ToolCalls) > 0 {
				ids := make(map[string]bool)
				for _, tc := range messages[2].ToolCalls {
					ids[tc.ID] = true
				}
				for dropEnd < len(messages) && messages[dropEnd].Role == RoleTool && ids[messages[dropEnd].ToolCallID] {
					dropEnd++
				}
			}
			messages = append(messages[:2], messages[dropEnd:]...)
		}
	}

	cm.msgSize = CountMessagesTokens(messages)
	return messages
}

// trimOldToolTurns truncates tool result content in older assistant turns
// to save space. Only the 3 most recent assistant-with-tools turns keep
// full tool results; older ones get truncated to 512 bytes.
func (cm *ContextManager) trimOldToolTurns() {
	const maxToolContent = 512
	toolTurnCount := 0
	for i := len(cm.memory) - 1; i >= 0; i-- {
		t := &cm.memory[i]
		if t.Role() == RoleAssistant && t.HasToolCalls() {
			toolTurnCount++
			if toolTurnCount > 2 {
				// Truncate tool results in this older turn.
				for j := 1; j < len(t.Messages); j++ {
					if t.Messages[j].Role == RoleTool && len(t.Messages[j].Content) > maxToolContent {
						t.Messages[j].Content = truncateToolContent(t.Messages[j].Content, maxToolContent)
					}
				}
			}
		}
	}
}

// HasPendingUserMessage returns true if there is a user turn after the last
// assistant turn in memory. Since user message injection is now blocked
// during LLM requests (via LockForLLM/UnlockForLLM), there is no race
// condition — any user turn found after the last assistant turn is
// guaranteed to not have been seen by the LLM yet.
func (cm *ContextManager) HasPendingUserMessage() bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	for i := len(cm.memory) - 1; i >= 0; i-- {
		role := cm.memory[i].Role()
		if role == RoleAssistant {
			return false
		}
		if role == RoleUser {
			return true
		}
	}
	return false
}

// AckPendingUserMessage is a no-op now that injection is synchronized via
// LockForLLM/UnlockForLLM. Kept for interface compatibility.
func (cm *ContextManager) AckPendingUserMessage() {}

// PopPendingUserMessages removes and returns all pending user messages
// (turns after the last assistant turn). This is used to extract @-mention
// messages without polluting the agent's real memory.
func (cm *ContextManager) PopPendingUserMessages() []Message {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	var result []Message
	// Walk backwards to find where pending user turns start.
	cutIdx := len(cm.memory)
	for i := len(cm.memory) - 1; i >= 0; i-- {
		role := cm.memory[i].Role()
		if role == RoleUser {
			cutIdx = i
		} else {
			break
		}
	}
	if cutIdx >= len(cm.memory) {
		return nil
	}
	for _, t := range cm.memory[cutIdx:] {
		result = append(result, FlattenTurns([]Turn{t})...)
	}
	cm.memory = cm.memory[:cutIdx]
	return result
}

// LockForLLM marks the agent as busy with an LLM request. Any subsequent
// AddMessage calls with a user-role message will block until UnlockForLLM
// is called.
func (cm *ContextManager) LockForLLM() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.llmBusy = true
}

// UnlockForLLM marks the LLM request as finished and wakes up any blocked
// AddMessage callers so they can inject their user messages before the next
// GetContext call.
func (cm *ContextManager) UnlockForLLM() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.llmBusy = false
	cm.llmCond.Broadcast()
	// Wait for all blocked writers to finish injecting their messages
	// before returning, so the next GetContext() includes them.
	for cm.waitingWriters > 0 {
		cm.writerDone.Wait()
	}
}

const contextManagerSummaryPrefix = "[Memory Summary]"

func (cm *ContextManager) shouldCompressLocked() bool {
	if cm.maxHistory <= 0 {
		return false
	}
	return TurnsSize(cm.memory) > cm.maxHistory
}

func (cm *ContextManager) snapshotCompressionCandidateLocked() (string, []Message, []Turn, bool) {
	if len(cm.memory) <= 2 {
		return "", nil, nil, false
	}
	budget := cm.maxHistory / 2
	if budget < 4096 {
		budget = 4096
	}
	kept := 0
	size := 0
	for i := len(cm.memory) - 1; i >= 0; i-- {
		s := cm.memory[i].Size()
		if size+s > budget && kept >= 4 {
			break
		}
		size += s
		kept++
	}
	if kept >= len(cm.memory) {
		return "", nil, nil, false
	}
	splitIdx := len(cm.memory) - kept
	oldFlat := FlattenTurns(cm.memory[:splitIdx])
	recentTurns := make([]Turn, kept)
	copy(recentTurns, cm.memory[splitIdx:])
	return cm.compressedSummary, oldFlat, recentTurns, true
}

func (cm *ContextManager) buildSummaryMessageLocked() Message {
	content := contextManagerSummaryPrefix + "\n(This summary was automatically generated by the system to compress earlier conversation history. It is NOT a message you wrote — treat it as reference context.)"
	if cm.compressedSummary != "" {
		content = content + "\n" + cm.compressedSummary
	}
	return Message{Role: RoleUser, Content: content}
}

// CompressIfNeeded checks whether the memory exceeds maxHistory and, if so,
// summarizes older turns via LLM and replaces them with a compact summary turn.
func (cm *ContextManager) CompressIfNeeded(cli Client, model string) error {
	if cli == nil {
		return errors.New("nil llm client")
	}

	cm.mu.Lock()
	need := cm.shouldCompressLocked()
	cm.mu.Unlock()
	if !need {
		return nil
	}

	var existingSummary string
	var oldMessages []Message
	var recentTurns []Turn
	var ok bool
	cm.mu.Lock()
	existingSummary, oldMessages, recentTurns, ok = cm.snapshotCompressionCandidateLocked()
	cm.mu.Unlock()
	if !ok {
		return nil
	}

	newSummary, err := summarizeMessagesWithLLM(cli, model, existingSummary, oldMessages)
	if err != nil {
		// Fallback: LLM compression failed. Force-drop old turns to prevent
		// unbounded memory growth. Keep only the recent turns.
		misc.Debug("CompressIfNeeded: LLM summarization failed (%s), falling back to hard truncation", err.Error())
		cm.mu.Lock()
		defer cm.mu.Unlock()
		// Re-snapshot in case memory changed while we were calling LLM.
		_, _, recentFallback, okFallback := cm.snapshotCompressionCandidateLocked()
		if okFallback {
			fallbackMsg := "[Memory Summary]\n(Automatic compression failed. Older conversation history has been discarded to stay within context limits.)"
			if cm.compressedSummary != "" {
				fallbackMsg += "\nPrevious summary:\n" + cm.compressedSummary
			}
			summaryTurn := Turn{Messages: []Message{{Role: RoleUser, Content: fallbackMsg}}}
			cm.memory = append([]Turn{summaryTurn}, recentFallback...)
		}
		return err
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.compressedSummary = newSummary
	summaryTurn := Turn{Messages: []Message{cm.buildSummaryMessageLocked()}}
	cm.memory = append([]Turn{summaryTurn}, recentTurns...)
	return nil
}

// summarizeMessagesWithLLM calls the LLM to produce a compact summary of messages.
func summarizeMessagesWithLLM(cli Client, model string, existingSummary string, msgs []Message) (string, error) {
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
		case RoleAssistant:
			limit = maxPerAssistant
		case RoleTool:
			limit = maxPerTool
		}
		if len(c) > limit {
			c = c[:limit] + " ...[truncated]"
		}
		if totalBytes+len(c) > maxPromptBytes {
			break
		}
		totalBytes += len(c)
		trimmed = append(trimmed, map[string]string{"role": m.Role, "content": c})
	}
	js, _ := json.Marshal(trimmed)

	sys := "You are a memory compression agent. Summarize the provided chat history into a concise, loss-minimizing memory.\n" +
		"Rules:\n" +
		"- Preserve: all decisions made, task assignments, discovered vulnerabilities and their IDs, exploit ideas/chains and their states, file paths, environment info (URLs/ports/credentials), tool call outcomes, and any invariants.\n" +
		"- For tool call results: keep the conclusion/outcome, discard verbose raw output.\n" +
		"- Keep it compact: use short sections and bullet points.\n" +
		"- Do NOT include code blocks.\n" +
		"- Output in plain text (Chinese is OK)."

	user := "Existing summary (may be empty):\n" + existingSummary + "\n\n" +
		"New messages to merge into summary (JSON array of {role,content}):\n" + string(js)

	ms := []Message{
		{Role: RoleSystem, Content: sys},
		{Role: RoleUser, Content: user},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resp, err := RequestLLM(cli, ctx, model, ms, nil)
	if err != nil {
		return "", err
	}
	if resp.Content == "" {
		return "", errors.New("empty summarization response")
	}
	return resp.Content, nil
}

const memoryResetPrefix = "[Previous Memory — Compressed Summary]\n" +
	"(The following is a compressed summary of your previous work sessions. " +
	"It was generated automatically by the system. Treat it as reference context " +
	"for your new task. You do NOT need to continue the previous task — a new task " +
	"will be assigned separately.)\n"

// ResetMemoryWithSummary compresses all existing memory into a concise summary
// via LLM, clears the conversation history, and injects the summary as a single
// user message so the agent retains key context (trial-and-error, findings,
// environment info, etc.) while starting fresh for a new task.
func (cm *ContextManager) ResetMemoryWithSummary(cli Client, model string) error {
	if cli == nil {
		return errors.New("nil llm client")
	}

	cm.mu.Lock()
	if len(cm.memory) == 0 {
		cm.mu.Unlock()
		return nil
	}
	allMessages := FlattenTurns(cm.memory)
	existingSummary := cm.compressedSummary
	cm.mu.Unlock()

	// Nothing to compress if there are no real messages.
	if len(allMessages) == 0 {
		return nil
	}

	newSummary, err := summarizeForReset(cli, model, existingSummary, allMessages)
	if err != nil {
		return fmt.Errorf("ResetMemoryWithSummary: summarize failed: %w", err)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.memory = make([]Turn, 0)
	cm.compressedSummary = ""
	if strings.TrimSpace(newSummary) != "" {
		summaryMsg := Message{Role: RoleUser, Content: memoryResetPrefix + newSummary}
		cm.memory = append(cm.memory, Turn{Messages: []Message{summaryMsg}})
	}
	return nil
}

// summarizeForReset calls the LLM to produce a compact summary of ALL messages,
// specifically tailored for a memory reset (new task assignment).
func summarizeForReset(cli Client, model string, existingSummary string, msgs []Message) (string, error) {
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
		case RoleAssistant:
			limit = maxPerAssistant
		case RoleTool:
			limit = maxPerTool
		}
		if len(c) > limit {
			c = c[:limit] + " ...[truncated]"
		}
		if totalBytes+len(c) > maxPromptBytes {
			break
		}
		totalBytes += len(c)
		trimmed = append(trimmed, map[string]string{"role": m.Role, "content": c})
	}
	js, _ := json.Marshal(trimmed)

	sys := "You are a memory compression agent. A digital human (AI agent) is about to receive a new task. " +
		"Compress the provided conversation history into a concise summary that the agent can use as reference.\n" +
		"Rules:\n" +
		"- Preserve: completed work and outcomes, discovered vulnerabilities and their IDs/states, " +
		"trial-and-error attempts (what worked, what failed and why), environment info (URLs/ports/credentials/cookies), " +
		"file paths, key decisions, and any important constraints learned.\n" +
		"- For tool call results: keep the conclusion/outcome, discard verbose raw output.\n" +
		"- Clearly separate: (1) what was accomplished, (2) what failed and why, (3) key environment/context info.\n" +
		"- Keep it compact: use short sections and bullet points.\n" +
		"- Do NOT include code blocks or raw HTTP responses.\n" +
		"- Output in plain text (Chinese preferred)."

	user := "Existing compressed summary (may be empty):\n" + existingSummary + "\n\n" +
		"Full conversation history to compress (JSON array of {role,content}):\n" + string(js)

	ms := []Message{
		{Role: RoleSystem, Content: sys},
		{Role: RoleUser, Content: user},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	resp, err := RequestLLM(cli, ctx, model, ms, nil)
	if err != nil {
		return "", err
	}
	if resp.Content == "" {
		return "", errors.New("empty summarization response")
	}
	return resp.Content, nil
}

func truncateToolContent(content string, maxLength int) string {
	if len(content) <= maxLength {
		return content
	}
	truncated := content[:maxLength]
	if lastSpace := strings.LastIndex(truncated, " "); lastSpace > maxLength/2 {
		truncated = content[:lastSpace]
	}
	return truncated + fmt.Sprintf(" ... [Historical message truncated, original length %d]", len(content))
}
