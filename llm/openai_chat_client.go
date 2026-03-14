package llm

import (
	"AIxVuln/misc"
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAIChatClient implements Client using the official openai/openai-go SDK's
// Chat Completions API (/v1/chat/completions).
type OpenAIChatClient struct {
	cli           openai.Client
	configSection string
}

// NewOpenAIChatClient creates a client that uses the Chat Completions API.
// configSection is the config.ini section (e.g. "decision", "ops") used to
// read per-section overrides such as STREAM.
func NewOpenAIChatClient(configSection string, opts ...option.RequestOption) *OpenAIChatClient {
	return &OpenAIChatClient{
		cli:           openai.NewClient(opts...),
		configSection: configSection,
	}
}

// Chat implements Client.Chat via the Chat Completions API.
func (c *OpenAIChatClient) Chat(ctx context.Context, model string, messages []Message, tools []ToolDef) (Response, error) {
	msgs := messagesToChatParams(messages)
	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(model),
		Messages: msgs,
	}
	maxOutput := c.maxOutputTokens()
	if maxOutput > 0 {
		// Keep both fields for better compatibility with OpenAI-compatible providers.
		params.MaxCompletionTokens = openai.Int(maxOutput)
		params.MaxTokens = openai.Int(maxOutput)
	}

	if len(tools) > 0 {
		params.Tools = toolDefsToChatTools(tools)
	}

	streamVal := misc.GetConfigValueDefault(c.configSection, "STREAM", "")
	if streamVal == "" {
		streamVal = misc.GetConfigValueDefault("main_setting", "STREAM", "false")
	}
	useStream := strings.EqualFold(streamVal, "true")

	if useStream {
		return c.chatStream(ctx, params)
	}
	return c.chatSync(ctx, params)
}

func (c *OpenAIChatClient) maxOutputTokens() int64 {
	raw := strings.TrimSpace(misc.GetConfigValueDefault(c.configSection, "MaxOutputTokens",
		misc.GetConfigValueDefault("main_setting", "MaxOutputTokens", "2048")))
	if raw == "" {
		return 2048
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 2048
	}
	if n > 8192 {
		n = 8192
	}
	return int64(n)
}

// chatSync performs a non-streaming Chat Completions request.
func (c *OpenAIChatClient) chatSync(ctx context.Context, params openai.ChatCompletionNewParams) (Response, error) {
	resp, err := c.cli.Chat.Completions.New(ctx, params)
	if err != nil {
		return Response{}, err
	}
	return fromChatCompletion(resp)
}

// chatStream performs a streaming Chat Completions request and accumulates
// the full response before returning.
func (c *OpenAIChatClient) chatStream(ctx context.Context, params openai.ChatCompletionNewParams) (Response, error) {
	stream := c.cli.Chat.Completions.NewStreaming(ctx, params)
	defer stream.Close()

	var r Response
	// Map of tool-call index → ToolCall being accumulated.
	tcMap := make(map[int]*ToolCall)

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) == 0 {
			continue
		}
		delta := chunk.Choices[0].Delta
		r.Content += delta.Content
		for _, tc := range delta.ToolCalls {
			idx := int(tc.Index)
			existing, ok := tcMap[idx]
			if !ok {
				existing = &ToolCall{}
				tcMap[idx] = existing
			}
			if tc.ID != "" {
				existing.ID = tc.ID
			}
			if tc.Function.Name != "" {
				existing.Name += tc.Function.Name
			}
			existing.Arguments += tc.Function.Arguments
		}
	}
	if err := stream.Err(); err != nil {
		return Response{}, err
	}

	// Collect tool calls in index order.
	if len(tcMap) > 0 {
		r.ToolCalls = make([]ToolCall, 0, len(tcMap))
		for i := 0; i < len(tcMap); i++ {
			if tc, ok := tcMap[i]; ok {
				r.ToolCalls = append(r.ToolCalls, *tc)
			}
		}
	}

	if r.Content == "" && len(r.ToolCalls) == 0 {
		return Response{Content: "empty streaming response"}, nil
	}
	return r, nil
}

// --- conversion helpers: llm types → Chat Completions params ---

func messagesToChatParams(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: openai.String(m.Content),
					},
				},
			})
		case RoleUser:
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: openai.String(m.Content),
					},
				},
			})
		case RoleAssistant:
			asst := &openai.ChatCompletionAssistantMessageParam{
				Content: openai.ChatCompletionAssistantMessageParamContentUnion{
					OfString: openai.String(m.Content),
				},
			}
			if len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					asst.ToolCalls = append(asst.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
						OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
							ID: tc.ID,
							Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
								Name:      tc.Name,
								Arguments: tc.Arguments,
							},
						},
					})
				}
			}
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfAssistant: asst,
			})
		case RoleTool:
			out = append(out, openai.ChatCompletionMessageParamUnion{
				OfTool: &openai.ChatCompletionToolMessageParam{
					ToolCallID: m.ToolCallID,
					Content: openai.ChatCompletionToolMessageParamContentUnion{
						OfString: openai.String(m.Content),
					},
				},
			})
		}
	}
	return out
}

func toolDefsToChatTools(defs []ToolDef) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, len(defs))
	for i, d := range defs {
		out[i] = openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        d.Name,
					Description: openai.String(d.Description),
					Parameters:  shared.FunctionParameters(d.Parameters),
				},
			},
		}
	}
	return out
}

// --- conversion helpers: Chat Completions output → llm types ---

func fromChatCompletion(resp *openai.ChatCompletion) (Response, error) {
	if len(resp.Choices) == 0 {
		return Response{}, fmt.Errorf("empty response (finish_reason=none)")
	}
	msg := resp.Choices[0].Message
	r := Response{Content: msg.Content}
	if resp.Usage.TotalTokens > 0 {
		r.Usage = Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}
	for _, tc := range msg.ToolCalls {
		if tc.Type == "function" {
			fn := tc.AsFunction()
			r.ToolCalls = append(r.ToolCalls, ToolCall{
				ID:        fn.ID,
				Name:      fn.Function.Name,
				Arguments: fn.Function.Arguments,
			})
		}
	}
	return r, nil
}
