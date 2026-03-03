package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
	// 6. Initialize checkpoint manager
	// ---------------------------------------------------------------
	cpMgr, err := agent.NewCheckpointManager(workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: checkpoints disabled: %v\n", err)
	}

	// ---------------------------------------------------------------
	// 7. Set session info and show welcome banner
	// ---------------------------------------------------------------
	ui.SetSessionInfo(cfg.Model, cfg.MaxTokens, len(allTools.Tools))
	ui.RenderBanner(workDir, len(allTools.Tools))

	// ---------------------------------------------------------------
	// 8. Interactive REPL loop
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
			handleSlashCommand(cmd, ui, cpMgr, &messages, workDir, len(allTools.Tools))
			continue
		}

		// Record HEAD before this turn so we can revert if needed
		if cpMgr != nil {
			cpMgr.RecordBeforeTurn()
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

		// Auto-checkpoint after each agent turn
		if cpMgr != nil {
			summary := buildCheckpointSummary(input, messages)
			msgJSON, _ := json.Marshal(messages)
			msgHash := agent.MessageHash(msgJSON)
			commitMsg := fmt.Sprintf("%s [conv:%s]", summary, msgHash)

			hash, cpErr := cpMgr.CreateCheckpoint(context.Background(), commitMsg, input)
			if cpErr != nil {
				fmt.Fprintf(os.Stdout, "  %s%s✗ Checkpoint failed: %s%s\n\n",
					tui.Bold, tui.Red, cpErr.Error(), tui.Reset)
			} else if hash != "" {
				fmt.Fprintf(os.Stdout, "  %s%s✔ Checkpoint: %s%s %s\"%s\"%s\n\n",
					tui.Dim, tui.Green, tui.Reset,
					hash,
					tui.Dim, summary, tui.Reset)
			}
		}
	}
}

// buildCheckpointSummary generates a short commit message from the user prompt
// and the last assistant response.
func buildCheckpointSummary(userPrompt string, messages []llm.Message) string {
	prompt := userPrompt
	if len(prompt) > 80 {
		prompt = prompt[:77] + "..."
	}

	// Look for the last assistant text in messages
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleAssistant {
			for _, block := range messages[i].Content {
				if block.Type == llm.ContentTypeText && block.Text != "" {
					text := block.Text
					if len(text) > 80 {
						text = text[:77] + "..."
					}
					return text
				}
			}
		}
	}

	return prompt
}

