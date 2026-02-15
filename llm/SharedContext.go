package llm

import (
	"strings"
	"sync"
)

// 用于多个Agent同时运行，既能保证单个Agent上下文不被污染，又能保证关键信息共享，并且允许同时为多个Agent的上下文添加重要记忆点
type SharedContext struct {
	Contexts     map[string]*ContextManager
	mu           sync.Mutex
	eventHandler func(string, string, int)
}

func NewSharedContext() *SharedContext {
	return &SharedContext{Contexts: make(map[string]*ContextManager)}
}
func (cm *SharedContext) AddContextManager(id string, contextManager *ContextManager) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if cm.eventHandler != nil {
		contextManager.SetEventHandler(cm.eventHandler)
	}
	cm.Contexts[id] = contextManager
}

// TODO
//func (cm *SharedContext) SaveMemoryToFile(filename string) error {
//	memoryInfoJson, _ := json.Marshal(cm)
//	err := os.WriteFile(filename, memoryInfoJson, 0644)
//	return err
//}
//
//func (cm *SharedContext) LoadMemoryByFile(filename string) error {
//	content, err := os.ReadFile(filename)
//	if err != nil {
//		return err
//	}
//	err = json.Unmarshal(content, &cm)
//	if err != nil {
//		return err
//	}
//	return nil
//}

func (cm *SharedContext) SetEventHandler(f func(string, string, int)) {
	cm.eventHandler = f
}

func (cm *SharedContext) SetKeyMessage(env map[string][]interface{}, id string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	c, e := cm.Contexts[id]
	if e {
		c.SetKeyMessage(env, id)
	} else {
		for _, contextManager := range cm.Contexts {
			contextManager.SetKeyMessage(env, id)
		}
	}
}

func (cm *SharedContext) GetKeyMessage(id string) map[string][]interface{} {
	c, e := cm.Contexts[id]
	if e {
		return c.GetKeyMessage(id)
	} else {
		for _, contextManager := range cm.Contexts {
			return contextManager.GetKeyMessage(id)
		}
	}
	return nil
}

func (cm *SharedContext) GetType() string {
	return "SharedContext"
}

func (cm *SharedContext) AddMessage(x *MessageX) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if x.Shared {
		for _, context := range cm.Contexts {
			context.AddMessage(x)
		}
	} else {
		c, f := cm.Contexts[x.ContextId]
		if !f {
			panic("Context not found in contextManager")
		}
		c.AddMessage(x)
	}
}

func (cm *SharedContext) SetTaskList(x *TaskListX) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	c, f := cm.Contexts[x.ContextId]
	if !f {
		panic("Context not found in contextManager")
	}
	c.SetTaskList(x)
}

func (cm *SharedContext) AddKeyMessage(x *EnvMessageX) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	if x.NotShared {
		c, f := cm.Contexts[x.ContextId]
		if !f {
			panic("Context not found in contextManager")
		}
		c.AddKeyMessage(x)
	} else {
		for _, context := range cm.Contexts {
			context.AddKeyMessage(x)
		}
	}
	if !x.Submit {
		cm.eventHandler("记忆体", x.jsonEncode(), 0)
		x.Submit = true
	}
}

func (cm *SharedContext) SetSystemPrompt(x *SystemPromptX) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	c, f := cm.Contexts[x.ContextId]
	if !f {
		panic("Context not found in contextManager")
	}
	c.SetSystemPrompt(x)
}

func (cm *SharedContext) SetTeamMessageHandler(f func(senderName string, msg string)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, context := range cm.Contexts {
		context.SetTeamMessageHandler(f)
	}
}

func (cm *SharedContext) SetUserMessageHandler(f func(senderCtxId string, msg string)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, context := range cm.Contexts {
		context.SetUserMessageHandler(f)
	}
}

func (cm *SharedContext) SetBrainMessageHandler(f func(senderCtxId string, msg string)) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, context := range cm.Contexts {
		context.SetBrainMessageHandler(f)
	}
}

func (cm *SharedContext) SetExtraSystemPrompt(prompt string, contextId string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	c, f := cm.Contexts[contextId]
	if !f {
		panic("Context not found in contextManager")
	}
	c.SetExtraSystemPrompt(prompt, contextId)
}

func (cm *SharedContext) GetContext(id string) []Message {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	c, f := cm.Contexts[id]
	if !f {
		panic("Context not found in contextManager")
	}
	return c.GetContext(id)
}
func (cm *SharedContext) GetMsgSize(id string) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	c, f := cm.Contexts[id]
	if !f {
		panic("Context not found in contextManager")
	}
	return c.GetMsgSize(id)
}
func (cm *SharedContext) GetMaxHistory() int {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, c := range cm.Contexts {
		return c.GetMaxHistory()
	}
	return 0
}

// CompressIfNeeded delegates compression to all underlying context managers.
func (cm *SharedContext) CompressIfNeeded(cli Client, model string) error {
	cm.mu.Lock()
	// Collect references under lock, then compress outside lock to avoid holding it during LLM call.
	managers := make([]*ContextManager, 0, len(cm.Contexts))
	for _, ctx := range cm.Contexts {
		managers = append(managers, ctx)
	}
	cm.mu.Unlock()
	for _, mgr := range managers {
		if err := mgr.CompressIfNeeded(cli, model); err != nil {
			return err
		}
	}
	return nil
}

// ResetMemoryWithSummary delegates memory reset to all underlying context managers.
func (cm *SharedContext) ResetMemoryWithSummary(cli Client, model string) error {
	cm.mu.Lock()
	managers := make([]*ContextManager, 0, len(cm.Contexts))
	for _, ctx := range cm.Contexts {
		managers = append(managers, ctx)
	}
	cm.mu.Unlock()
	for _, mgr := range managers {
		if err := mgr.ResetMemoryWithSummary(cli, model); err != nil {
			return err
		}
	}
	return nil
}

// AckPendingUserMessage is a no-op now that injection is synchronized via
// LockForLLM/UnlockForLLM. Kept for interface compatibility.
func (cm *SharedContext) AckPendingUserMessage() {}

// LockForLLM locks all underlying context managers for LLM request.
func (cm *SharedContext) LockForLLM() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, ctx := range cm.Contexts {
		ctx.LockForLLM()
	}
}

// UnlockForLLM unlocks all underlying context managers after LLM request.
func (cm *SharedContext) UnlockForLLM() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, ctx := range cm.Contexts {
		ctx.UnlockForLLM()
	}
}

// HasPendingUserMessage returns true if any context manager has a pending user message.
func (cm *SharedContext) HasPendingUserMessage() bool {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	for _, ctx := range cm.Contexts {
		if ctx.HasPendingUserMessage() {
			return true
		}
	}
	return false
}

// PopPendingUserMessages pops pending user messages from all sub-contexts.
func (cm *SharedContext) PopPendingUserMessages() []Message {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	var result []Message
	for _, ctx := range cm.Contexts {
		result = append(result, ctx.PopPendingUserMessages()...)
	}
	return result
}

func getLastLine(text string) string {
	lines := strings.FieldsFunc(text, func(r rune) bool {
		return r == '\n' || r == '\r'
	})

	if len(lines) == 0 {
		return ""
	}

	return lines[len(lines)-1]
}
