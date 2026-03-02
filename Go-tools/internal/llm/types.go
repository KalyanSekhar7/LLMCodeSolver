package llm

import (
	"encoding/json"
	"strings"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonToolUse   StopReason = "tool_use"
	StopReasonMaxTokens StopReason = "max_tokens"
)

type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeThinking   ContentType = "thinking"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

// ContentBlock is a single piece of content within a message.
// Depending on Type, different fields are populated:
//   - text:        Text
//   - thinking:    Text, Signature
//   - tool_use:    ID, Name, Input
//   - tool_result: ToolUseID, Content, IsError
type ContentBlock struct {
	Type ContentType

	// text and thinking blocks
	Text string

	// thinking blocks — signature is required for round-tripping with the API
	Signature string

	// tool_use blocks
	ID    string
	Name  string
	Input json.RawMessage

	// tool_result blocks
	ToolUseID string
	Content   string
	IsError   bool
}

// Message represents a single message in a conversation.
type Message struct {
	Role    Role
	Content []ContentBlock
}

// ToolDefinition describes a tool available to the LLM.
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage // full JSON Schema object
}

// ChatRequest contains all parameters for an LLM API call.
type ChatRequest struct {
	Model          string
	SystemPrompt   string
	Messages       []Message
	Tools          []ToolDefinition
	MaxTokens      int
	Temperature    *float64
	ThinkingBudget int // extended thinking budget in tokens; 0 = disabled
}

// ChatResponse contains the LLM's complete response.
type ChatResponse struct {
	Content    []ContentBlock
	StopReason StopReason
	Usage      Usage
}

// Usage tracks token consumption for a single request.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

type StreamEventType string

const (
	StreamEventThinking StreamEventType = "thinking"
	StreamEventText     StreamEventType = "text"
	StreamEventToolUse  StreamEventType = "tool_use"
	StreamEventDone     StreamEventType = "done"
	StreamEventError    StreamEventType = "error"
)

// StreamEvent represents a single event during response streaming.
type StreamEvent struct {
	Type       StreamEventType
	Text       string     // delta text for thinking/text events
	ToolCall   *ToolCall  // completed tool call for tool_use events
	StopReason StopReason // set on done events
	Error      error      // set on error events
	Usage      *Usage     // set on done events
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// ToolResult represents the outcome of executing a tool.
type ToolResult struct {
	ToolUseID string
	Content   string
	IsError   bool
}

// StreamHandler is a callback invoked for each streaming event.
type StreamHandler func(event StreamEvent)

// ---------------------------------------------------------------------------
// Constructors
// ---------------------------------------------------------------------------

func NewUserMessage(text string) Message {
	return Message{
		Role:    RoleUser,
		Content: []ContentBlock{{Type: ContentTypeText, Text: text}},
	}
}

func NewAssistantMessage(blocks ...ContentBlock) Message {
	return Message{Role: RoleAssistant, Content: blocks}
}

func NewToolResultMessage(results ...ToolResult) Message {
	blocks := make([]ContentBlock, len(results))
	for i, r := range results {
		blocks[i] = ContentBlock{
			Type:      ContentTypeToolResult,
			ToolUseID: r.ToolUseID,
			Content:   r.Content,
			IsError:   r.IsError,
		}
	}
	return Message{Role: RoleUser, Content: blocks}
}

// ---------------------------------------------------------------------------
// ChatResponse helpers
// ---------------------------------------------------------------------------

// ToolCalls extracts all tool_use blocks from the response.
func (r *ChatResponse) ToolCalls() []ToolCall {
	var calls []ToolCall
	for _, b := range r.Content {
		if b.Type == ContentTypeToolUse {
			calls = append(calls, ToolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		}
	}
	return calls
}

// HasToolUse reports whether the response contains any tool_use blocks.
func (r *ChatResponse) HasToolUse() bool {
	for _, b := range r.Content {
		if b.Type == ContentTypeToolUse {
			return true
		}
	}
	return false
}

// TextContent returns the concatenated text from all text blocks.
func (r *ChatResponse) TextContent() string {
	var parts []string
	for _, b := range r.Content {
		if b.Type == ContentTypeText {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "")
}
