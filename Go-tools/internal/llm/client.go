package llm

import (
	"context"
	"fmt"
)

// Provider is the interface every LLM backend must implement.
type Provider interface {
	// Chat sends a non-streaming request and returns the complete response.
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// ChatStream sends a streaming request. The handler is called for each
	// incremental event (thinking deltas, text deltas, completed tool calls).
	// The returned ChatResponse contains the fully accumulated result.
	ChatStream(ctx context.Context, req *ChatRequest, handler StreamHandler) (*ChatResponse, error)
}

// NewProvider creates a Provider for the given backend name.
// Supported names: "anthropic" (default when empty).
func NewProvider(name, apiKey string) (Provider, error) {
	switch name {
	case "anthropic", "":
		return NewAnthropicProvider(apiKey), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider: %q", name)
	}
}
