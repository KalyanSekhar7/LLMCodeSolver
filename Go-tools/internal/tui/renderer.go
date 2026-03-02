package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"tools/internal/llm"
)

const (
	maxResultLines   = 50
	boxContentWidth  = 70
	thinkingIndent   = "    "
	textIndent       = "  "
	toolArgIndent    = "    "
	boxIndent        = "    "
)

// TUI renders agent events to the terminal and handles user input.
// It implements the agent.Renderer interface.
type TUI struct {
	w  io.Writer
	rl ReadLiner // readline abstraction (set by prompt.go)
	mu sync.Mutex

	// Streaming state: which block type is currently being rendered.
	inThinking  bool
	inText      bool
	atLineStart bool

	// Session info for the banner.
	model     string
	maxTokens int
	toolCount int

	// Cumulative session usage.
	SessionTokensIn  int
	SessionTokensOut int
	SessionToolCalls int
	SessionPrompts   int
}

// ReadLiner abstracts readline so the renderer doesn't depend on it directly.
type ReadLiner interface {
	Readline() (string, error)
	Close() error
}

// ---------------------------------------------------------------------------
// agent.Renderer implementation
// ---------------------------------------------------------------------------

func (t *TUI) RenderThinking(delta string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.endText()

	if !t.inThinking {
		fmt.Fprintf(t.w, "\n  %s%s%s Thinking...%s\n", Dim, Cyan, SymThinking, Reset)
		fmt.Fprint(t.w, Dim+Italic)
		t.inThinking = true
		t.atLineStart = true
	}

	t.writeIndented(delta, thinkingIndent)
}

func (t *TUI) RenderText(delta string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.endThinking()

	if !t.inText {
		fmt.Fprintln(t.w)
		t.inText = true
		t.atLineStart = true
	}

	t.writeIndented(delta, textIndent)
}

func (t *TUI) RenderToolCall(name string, input json.RawMessage) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.endThinking()
	t.endText()

	fmt.Fprintf(t.w, "\n  %s%s%s Tool: %s%s\n", Bold, Cyan, SymTool, name, Reset)

	var args map[string]interface{}
	if json.Unmarshal(input, &args) == nil && len(args) > 0 {
		keys := sortedKeys(args)
		for _, key := range keys {
			val := args[key]
			if isEmptyValue(val) {
				continue
			}
			display := formatArgValue(val)
			if utf8.RuneCountInString(display) > 100 {
				display = string([]rune(display)[:97]) + "..."
			}
			fmt.Fprintf(t.w, "%s%s%s:%s %s\n", toolArgIndent, Gray, key, Reset, display)
		}
	}
}

func (t *TUI) RenderToolResult(name string, result string, isError bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	result = strings.TrimRight(result, "\n")
	if result == "" {
		fmt.Fprintf(t.w, "%s%s(empty)%s\n", toolArgIndent, Dim, Reset)
		return
	}

	if isError {
		fmt.Fprintf(t.w, "%s%s%s%s Error: %s%s\n", toolArgIndent, Bold, Red, SymError, result, Reset)
		return
	}

	lines := strings.Split(result, "\n")
	if len(lines) <= 3 {
		for _, line := range lines {
			fmt.Fprintf(t.w, "%s%s%s%s\n", boxIndent, Dim, truncateLine(line, boxContentWidth), Reset)
		}
		return
	}

	t.drawBox(lines, maxResultLines)
}

func (t *TUI) RenderError(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.endThinking()
	t.endText()

	fmt.Fprintf(t.w, "\n  %s%s%s Error: %s%s\n", Bold, Red, SymError, err.Error(), Reset)
}

func (t *TUI) RenderDone(usage *llm.Usage, toolCallCount int, elapsed time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.endThinking()
	t.endText()

	// Accumulate session totals
	t.SessionToolCalls += toolCallCount
	t.SessionPrompts++
	if usage != nil {
		t.SessionTokensIn += usage.InputTokens
		t.SessionTokensOut += usage.OutputTokens
	}

	secs := elapsed.Seconds()
	fmt.Fprintf(t.w, "\n  %s%s%s Done%s", Bold, Green, SymDone, Reset)
	if toolCallCount > 0 || usage != nil {
		fmt.Fprint(t.w, " (")
		parts := []string{}
		if toolCallCount > 0 {
			noun := "tool calls"
			if toolCallCount == 1 {
				noun = "tool call"
			}
			parts = append(parts, fmt.Sprintf("%d %s", toolCallCount, noun))
		}
		parts = append(parts, fmt.Sprintf("%.1fs", secs))
		if usage != nil {
			parts = append(parts, fmt.Sprintf("%d→%d tokens", usage.InputTokens, usage.OutputTokens))
		}
		fmt.Fprint(t.w, strings.Join(parts, ", "))
		fmt.Fprint(t.w, ")")
	}
	fmt.Fprintln(t.w)

	// Show cumulative session usage
	fmt.Fprintf(t.w, "  %sSession: %d in + %d out = %d tokens │ %d tool calls │ %d prompts%s\n\n",
		Dim, t.SessionTokensIn, t.SessionTokensOut,
		t.SessionTokensIn+t.SessionTokensOut,
		t.SessionToolCalls, t.SessionPrompts, Reset)
}

