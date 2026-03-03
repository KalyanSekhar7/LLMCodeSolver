package agent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	checkpointPrefix   = "agent: "
	agentDir           = ".agent"
	checkpointFileName = "checkpoints.md"
	gitTimeout         = 30 * time.Second
)

// Checkpoint represents a single saved checkpoint.
type Checkpoint struct {
	Hash       string
	BeforeHash string
	Message    string
	Timestamp  time.Time
	Prompt     string // the user prompt that triggered this turn
}

// CheckpointManager handles git-based checkpointing: auto-committing after
// each agent turn, reverting to previous states, and maintaining a persistent
// checkpoint log that survives git resets.
type CheckpointManager struct {
	workDir        string
	beforeTurnHash string // HEAD recorded before the current turn
	checkpoints    []Checkpoint
}

// NewCheckpointManager creates a manager for the given working directory.
// It ensures the .agent/ directory exists (gitignored) and loads any existing
// checkpoint log.
func NewCheckpointManager(workDir string) (*CheckpointManager, error) {
	cm := &CheckpointManager{workDir: workDir}

	if err := cm.ensureAgentDir(); err != nil {
		return nil, fmt.Errorf("checkpoint init: %w", err)
	}

	cm.loadCheckpointLog()
	return cm, nil
}

// RecordBeforeTurn snapshots the current HEAD so we can revert back to it.
func (cm *CheckpointManager) RecordBeforeTurn() {
	if hash, err := cm.currentHEAD(); err == nil {
		cm.beforeTurnHash = hash
	}
}

// BeforeTurnHash returns the HEAD hash recorded before the last turn started.
func (cm *CheckpointManager) BeforeTurnHash() string {
	return cm.beforeTurnHash
}

// CreateCheckpoint stages all changes and commits with an "agent:" prefixed
// message. It also appends an entry to the persistent checkpoint log.
// Returns the commit hash, or empty string if there was nothing to commit.
func (cm *CheckpointManager) CreateCheckpoint(ctx context.Context, message, userPrompt string) (string, error) {
	// Stage everything
	if _, _, err := cm.runGit(ctx, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}

	// Check if there's anything to commit
	status, _, _ := cm.runGit(ctx, "status", "--porcelain")
	if strings.TrimSpace(status) == "" {
		return "", nil
	}

	commitMsg := checkpointPrefix + message
	_, stderr, err := cm.runGit(ctx, "commit", "-m", commitMsg)
	if err != nil {
		if strings.Contains(stderr, "nothing to commit") {
			return "", nil
		}
		return "", fmt.Errorf("git commit: %w", err)
	}

	hash, err := cm.currentHEAD()
	if err != nil {
		return "", err
	}

	cp := Checkpoint{
		Hash:       hash,
		BeforeHash: cm.beforeTurnHash,
		Message:    message,
		Timestamp:  time.Now(),
		Prompt:     userPrompt,
	}
	cm.checkpoints = append(cm.checkpoints, cp)
	cm.writeCheckpointLog()

	return hash, nil
}

// RevertToCheckpoint does a hard reset to the given hash.
func (cm *CheckpointManager) RevertToCheckpoint(ctx context.Context, hash string) error {
	if hash == "" {
		if cm.beforeTurnHash == "" {
			return fmt.Errorf("no checkpoint to revert to")
		}
		hash = cm.beforeTurnHash
	}

	_, stderr, err := cm.runGit(ctx, "reset", "--hard", hash)
	if err != nil {
		return fmt.Errorf("git reset --hard %s: %s", hash, stderr)
	}

	return nil
}

// RevertLast reverts to the state before the most recent agent turn.
func (cm *CheckpointManager) RevertLast(ctx context.Context) (string, error) {
	if len(cm.checkpoints) == 0 {
		return "", fmt.Errorf("no checkpoints to revert")
	}

	last := cm.checkpoints[len(cm.checkpoints)-1]
	target := last.BeforeHash
	if target == "" {
		return "", fmt.Errorf("no before-hash recorded for last checkpoint")
	}

	if err := cm.RevertToCheckpoint(ctx, target); err != nil {
		return "", err
	}

	return target, nil
}

// ListCheckpoints returns all recorded checkpoints.
func (cm *CheckpointManager) ListCheckpoints() []Checkpoint {
	return cm.checkpoints
}

// Status returns the current git status (porcelain format).
func (cm *CheckpointManager) Status(ctx context.Context) (string, error) {
	stdout, _, err := cm.runGit(ctx, "status", "--short")
	if err != nil {
		return "", err
	}
	return stdout, nil
}

// Diff returns the current git diff.
func (cm *CheckpointManager) Diff(ctx context.Context) (string, error) {
	stdout, _, err := cm.runGit(ctx, "diff")
	if err != nil {
		return "", err
	}

	staged, _, _ := cm.runGit(ctx, "diff", "--cached")
	if staged != "" {
		if stdout != "" {
			stdout += "\n"
		}
		stdout += "--- Staged changes ---\n" + staged
	}

	return stdout, nil
}

// Log returns recent git log entries.
func (cm *CheckpointManager) Log(ctx context.Context, n int) (string, error) {
	if n <= 0 {
		n = 20
	}
	stdout, _, err := cm.runGit(ctx, "log", "--oneline", fmt.Sprintf("-n%d", n))
	if err != nil {
		return "", err
	}
	return stdout, nil
}