// handleSlashCommand processes interactive slash commands.
func handleSlashCommand(cmd *tui.SlashCommand, ui *tui.TUI, cpMgr *agent.CheckpointManager, messages *[]llm.Message, workDir string, toolCount int) {
	ctx := context.Background()

	switch cmd.Name {
	case "help":
		ui.RenderBanner(workDir, toolCount)

	case "clear":
		*messages = nil
		fmt.Fprintf(os.Stdout, "  %sConversation cleared.%s\n\n", tui.Dim, tui.Reset)

	case "status":
		if cpMgr == nil {
			fmt.Fprintf(os.Stdout, "  %sCheckpoints not available.%s\n\n", tui.Yellow, tui.Reset)
			return
		}
		status, err := cpMgr.Status(ctx)
		if err != nil {
			fmt.Fprintf(os.Stdout, "  %s%s✗ %s%s\n\n", tui.Bold, tui.Red, err.Error(), tui.Reset)
			return
		}
		if strings.TrimSpace(status) == "" {
			fmt.Fprintf(os.Stdout, "  %sWorking tree clean.%s\n\n", tui.Green, tui.Reset)
		} else {
			fmt.Fprintf(os.Stdout, "  %s%sGit status:%s\n", tui.Bold, tui.Cyan, tui.Reset)
			for _, line := range strings.Split(strings.TrimRight(status, "\n"), "\n") {
				fmt.Fprintf(os.Stdout, "    %s\n", colorizeStatusLine(line))
			}
			fmt.Fprintln(os.Stdout)
		}

	case "diff":
		if cpMgr == nil {
			fmt.Fprintf(os.Stdout, "  %sCheckpoints not available.%s\n\n", tui.Yellow, tui.Reset)
			return
		}
		diff, err := cpMgr.Diff(ctx)
		if err != nil {
			fmt.Fprintf(os.Stdout, "  %s%s✗ %s%s\n\n", tui.Bold, tui.Red, err.Error(), tui.Reset)
			return
		}
		if strings.TrimSpace(diff) == "" {
			fmt.Fprintf(os.Stdout, "  %sNo changes.%s\n\n", tui.Dim, tui.Reset)
		} else {
			fmt.Fprintf(os.Stdout, "  %s%sDiff:%s\n", tui.Bold, tui.Cyan, tui.Reset)
			for _, line := range strings.Split(strings.TrimRight(diff, "\n"), "\n") {
				fmt.Fprintf(os.Stdout, "    %s\n", colorizeDiffLine(line))
			}
			fmt.Fprintln(os.Stdout)
		}

	case "commit":
		if cpMgr == nil {
			fmt.Fprintf(os.Stdout, "  %sCheckpoints not available.%s\n\n", tui.Yellow, tui.Reset)
			return
		}
		msg := cmd.Args
		if msg == "" {
			fmt.Fprintf(os.Stdout, "  %sUsage: /commit <message>%s\n\n", tui.Yellow, tui.Reset)
			return
		}
		// Strip -m flag if present (user might type /commit -m "msg")
		msg = strings.TrimPrefix(msg, "-m ")
		msg = strings.Trim(msg, "\"'")

		hash, err := cpMgr.CommitWithMessage(ctx, msg)
		if err != nil {
			fmt.Fprintf(os.Stdout, "  %s%s✗ %s%s\n\n", tui.Bold, tui.Red, err.Error(), tui.Reset)
		} else {
			fmt.Fprintf(os.Stdout, "  %s%s✔ Committed:%s %s\n\n", tui.Bold, tui.Green, tui.Reset, hash)
		}

	case "revert":
		if cpMgr == nil {
			fmt.Fprintf(os.Stdout, "  %sCheckpoints not available.%s\n\n", tui.Yellow, tui.Reset)
			return
		}
		if cmd.Args != "" {
			// Revert to specific hash
			if err := cpMgr.RevertToCheckpoint(ctx, cmd.Args); err != nil {
				fmt.Fprintf(os.Stdout, "  %s%s✗ %s%s\n\n", tui.Bold, tui.Red, err.Error(), tui.Reset)
			} else {
				fmt.Fprintf(os.Stdout, "  %s%s✔ Reverted to %s%s\n\n", tui.Bold, tui.Green, cmd.Args, tui.Reset)
			}
		} else {
			// Revert to before last agent turn
			hash, err := cpMgr.RevertLast(ctx)
			if err != nil {
				fmt.Fprintf(os.Stdout, "  %s%s✗ %s%s\n\n", tui.Bold, tui.Red, err.Error(), tui.Reset)
			} else {
				fmt.Fprintf(os.Stdout, "  %s%s✔ Reverted to %s%s (before last agent run)\n\n",
					tui.Bold, tui.Green, hash, tui.Reset)
			}
		}

	case "log":
		if cpMgr == nil {
			fmt.Fprintf(os.Stdout, "  %sCheckpoints not available.%s\n\n", tui.Yellow, tui.Reset)
			return
		}
		checkpoints := cpMgr.ListCheckpoints()
		if len(checkpoints) == 0 {
			// Fall back to git log
			log, err := cpMgr.Log(ctx, 20)
			if err != nil {
				fmt.Fprintf(os.Stdout, "  %s%s✗ %s%s\n\n", tui.Bold, tui.Red, err.Error(), tui.Reset)
				return
			}
			fmt.Fprintf(os.Stdout, "  %s%sGit log:%s\n", tui.Bold, tui.Cyan, tui.Reset)
			for _, line := range strings.Split(strings.TrimRight(log, "\n"), "\n") {
				fmt.Fprintf(os.Stdout, "    %s\n", line)
			}
			fmt.Fprintln(os.Stdout)
		} else {
			fmt.Fprintf(os.Stdout, "  %s%sAgent checkpoints:%s\n\n", tui.Bold, tui.Cyan, tui.Reset)
			for i := len(checkpoints) - 1; i >= 0; i-- {
				cp := checkpoints[i]
				fmt.Fprintf(os.Stdout, "    %s%s%s %s%s%s %s\n",
					tui.Bold, cp.Hash, tui.Reset,
					tui.Dim, cp.Timestamp.Format("15:04:05"), tui.Reset,
					cp.Message)
				if cp.BeforeHash != "" {
					fmt.Fprintf(os.Stdout, "    %s  (before: %s)%s\n", tui.Dim, cp.BeforeHash, tui.Reset)
				}
			}
			fmt.Fprintln(os.Stdout)
		}

	default:
		fmt.Fprintf(os.Stdout, "  %sUnknown command: /%s — type /help for available commands.%s\n\n", tui.Yellow, cmd.Name, tui.Reset)
	}
}

// colorizeStatusLine adds ANSI colors to git status --short output lines.
func colorizeStatusLine(line string) string {
	if len(line) < 3 {
		return line
	}
	indicator := line[:2]
	rest := line[2:]

	switch {
	case strings.HasPrefix(indicator, "?"):
		return tui.Red + line + tui.Reset
	case strings.HasPrefix(indicator, "M"), strings.HasPrefix(indicator, "A"):
		return tui.Green + line + tui.Reset
	case strings.HasPrefix(indicator, "D"):
		return tui.Red + line + tui.Reset
	case strings.Contains(indicator, "M"):
		return tui.Yellow + indicator + tui.Reset + rest
	default:
		return line
	}
}

// colorizeDiffLine adds ANSI colors to unified diff output lines.
func colorizeDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return tui.Bold + line + tui.Reset
	case strings.HasPrefix(line, "+"):
		return tui.Green + line + tui.Reset
	case strings.HasPrefix(line, "-"):
		return tui.Red + line + tui.Reset
	case strings.HasPrefix(line, "@@"):
		return tui.Cyan + line + tui.Reset
	default:
		return line
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
