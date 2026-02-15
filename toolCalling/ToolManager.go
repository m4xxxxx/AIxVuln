package toolCalling

import (
	"AIxVuln/llm"
	"AIxVuln/misc"
	"context"
	"encoding/json"
	"fmt"
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

func NewToolManager() *ToolManager {
	return &ToolManager{
		handlers:      make(map[string]ToolHandler),
		maxIterations: 10,
	}
}

func (fm *ToolManager) Register(handler ToolHandler) {
	fm.handlers[handler.Name()] = handler
}

func (fm *ToolManager) RemoveTool(name string) {
	delete(fm.handlers, name)
}

func (fm *ToolManager) GetTools() []llm.ToolDef {
	var definitions []llm.ToolDef
	for _, handler := range fm.handlers {
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
	tools := fm.GetTools()
	count := 0
	var resp llm.Response
	var err error
	opts := llm.RequestLLMOpts{AgentLabel: agentLabel}
	if len(projectName) > 0 {
		opts.ProjectName = projectName[0]
	}
	for {
		reqCtx, c := context.WithTimeout(ctx, time.Duration(600)*time.Second)
		defer c()
		size := 0
		for _, v := range messages {
			d, _ := v.MarshalJSON()
			size += len(d)
		}
		_ = size
		resp, err = llm.RequestLLMWithOpts(cli, reqCtx, model, messages, tools, opts)
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
