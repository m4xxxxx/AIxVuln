package toolCalling

import (
	"AIxVuln/llm"
	"AIxVuln/misc"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ToolHandler interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(args map[string]interface{}) string
}

type ToolManager struct {
	handlers      map[string]ToolHandler
	maxIterations int // 每次最大调用函数数量限制
}

const (
	defaultLLMReplyTokenReserve = 2048
	defaultLLMInputHardLimit    = 180000
)

func NewToolManager() *ToolManager {
	fm := &ToolManager{
		handlers:      make(map[string]ToolHandler),
		maxIterations: 10,
	}
	for _, h := range GetMCPToolHandlers() {
		fm.Register(h)
	}
	return fm
}

func (fm *ToolManager) RegisterHandlers(handlers ...ToolHandler) {
	for _, handler := range handlers {
		if handler == nil {
			continue
		}
		fm.Register(handler)
	}
}

func (fm *ToolManager) Register(handler ToolHandler) {
	fm.handlers[handler.Name()] = handler
}

func (fm *ToolManager) RemoveTool(name string) {
	delete(fm.handlers, name)
}

func (fm *ToolManager) GetTools() []llm.ToolDef {
	names := make([]string, 0, len(fm.handlers))
	for name := range fm.handlers {
		names = append(names, name)
	}
	sort.Strings(names)
	definitions := make([]llm.ToolDef, 0, len(names))
	for _, name := range names {
		handler := fm.handlers[name]
		definitions = append(definitions, llm.ToolDef{
			Name:        handler.Name(),
			Description: handler.Description(),
			Parameters:  handler.Parameters(),
		})
	}
	return definitions
}

// return Assistant Message、tool Result Messages、error
func (fm *ToolManager) ToolCallRequest(
	ctx context.Context,
	cli llm.Client,
	messages []llm.Message,
	model string,
	agentName string,
	projectName ...string,
) (llm.Message, []llm.Message, error) {
	return fm.ToolCallRequestWithLabel(ctx, cli, messages, model, agentName, "", projectName...)
}

// ToolCallRequestWithLabel is like ToolCallRequest but also tracks per-agent token usage via agentLabel.
func (fm *ToolManager) ToolCallRequestWithLabel(
	ctx context.Context,
	cli llm.Client,
	messages []llm.Message,
	model string,
	agentName string,
	agentLabel string,
	projectName ...string,
) (llm.Message, []llm.Message, error) {
	inputBudget := fm.estimateInputBudget(agentName)
	tools := fm.prepareToolsForRequest(fm.GetTools(), inputBudget)
	reqMessages := fm.prepareMessagesForRequest(messages, tools, inputBudget, agentName)
	count := 0
	var resp llm.Response
	var err error
	opts := llm.RequestLLMOpts{AgentLabel: agentLabel}
	if len(projectName) > 0 {
		opts.ProjectName = projectName[0]
	}
	for {
		reqCtx, c := context.WithTimeout(ctx, time.Duration(600)*time.Second)
		resp, err = llm.RequestLLMWithOpts(cli, reqCtx, model, reqMessages, tools, opts)
		c()
		if err == nil || count >= misc.GetMaxTryCount() {
			break
		}
		time.Sleep(time.Duration(5) * time.Second)
		count++
	}
	if err != nil {
		return llm.Message{}, nil, err
	}
	message := llm.ResponseToMessage(resp)
	var toolMessage []llm.Message
	for _, toolCall := range message.ToolCalls {
		handler, exists := fm.handlers[toolCall.Name]
		if !exists {
			toolMessage = append(toolMessage, llm.Message{
				Role:       llm.RoleTool,
				Content:    Fail(fmt.Sprintf("%s is not registered", toolCall.Name)),
				ToolCallID: toolCall.ID,
			})
			continue
		}

		// 解析参数
		var args map[string]interface{}
		if err := json.Unmarshal([]byte(toolCall.Arguments), &args); err != nil {
			toolMessage = append(toolMessage, llm.Message{
				Role:       llm.RoleTool,
				Content:    Fail("parse arguments failed"),
				ToolCallID: toolCall.ID,
			})
			continue
		}
		resultJSON := handler.Execute(args)
		toolMessage = append(toolMessage, llm.Message{
			Role:       llm.RoleTool,
			Content:    resultJSON,
			ToolCallID: toolCall.ID,
		})
	}
	return message, toolMessage, nil
}

