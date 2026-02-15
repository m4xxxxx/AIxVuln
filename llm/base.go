package llm

import (
	"encoding/json"
)

type Memory interface {
	AddMessage(x *MessageX)
	SetTaskList(x *TaskListX)
	AddKeyMessage(x *EnvMessageX)
	SetKeyMessage(map[string][]interface{}, string)
	GetKeyMessage(string) map[string][]interface{}
	SetSystemPrompt(x *SystemPromptX)
	SetExtraSystemPrompt(prompt string, contextId string)
	GetContext(x string) []Message
	GetType() string
	GetMsgSize(string2 string) int
	AddContextManager(id string, contextManager *ContextManager)
	SetEventHandler(f func(string, string, int))
	SetTeamMessageHandler(f func(senderName string, msg string))
	SetUserMessageHandler(f func(senderCtxId string, msg string))
	SetBrainMessageHandler(f func(senderCtxId string, msg string))
	HasPendingUserMessage() bool
	AckPendingUserMessage()
	PopPendingUserMessages() []Message
	LockForLLM()
	UnlockForLLM()
	GetMaxHistory() int
	CompressIfNeeded(cli Client, model string) error
	// ResetMemoryWithSummary compresses all existing memory into a single
	// summary message via LLM, clears the memory, and injects the summary
	// as a "previous memory" user message. Used before assigning a new task
	// to a persistent agent so it starts fresh but retains key context.
	ResetMemoryWithSummary(cli Client, model string) error
}

type MessageX struct {
	Msg       Message
	Shared    bool
	ContextId string
}

type TaskListX struct {
	TaskList  []map[string]string
	ContextId string
}
type EnvMessageX struct {
	Key       string `json:"key"`
	Content   any    `json:"content"`
	AppendEnv bool   `json:"-"`
	ContextId string `json:"contextId"`
	NotShared bool   `json:"-"` //设置为False则不跟其它agent共享
	Submit    bool   `json:"-"`
}

func (ex *EnvMessageX) jsonEncode() string {
	js, _ := json.MarshalIndent(ex, "", "  ")
	return string(js)
}

type SystemPromptX struct {
	SystemPrompt string
	ContextId    string
}
