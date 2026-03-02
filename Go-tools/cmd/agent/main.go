package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"tools/internal/agent"
	"tools/internal/llm"
	"tools/internal/tools"
	"tools/internal/tui"
)

// Compile-time check: TUI must satisfy the agent.Renderer interface.
var _ agent.Renderer = (*tui.TUI)(nil)

func main() {
	os.Exit(run())
}

func run() int {
	// ---------------------------------------------------------------
	// 1. Parse environment variables
	// ---------------------------------------------------------------
	providerName := envOr("LLM_PROVIDER", "anthropic")
	model := envOr("LLM_MODEL", "")
	workDir := envOr("WORK_DIR", "")
	maxTurnsStr := envOr("MAX_TURNS", "50")

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if providerName == "openai" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}

	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot determine working directory: %v\n", err)
			return 1
		}
		workDir = wd
	}

	maxTurns, err := strconv.Atoi(maxTurnsStr)
	if err != nil || maxTurns <= 0 {
		maxTurns = 50
	}

	// ---------------------------------------------------------------
	// 2. Create LLM provider
	// ---------------------------------------------------------------
	provider, err := llm.NewProvider(providerName, apiKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	// ---------------------------------------------------------------
	// 3. Load tools
	// ---------------------------------------------------------------
	allTools := tools.AllTools()

	// ---------------------------------------------------------------
	// 4. Build agent config
	// ---------------------------------------------------------------
	cfg := agent.DefaultConfig()
	if model != "" {
		cfg.Model = model
	}
	cfg.MaxTurns = maxTurns

	systemPrompt, err := buildSystemPrompt(workDir, allTools)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to build system prompt: %v\n", err)
		return 1
	}
	cfg.SystemPrompt = systemPrompt

	// ---------------------------------------------------------------
	// 5. Create terminal UI
	// ---------------------------------------------------------------
	ui, err := tui.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer ui.Close()

	// ---------------------------------------------------------------
	// 6. Set session info and show welcome banner
	// ---------------------------------------------------------------
	ui.SetSessionInfo(cfg.Model, cfg.MaxTokens, len(allTools.Tools))
	ui.RenderBanner(workDir, len(allTools.Tools))

	// ---------------------------------------------------------------
	// 7. Interactive REPL loop
	// ---------------------------------------------------------------
	var messages []llm.Message

	for {
		input, err := ui.ReadPrompt()
		if err == io.EOF {
			fmt.Fprintln(os.Stdout, "\nGoodbye.")
			return 0
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}

		if cmd, ok := tui.ParseSlashCommand(input); ok {
			handleSlashCommand(cmd, ui, &messages, workDir, len(allTools.Tools))
			continue
		}

		// Append user message to conversation
		messages = append(messages, llm.NewUserMessage(input))

		// Create a cancellable context for this agent run.
		// SIGINT (Ctrl+C) cancels the current run, returning to the prompt.
		ctx, cancel := context.WithCancel(context.Background())
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT)
		go func() {
			select {
			case <-sigCh:
				cancel()
			case <-ctx.Done():
			}
			signal.Stop(sigCh)
		}()

		err = agent.RunAgentLoop(ctx, provider, allTools, cfg, &messages, ui)
		cancel()

		if err != nil {
			if ctx.Err() != nil {
				fmt.Fprintf(os.Stdout, "\n  %s%sInterrupted%s\n\n", tui.Bold, tui.Yellow, tui.Reset)
			} else {
				fmt.Fprintf(os.Stdout, "\n  %s%s✗ Error: %s%s\n\n", tui.Bold, tui.Red, err.Error(), tui.Reset)
			}
		}
	}
}

// handleSlashCommand processes interactive slash commands.
// Full implementations for /commit, /revert, /log arrive in phase 2.
func handleSlashCommand(cmd *tui.SlashCommand, ui *tui.TUI, messages *[]llm.Message, workDir string, toolCount int) {
	switch cmd.Name {
	case "help":
		ui.RenderBanner(workDir, toolCount)

	case "clear":
		*messages = nil
		fmt.Fprintf(os.Stdout, "  %sConversation cleared.%s\n\n", tui.Dim, tui.Reset)

	case "status", "diff", "commit", "revert", "log":
		fmt.Fprintf(os.Stdout, "  %s/%s will be available in phase 2 (checkpoints).%s\n\n", tui.Dim, cmd.Name, tui.Reset)

	default:
		fmt.Fprintf(os.Stdout, "  %sUnknown command: /%s — type /help for available commands.%s\n\n", tui.Yellow, cmd.Name, tui.Reset)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