func (fm *ToolManager) prepareToolsForRequest(tools []llm.ToolDef, inputBudget int) []llm.ToolDef {
	if len(tools) == 0 {
		return tools
	}
	core := make([]llm.ToolDef, 0, len(tools))
	mcp := make([]llm.ToolDef, 0, len(tools))
	for _, t := range tools {
		if strings.HasPrefix(t.Name, "MCP_") {
			mcp = append(mcp, compactMCPToolDef(t))
			continue
		}
		core = append(core, t)
	}
	// Keep core tools intact. For MCP tools, dynamically cap by token budget.
	sort.SliceStable(mcp, func(i, j int) bool {
		pi := mcpToolPriority(mcp[i].Name)
		pj := mcpToolPriority(mcp[j].Name)
		if pi != pj {
			return pi < pj
		}
		return mcp[i].Name < mcp[j].Name
	})
	toolBudget := inputBudget / 2
	if toolBudget < 8192 {
		toolBudget = 8192
	}
	if toolBudget > 60000 {
		toolBudget = 60000
	}
	coreTokens := estimateToolsTokens(core)
	selectedMCP := make([]llm.ToolDef, 0, len(mcp))
	used := coreTokens
	for _, t := range mcp {
		tk := estimateToolsTokens([]llm.ToolDef{t})
		if used+tk > toolBudget && len(selectedMCP) >= 8 {
			continue
		}
		selectedMCP = append(selectedMCP, t)
		used += tk
	}
	if len(selectedMCP) == 0 && len(mcp) > 0 {
		keep := len(mcp)
		if keep > 8 {
			keep = 8
		}
		selectedMCP = append(selectedMCP, mcp[:keep]...)
	}
	out := make([]llm.ToolDef, 0, len(core)+len(selectedMCP))
	out = append(out, core...)
	out = append(out, selectedMCP...)
	return out
}

func mcpToolPriority(name string) int {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "search"), strings.Contains(n, "query"):
		return 1
	case strings.Contains(n, "repo"), strings.Contains(n, "repository"), strings.Contains(n, "list"):
		return 2
	case strings.Contains(n, "read"), strings.Contains(n, "get"), strings.Contains(n, "file"), strings.Contains(n, "code"):
		return 3
	case strings.Contains(n, "issue"), strings.Contains(n, "pull"), strings.Contains(n, "pr"), strings.Contains(n, "workflow"):
		return 4
	default:
		return 10
	}
}

func compactMCPToolDef(in llm.ToolDef) llm.ToolDef {
	in.Description = truncateString(in.Description, 180)
	// MCP tools often carry very verbose JSON schema/description. Keep a
	// generic object schema to reduce prompt tokens and avoid context overflow.
	in.Parameters = map[string]interface{}{
		"type":                 "object",
		"properties":           map[string]interface{}{},
		"additionalProperties": true,
	}
	return in
}

func truncateString(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max < 16 {
		return s[:max]
	}
	return s[:max] + "...[truncated]"
}

func (fm *ToolManager) prepareMessagesForRequest(messages []llm.Message, tools []llm.ToolDef, inputBudget int, agentName string) []llm.Message {
	if len(messages) <= 3 {
		return messages
	}
	if inputBudget <= 0 {
		return messages
	}
	out := append([]llm.Message(nil), messages...)
	estimate := llm.CountMessagesTokens(out) + estimateToolsTokens(tools)
	for estimate > inputBudget && len(out) > 3 {
		dropEnd := 3 // at least drop oldest conversation message (index=2)
		if out[2].Role == llm.RoleAssistant && len(out[2].ToolCalls) > 0 {
			ids := make(map[string]struct{}, len(out[2].ToolCalls))
			for _, tc := range out[2].ToolCalls {
				ids[tc.ID] = struct{}{}
			}
			for dropEnd < len(out) && out[dropEnd].Role == llm.RoleTool {
				if _, ok := ids[out[dropEnd].ToolCallID]; !ok {
					break
				}
				dropEnd++
			}
		}
		out = append(out[:2], out[dropEnd:]...)
		estimate = llm.CountMessagesTokens(out) + estimateToolsTokens(tools)
	}
	if estimate > inputBudget {
		misc.Debug("%s: request still near token limit after trimming (estimate=%d, budget=%d, tools=%d)", agentName, estimate, inputBudget, len(tools))
	}
	return out
}

func estimateToolsTokens(tools []llm.ToolDef) int {
	total := 0
	for _, t := range tools {
		total += 16 // per-tool structural overhead
		total += llm.CountTokens(t.Name)
		total += llm.CountTokens(t.Description)
		total += llm.EstimateJSONTokens(t.Parameters)
	}
	return total
}

func (fm *ToolManager) estimateInputBudget(agentName string) int {
	section := sectionFromAgentName(agentName)
	budget := misc.GetMaxContext(section, "main_setting")
	if budget > defaultLLMInputHardLimit {
		budget = defaultLLMInputHardLimit
	}
	reserve := defaultLLMReplyTokenReserve
	if v := strings.TrimSpace(misc.GetConfigValueDefault(section, "MaxOutputTokens",
		misc.GetConfigValueDefault("main_setting", "MaxOutputTokens", ""))); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			reserve = n
		}
	}
	budget -= reserve + 1024 // safety margin
	if budget < 4096 {
		budget = 4096
	}
	return budget
}

func sectionFromAgentName(agentName string) string {
	name := strings.ToLower(strings.TrimSpace(agentName))
	switch {
	case strings.Contains(name, "decision"):
		return "decision"
	case strings.Contains(name, "general"):
		return "general"
	case strings.Contains(name, "analyze"):
		return "analyze"
	case strings.Contains(name, "verifier"):
		return "verifier"
	case strings.Contains(name, "overview"):
		return "overview"
	case strings.Contains(name, "report"):
		return "report"
	case strings.Contains(name, "ops"):
		return "ops"
	default:
		return "main_setting"
	}
}
