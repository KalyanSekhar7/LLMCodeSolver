package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/chzyer/readline"
)

// SlashCommand represents a parsed slash command from user input.
type SlashCommand struct {
	Name string // command name without the leading /
	Args string // remaining text after the command name
}

// New creates a TUI instance with readline-based input and stdout rendering.
func New() (*TUI, error) {
	historyFile := filepath.Join(os.TempDir(), "llm-agent-history")

	rl, err := readline.NewEx(&readline.Config{
		Prompt:            Bold + ">>> " + Reset,
		HistoryFile:       historyFile,
		InterruptPrompt:   "^C",
		EOFPrompt:         "",
		HistorySearchFold: true,
	})
	if err != nil {
		return nil, fmt.Errorf("readline init: %w", err)
	}

	return &TUI{
		w:           os.Stdout,
		rl:          rl,
		atLineStart: true,
	}, nil
}

// ReadPrompt reads a line of user input using readline.
// Returns io.EOF on Ctrl+D. Ctrl+C clears the line and re-prompts.
// Empty lines are silently skipped.
func (t *TUI) ReadPrompt() (string, error) {
	for {
		line, err := t.rl.Readline()
		if err == readline.ErrInterrupt {
			continue
		}
		if err != nil {
			return "", io.EOF
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return line, nil
	}
}

// ParseSlashCommand checks if input starts with "/" and parses it into a
// SlashCommand. Returns nil, false if the input is not a slash command.
func ParseSlashCommand(input string) (*SlashCommand, bool) {
	if !strings.HasPrefix(input, "/") {
		return nil, false
	}
	trimmed := strings.TrimPrefix(input, "/")
	parts := strings.SplitN(trimmed, " ", 2)
	cmd := &SlashCommand{Name: parts[0]}
	if len(parts) > 1 {
		cmd.Args = strings.TrimSpace(parts[1])
	}
	return cmd, true
}

// RenderBanner displays the welcome banner with session info, model/token
// details, session usage, and keyboard shortcuts.
func (t *TUI) RenderBanner(workDir string, toolCount int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	width := 64
	border := strings.Repeat(BoxHorizontal, width)

	printRow := func(line string) {
		plain := stripANSI(line)
		pad := width - 2 - len([]rune(plain))
		if pad < 0 {
			pad = 0
		}
		fmt.Fprintf(t.w, "%s%s%s %s%s %s%s%s\n",
			Cyan, BoxVertical, Reset,
			line, strings.Repeat(" ", pad),
			Cyan, BoxVertical, Reset)
	}

	printEmpty := func() { printRow("") }

	fmt.Fprintf(t.w, "\n%s%s%s%s%s\n", Cyan, BoxTopLeft, border, BoxTopRight, Reset)

	printRow(fmt.Sprintf("%s%sLLM Code Solver%s", Bold, BrightCyan, Reset))
	printEmpty()
	printRow(fmt.Sprintf("Working dir:  %s%s%s", Bold, workDir, Reset))

	model := t.model
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	printRow(fmt.Sprintf("Model:        %s%s%s", Bold, model, Reset))
	printRow(fmt.Sprintf("Max tokens:   %s%d%s per response", Bold, t.maxTokens, Reset))
	printRow(fmt.Sprintf("Tools:        %s%d%s available", Bold, toolCount, Reset))

	printEmpty()
	totalTokens := t.SessionTokensIn + t.SessionTokensOut
	if totalTokens > 0 {
		printRow(fmt.Sprintf("%s%sSession usage:%s", Bold, Yellow, Reset))
		printRow(fmt.Sprintf("  Tokens: %s%d%s in + %s%d%s out = %s%d%s total",
			BrightCyan, t.SessionTokensIn, Reset,
			BrightCyan, t.SessionTokensOut, Reset,
			Bold, totalTokens, Reset))
		printRow(fmt.Sprintf("  Calls:  %s%d%s tool calls across %s%d%s prompts",
			BrightCyan, t.SessionToolCalls, Reset,
			BrightCyan, t.SessionPrompts, Reset))
	} else {
		printRow(fmt.Sprintf("%sSession: no tokens used yet%s", Dim, Reset))
	}

	printEmpty()
	printRow(fmt.Sprintf("%sCtrl+C%s interrupt  %sCtrl+D%s exit  %s/help%s this  %s/clear%s reset",
		Bold, Reset, Bold, Reset, Bold, Reset, Bold, Reset))

	fmt.Fprintf(t.w, "%s%s%s%s%s\n\n", Cyan, BoxBottomLeft, border, BoxBottomRight, Reset)
}

// Close releases readline resources.
func (t *TUI) Close() {
	if t.rl != nil {
		t.rl.Close()
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func stripANSI(s string) string {
	var b strings.Builder
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
		b.WriteRune(r)
	}
	return b.String()
}