// ---------------------------------------------------------------------------
// Tool hint bar
// ---------------------------------------------------------------------------

// SetSessionInfo stores model and config details for the banner display.
func (t *TUI) SetSessionInfo(model string, maxTokens int, toolCount int) {
	t.model = model
	t.maxTokens = maxTokens
	t.toolCount = toolCount
}

// ---------------------------------------------------------------------------
// Streaming state helpers
// ---------------------------------------------------------------------------

func (t *TUI) endThinking() {
	if !t.inThinking {
		return
	}
	if !t.atLineStart {
		fmt.Fprintln(t.w)
	}
	fmt.Fprint(t.w, Reset)
	t.inThinking = false
	t.atLineStart = true
}

func (t *TUI) endText() {
	if !t.inText {
		return
	}
	if !t.atLineStart {
		fmt.Fprintln(t.w)
	}
	t.inText = false
	t.atLineStart = true
}

// writeIndented writes s to the output, adding indent at the start of each line.
// It tracks whether the cursor is at the beginning of a line across calls to
// support streaming deltas that may split mid-line.
func (t *TUI) writeIndented(s, indent string) {
	for _, part := range strings.SplitAfter(s, "\n") {
		if part == "" {
			continue
		}
		if t.atLineStart {
			io.WriteString(t.w, indent)
		}
		io.WriteString(t.w, part)
		t.atLineStart = strings.HasSuffix(part, "\n")
	}
}

// ---------------------------------------------------------------------------
// Box drawing
// ---------------------------------------------------------------------------

func (t *TUI) drawBox(lines []string, maxLines int) {
	truncated := 0
	if maxLines > 0 && len(lines) > maxLines {
		truncated = len(lines) - maxLines
		lines = lines[:maxLines]
	}

	width := boxContentWidth
	topBottom := strings.Repeat(BoxHorizontal, width+2)

	fmt.Fprintf(t.w, "%s%s%s%s%s%s\n", boxIndent, Gray, BoxTopLeft, topBottom, BoxTopRight, Reset)

	for _, line := range lines {
		display := truncateLine(line, width)
		padding := width - visibleLen(display)
		if padding < 0 {
			padding = 0
		}
		fmt.Fprintf(t.w, "%s%s%s%s %s%s %s%s%s\n",
			boxIndent, Gray, BoxVertical, Reset,
			display, strings.Repeat(" ", padding),
			Gray, BoxVertical, Reset)
	}

	if truncated > 0 {
		msg := fmt.Sprintf("[+%d more lines]", truncated)
		padding := width - len(msg)
		if padding < 0 {
			padding = 0
		}
		fmt.Fprintf(t.w, "%s%s%s%s %s%s%s%s %s%s%s\n",
			boxIndent, Gray, BoxVertical, Reset,
			Dim, msg, Reset, strings.Repeat(" ", padding),
			Gray, BoxVertical, Reset)
	}

	fmt.Fprintf(t.w, "%s%s%s%s%s%s\n", boxIndent, Gray, BoxBottomLeft, topBottom, BoxBottomRight, Reset)
}

// ---------------------------------------------------------------------------
// Formatting helpers
// ---------------------------------------------------------------------------

func truncateLine(s string, maxWidth int) string {
	runes := []rune(s)
	// Replace tabs with spaces for display
	cleaned := strings.ReplaceAll(string(runes), "\t", "    ")
	runes = []rune(cleaned)
	if len(runes) > maxWidth {
		return string(runes[:maxWidth-3]) + "..."
	}
	return cleaned
}

func visibleLen(s string) int {
	n := 0
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		n++
	}
	return n
}

func isEmptyValue(v interface{}) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return val == ""
	case float64:
		return val == 0
	case bool:
		return !val
	case []interface{}:
		return len(val) == 0
	case map[string]interface{}:
		return len(val) == 0
	}
	return false
}

func formatArgValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case bool:
		return fmt.Sprintf("%v", val)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