// CommitWithMessage creates a user-initiated commit (not an auto-checkpoint).
func (cm *CheckpointManager) CommitWithMessage(ctx context.Context, message string) (string, error) {
	if _, _, err := cm.runGit(ctx, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}

	_, stderr, err := cm.runGit(ctx, "commit", "-m", message)
	if err != nil {
		if strings.Contains(stderr, "nothing to commit") {
			return "", fmt.Errorf("nothing to commit")
		}
		return "", fmt.Errorf("git commit: %s", stderr)
	}

	hash, err := cm.currentHEAD()
	if err != nil {
		return "", err
	}
	return hash, nil
}

// MessageHash returns a short hash of the conversation messages, useful for
// identifying the conversation state at a checkpoint.
func MessageHash(messages []byte) string {
	h := sha256.Sum256(messages)
	return fmt.Sprintf("%x", h[:8])
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (cm *CheckpointManager) currentHEAD() (string, error) {
	stdout, _, err := cm.runGit(context.Background(), "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stdout), nil
}

func (cm *CheckpointManager) runGit(ctx context.Context, args ...string) (string, string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "git", args...)
	cmd.Dir = cm.workDir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return stdout.String(), stderr.String(), fmt.Errorf("git command timed out")
		}
		if _, ok := err.(*exec.ExitError); ok {
			return stdout.String(), stderr.String(), fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
		}
		return "", "", fmt.Errorf("failed to run git: %w", err)
	}

	return stdout.String(), stderr.String(), nil
}

func (cm *CheckpointManager) ensureAgentDir() error {
	dir := filepath.Join(cm.workDir, agentDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Ensure .agent/ is gitignored so checkpoint.md survives reverts
	gitignorePath := filepath.Join(cm.workDir, ".gitignore")
	content, _ := os.ReadFile(gitignorePath)
	if !strings.Contains(string(content), agentDir) {
		entry := "\n# LLM agent checkpoint data\n" + agentDir + "/\n"
		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString(entry); err != nil {
			return err
		}
	}

	return nil
}

func (cm *CheckpointManager) checkpointLogPath() string {
	return filepath.Join(cm.workDir, agentDir, checkpointFileName)
}

func (cm *CheckpointManager) writeCheckpointLog() {
	var sb strings.Builder
	sb.WriteString("# Agent Checkpoints\n\n")
	sb.WriteString("This file records every checkpoint created by the agent.\n")
	sb.WriteString("It persists across git reverts (stored in .agent/, which is gitignored).\n")
	sb.WriteString("The agent can read this file to understand session history and revert to any checkpoint.\n\n")
	sb.WriteString("---\n\n")

	for i, cp := range cm.checkpoints {
		sb.WriteString(fmt.Sprintf("## Checkpoint %d\n\n", i+1))
		sb.WriteString(fmt.Sprintf("- **Hash:** `%s`\n", cp.Hash))
		if cp.BeforeHash != "" {
			sb.WriteString(fmt.Sprintf("- **Before:** `%s`\n", cp.BeforeHash))
		}
		sb.WriteString(fmt.Sprintf("- **Time:** %s\n", cp.Timestamp.Format("2006-01-02 15:04:05")))
		if cp.Prompt != "" {
			prompt := cp.Prompt
			if len(prompt) > 200 {
				prompt = prompt[:197] + "..."
			}
			sb.WriteString(fmt.Sprintf("- **Prompt:** %s\n", prompt))
		}
		sb.WriteString(fmt.Sprintf("- **Summary:** %s\n", cp.Message))
		sb.WriteString("\n---\n\n")
	}

	os.WriteFile(cm.checkpointLogPath(), []byte(sb.String()), 0o644)
}

func (cm *CheckpointManager) loadCheckpointLog() {
	data, err := os.ReadFile(cm.checkpointLogPath())
	if err != nil {
		return
	}

	// Parse checkpoint entries from the markdown log.
	// Each checkpoint block starts with "## Checkpoint" and has key-value lines.
	blocks := strings.Split(string(data), "## Checkpoint ")
	for _, block := range blocks[1:] { // skip header
		var cp Checkpoint
		for _, line := range strings.Split(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- **Hash:** `") {
				cp.Hash = strings.TrimSuffix(strings.TrimPrefix(line, "- **Hash:** `"), "`")
			} else if strings.HasPrefix(line, "- **Before:** `") {
				cp.BeforeHash = strings.TrimSuffix(strings.TrimPrefix(line, "- **Before:** `"), "`")
			} else if strings.HasPrefix(line, "- **Time:** ") {
				ts := strings.TrimPrefix(line, "- **Time:** ")
				cp.Timestamp, _ = time.Parse("2006-01-02 15:04:05", ts)
			} else if strings.HasPrefix(line, "- **Prompt:** ") {
				cp.Prompt = strings.TrimPrefix(line, "- **Prompt:** ")
			} else if strings.HasPrefix(line, "- **Summary:** ") {
				cp.Message = strings.TrimPrefix(line, "- **Summary:** ")
			}
		}
		if cp.Hash != "" {
			cm.checkpoints = append(cm.checkpoints, cp)
		}
	}
}
