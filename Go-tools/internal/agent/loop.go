package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	"tools/internal/llm"
	"tools/internal/tools"
)

// Renderer receives agent events and renders them to the user.
// Implementations live in the tui package (phase 1.3).
type Renderer interface {
	RenderThinking(delta string)
	RenderText(delta string)
	RenderToolCall(name string, input json.RawMessage)
	RenderToolResult(name string, result string, isError bool)
	RenderError(err error)
	RenderDone(usage *llm.Usage, toolCallCount int, elapsed time.Duration)
}

// Config holds parameters for the agent loop.
type Config struct {
	Model          string
	SystemPrompt   string
	MaxTokens      int
	MaxTurns       int // safety limit: max LLM round-trips per user prompt
	ThinkingBudget int // extended thinking token budget; 0 = disabled
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:          "claude-sonnet-4-20250514",
		MaxTokens:      16384,
		MaxTurns:       50,
		ThinkingBudget: 10000,
	}
}

// ToolsToDefinitions converts a ToolSet into LLM tool definitions by
// generating a JSON Schema from each tool's InputType via reflection.
func ToolsToDefinitions(ts *tools.ToolSet) ([]llm.ToolDefinition, error) {
	defs := make([]llm.ToolDefinition, 0, len(ts.Tools))
	for _, t := range ts.Tools {
		inputType := reflect.TypeOf(t.InputType())
		schema, err := jsonschema.ForType(inputType, &jsonschema.ForOptions{})
		if err != nil {
			return nil, fmt.Errorf("schema for tool %s: %w", t.Name(), err)
		}
		schemaJSON, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("marshal schema for tool %s: %w", t.Name(), err)
		}
		defs = append(defs, llm.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: schemaJSON,
		})
	}
	return defs, nil
}

// ExecuteToolCall finds a tool by name in the ToolSet and executes it.
// Returns a ToolResult with the output or error.
func ExecuteToolCall(ctx context.Context, ts *tools.ToolSet, call llm.ToolCall) llm.ToolResult {
	for _, t := range ts.Tools {
		if t.Name() == call.Name {
			result, err := t.Execute(ctx, call.Input)
			if err != nil {
				return llm.ToolResult{
					ToolUseID: call.ID,
					Content:   err.Error(),
					IsError:   true,
				}
			}
			return llm.ToolResult{
				ToolUseID: call.ID,
				Content:   string(result),
			}
		}
	}
	return llm.ToolResult{
		ToolUseID: call.ID,
		Content:   fmt.Sprintf("unknown tool: %s", call.Name),
		IsError:   true,
	}
}

// RunAgentLoop runs the core agent loop:
//  1. Send messages + tool definitions to the LLM (streaming)
//  2. Render thinking/text deltas and collect tool_use blocks
//  3. Execute each tool call, render results, append to conversation
//  4. Repeat from step 1 until the LLM stops calling tools or max turns hit
//
// The messages slice is updated in place with assistant and tool-result
// messages as the loop progresses.
func RunAgentLoop(
	ctx context.Context,
	provider llm.Provider,
	ts *tools.ToolSet,
	cfg Config,
	messages *[]llm.Message,
	renderer Renderer,
) error {
	toolDefs, err := ToolsToDefinitions(ts)
	if err != nil {
		return fmt.Errorf("build tool definitions: %w", err)
	}

	start := time.Now()
	totalToolCalls := 0

	for turn := 0; turn < cfg.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		req := &llm.ChatRequest{
			Model:          cfg.Model,
			SystemPrompt:   cfg.SystemPrompt,
			Messages:       *messages,
			Tools:          toolDefs,
			MaxTokens:      cfg.MaxTokens,
			ThinkingBudget: cfg.ThinkingBudget,
		}

		resp, err := provider.ChatStream(ctx, req, func(event llm.StreamEvent) {
			switch event.Type {
			case llm.StreamEventThinking:
				renderer.RenderThinking(event.Text)
			case llm.StreamEventText:
				renderer.RenderText(event.Text)
			case llm.StreamEventToolUse:
				renderer.RenderToolCall(event.ToolCall.Name, event.ToolCall.Input)
			case llm.StreamEventError:
				renderer.RenderError(event.Error)
			}
		})
		if err != nil {
			return fmt.Errorf("llm request (turn %d): %w", turn+1, err)
		}

		*messages = append(*messages, llm.NewAssistantMessage(resp.Content...))

		toolCalls := resp.ToolCalls()
		if len(toolCalls) == 0 {
			renderer.RenderDone(&resp.Usage, totalToolCalls, time.Since(start))
			return nil
		}

		var results []llm.ToolResult
		for _, call := range toolCalls {
			if err := ctx.Err(); err != nil {
				return err
			}
			result := ExecuteToolCall(ctx, ts, call)
			renderer.RenderToolResult(call.Name, result.Content, result.IsError)
			results = append(results, result)
			totalToolCalls++
		}

		*messages = append(*messages, llm.NewToolResultMessage(results...))
	}

	return fmt.Errorf("agent loop exceeded maximum turns (%d)", cfg.MaxTurns)
}
