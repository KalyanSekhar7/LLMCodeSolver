package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// AnthropicProvider implements Provider using the Anthropic Messages API.
type AnthropicProvider struct {
	client anthropic.Client
}

// NewAnthropicProvider creates a provider for the Anthropic API.
// If apiKey is empty, the SDK falls back to ANTHROPIC_API_KEY from the environment.
func NewAnthropicProvider(apiKey string) *AnthropicProvider {
	var opts []option.RequestOption
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}
	return &AnthropicProvider{client: anthropic.NewClient(opts...)}
}

// Chat sends a non-streaming request and returns the complete response.
func (p *AnthropicProvider) Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	params := p.buildParams(req)
	msg, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic: %w", err)
	}
	return p.convertResponse(msg), nil
}

// ChatStream sends a streaming request, calling handler for each incremental
// event. Returns the fully accumulated response when the stream completes.
func (p *AnthropicProvider) ChatStream(ctx context.Context, req *ChatRequest, handler StreamHandler) (*ChatResponse, error) {
	params := p.buildParams(req)
	stream := p.client.Messages.NewStreaming(ctx, params)

	msg := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		if err := msg.Accumulate(event); err != nil {
			handler(StreamEvent{Type: StreamEventError, Error: err})
			return nil, fmt.Errorf("anthropic accumulate: %w", err)
		}

		switch variant := event.AsAny().(type) {
		case anthropic.ContentBlockDeltaEvent:
			switch delta := variant.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				handler(StreamEvent{Type: StreamEventText, Text: delta.Text})
			case anthropic.ThinkingDelta:
				handler(StreamEvent{Type: StreamEventThinking, Text: delta.Thinking})
			}
		case anthropic.ContentBlockStopEvent:
			// Emit completed tool_use blocks so the caller can display them
			idx := int(variant.Index)
			if idx < len(msg.Content) {
				if toolUse, ok := msg.Content[idx].AsAny().(anthropic.ToolUseBlock); ok {
					inputJSON, _ := json.Marshal(toolUse.Input)
					handler(StreamEvent{
						Type: StreamEventToolUse,
						ToolCall: &ToolCall{
							ID:    toolUse.ID,
							Name:  toolUse.Name,
							Input: inputJSON,
						},
					})
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		handler(StreamEvent{Type: StreamEventError, Error: err})
		return nil, fmt.Errorf("anthropic stream: %w", err)
	}

	resp := p.convertResponse(&msg)
	handler(StreamEvent{
		Type:       StreamEventDone,
		StopReason: resp.StopReason,
		Usage:      &resp.Usage,
	})
	return resp, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (p *AnthropicProvider) buildParams(req *ChatRequest) anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  p.convertMessages(req.Messages),
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	if len(req.Tools) > 0 {
		params.Tools = p.convertTools(req.Tools)
	}

	if req.ThinkingBudget > 0 {
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{
				BudgetTokens: int64(req.ThinkingBudget),
			},
		}
	}

	// Temperature cannot be set when thinking is enabled.
	if req.Temperature != nil && req.ThinkingBudget <= 0 {
		params.Temperature = anthropic.Float(*req.Temperature)
	}

	return params
}

func (p *AnthropicProvider) convertMessages(msgs []Message) []anthropic.MessageParam {
	result := make([]anthropic.MessageParam, 0, len(msgs))
	for _, msg := range msgs {
		var blocks []anthropic.ContentBlockParamUnion
		for _, b := range msg.Content {
			switch b.Type {
			case ContentTypeText:
				blocks = append(blocks, anthropic.NewTextBlock(b.Text))
			case ContentTypeThinking:
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfThinking: &anthropic.ThinkingBlockParam{
						Thinking:  b.Text,
						Signature: b.Signature,
					},
				})
			case ContentTypeToolUse:
				blocks = append(blocks, anthropic.ContentBlockParamUnion{
					OfToolUse: &anthropic.ToolUseBlockParam{
						ID:    b.ID,
						Name:  b.Name,
						Input: json.RawMessage(b.Input),
					},
				})
			case ContentTypeToolResult:
				blocks = append(blocks, anthropic.NewToolResultBlock(b.ToolUseID, b.Content, b.IsError))
			}
		}
		result = append(result, anthropic.MessageParam{
			Role:    anthropic.MessageParamRole(msg.Role),
			Content: blocks,
		})
	}
	return result
}

func (p *AnthropicProvider) convertTools(tools []ToolDefinition) []anthropic.ToolUnionParam {
	result := make([]anthropic.ToolUnionParam, len(tools))
	for i, t := range tools {
		var schema anthropic.ToolInputSchemaParam
		if t.InputSchema != nil {
			var schemaMap map[string]any
			if err := json.Unmarshal(t.InputSchema, &schemaMap); err == nil {
				schema.Properties = schemaMap["properties"]
				// Forward "required" and other top-level schema fields the
				// SDK's ToolInputSchemaParam doesn't model directly.
				extra := make(map[string]any)
				for _, key := range []string{"required", "additionalProperties"} {
					if v, ok := schemaMap[key]; ok {
						extra[key] = v
					}
				}
				if len(extra) > 0 {
					schema.SetExtraFields(extra)
				}
			}
		}
		result[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: schema,
			},
		}
	}
	return result
}

func (p *AnthropicProvider) convertResponse(msg *anthropic.Message) *ChatResponse {
	resp := &ChatResponse{
		StopReason: StopReason(msg.StopReason),
		Usage: Usage{
			InputTokens:  int(msg.Usage.InputTokens),
			OutputTokens: int(msg.Usage.OutputTokens),
		},
	}

	for _, block := range msg.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			resp.Content = append(resp.Content, ContentBlock{
				Type: ContentTypeText,
				Text: v.Text,
			})
		case anthropic.ThinkingBlock:
			resp.Content = append(resp.Content, ContentBlock{
				Type:      ContentTypeThinking,
				Text:      v.Thinking,
				Signature: v.Signature,
			})
		case anthropic.ToolUseBlock:
			inputJSON, _ := json.Marshal(v.Input)
			resp.Content = append(resp.Content, ContentBlock{
				Type:  ContentTypeToolUse,
				ID:    v.ID,
				Name:  v.Name,
				Input: inputJSON,
			})
		}
	}

	return resp
}
