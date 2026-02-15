package llm

import (
	"AIxVuln/misc"
	"context"
	"runtime"
)

// RequestLLMOpts carries optional metadata for a single LLM request.
type RequestLLMOpts struct {
	ProjectName string // project-level token accumulation
	AgentLabel  string // per-agent token accumulation (e.g. persona name or "决策大脑")
}

// RequestLLM sends a chat request through the abstract Client interface.
// Per-API-key concurrency limiting is handled by the rateLimitedClient wrapper
// created in client_factory.go.
// The variadic projectName is kept for backward compatibility; prefer RequestLLMWithOpts for new code.
func RequestLLM(cli Client, ctx context.Context, model string, messages []Message, tools []ToolDef, projectName ...string) (Response, error) {
	opts := RequestLLMOpts{}
	if len(projectName) > 0 {
		opts.ProjectName = projectName[0]
	}
	return RequestLLMWithOpts(cli, ctx, model, messages, tools, opts)
}

// RequestLLMWithOpts sends a chat request and accumulates token usage per project and per agent.
func RequestLLMWithOpts(cli Client, ctx context.Context, model string, messages []Message, tools []ToolDef, opts RequestLLMOpts) (Response, error) {
	resp, err := cli.Chat(ctx, model, messages, tools)
	if err != nil {
		pc, f, _, ok := runtime.Caller(1)
		if ok {
			funcName := runtime.FuncForPC(pc).Name()
			misc.Debug("API ERR: %s. from: %s-%s", err.Error(), f, funcName)
		} else {
			misc.Debug("API ERR: %s", err.Error())
		}
	}
	// Accumulate token usage for the project (and optionally per-agent).
	if err == nil && resp.Usage.TotalTokens > 0 && opts.ProjectName != "" {
		if opts.AgentLabel != "" {
			AddProjectAgentTokenUsage(opts.ProjectName, opts.AgentLabel, resp.Usage)
		} else {
			AddProjectTokenUsage(opts.ProjectName, resp.Usage)
		}
	}
	return resp, err
}

// ResponseToMessage converts a Response into an assistant Message,
// preserving any tool calls.
func ResponseToMessage(resp Response) Message {
	return Message{
		Role:      RoleAssistant,
		Content:   resp.Content,
		ToolCalls: resp.ToolCalls,
	}
}
