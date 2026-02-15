package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Safety: command allowlist / denylist
// ---------------------------------------------------------------------------

// defaultDenyPatterns blocks dangerous commands by default.
// These can be overridden per-deployment via configuration.
var defaultDenyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+-rf\s+/\s*$`),      // rm -rf /
	regexp.MustCompile(`\bmkfs\b`),                   // format filesystems
	regexp.MustCompile(`\bdd\s+.*of=/dev/`),          // raw disk writes
	regexp.MustCompile(`>\s*/dev/sd[a-z]`),            // redirect to disk
	regexp.MustCompile(`\b:(){ :|:& };:`),             // fork bomb
	regexp.MustCompile(`\bshutdown\b`),                // shutdown
	regexp.MustCompile(`\breboot\b`),                  // reboot
	regexp.MustCompile(`\bsystemctl\s+(start|stop|disable|enable|mask)\b`), // service management
}

// sensitiveEnvKeys are redacted from get_env by default.
var sensitiveEnvKeys = map[string]bool{
	"AWS_SECRET_ACCESS_KEY":     true,
	"AWS_SESSION_TOKEN":         true,
	"GITHUB_TOKEN":              true,
	"GH_TOKEN":                  true,
	"OPENAI_API_KEY":            true,
	"ANTHROPIC_API_KEY":         true,
	"DATABASE_URL":              true,
	"DB_PASSWORD":               true,
	"SECRET_KEY":                true,
	"PRIVATE_KEY":               true,
	"API_KEY":                   true,
	"API_SECRET":                true,
	"PASSWORD":                  true,
	"TOKEN":                     true,
	"STRIPE_SECRET_KEY":         true,
	"SENDGRID_API_KEY":          true,
}

const (
	defaultTimeoutMs   = 30_000        // 30 seconds
	maxOutputBytes     = 1 * 1024 * 1024 // 1 MB output cap
)

// ---------------------------------------------------------------------------
// run_command — execute a single command with args
// ---------------------------------------------------------------------------

type RunCommandInput struct {
	Cmd          string            `json:"cmd"`
	Args         []string          `json:"args"`
	Cwd          string            `json:"cwd"`
	EnvOverrides map[string]string `json:"env_overrides"`
	TimeoutMs    int               `json:"timeout_ms"`
}

type RunCommandOutput struct {
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Truncated bool   `json:"truncated"`
	TimedOut  bool   `json:"timed_out"`
	DurationMs int64 `json:"duration_ms"`
}

type RunCommandTool struct{}

func (t *RunCommandTool) Name() string { return "run_command" }
func (t *RunCommandTool) Description() string {
	return "Executes a command with arguments. Supports timeouts, env overrides, and output caps."
}

func (t *RunCommandTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in RunCommandInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Cmd == "" {
		return nil, errors.New("cmd required")
	}

	// Safety: check denylist
	fullCmd := in.Cmd + " " + strings.Join(in.Args, " ")
	if blocked, pattern := isDenied(fullCmd); blocked {
		return nil, fmt.Errorf("command blocked by safety denylist (matched: %s)", pattern)
	}

	// Timeout
	timeoutMs := in.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultTimeoutMs
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, in.Cmd, in.Args...)

	// Working directory
	if in.Cwd != "" {
		absPath, err := filepath.Abs(in.Cwd)
		if err != nil {
			return nil, fmt.Errorf("invalid cwd: %w", err)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("cwd does not exist or is not a directory: %s", in.Cwd)
		}
		cmd.Dir = absPath
	}

	// Environment: inherit current + apply overrides
	if len(in.EnvOverrides) > 0 {
		env := os.Environ()
		for k, v := range in.EnvOverrides {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	out := RunCommandOutput{
		DurationMs: duration.Milliseconds(),
	}

	// Check for timeout
	if cmdCtx.Err() == context.DeadlineExceeded {
		out.TimedOut = true
		out.ExitCode = -1
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			out.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to run command: %w", err)
		}
	}

	// Cap output
	out.Stdout, out.Truncated = capOutput(stdout.Bytes())
	stderrStr, stderrTrunc := capOutput(stderr.Bytes())
	out.Stderr = stderrStr
	if stderrTrunc {
		out.Truncated = true
	}

	return json.Marshal(out)
}

func isDenied(fullCmd string) (bool, string) {
	for _, re := range defaultDenyPatterns {
		if re.MatchString(fullCmd) {
			return true, re.String()
		}
	}
	return false, ""
}

func capOutput(data []byte) (string, bool) {
	if len(data) > maxOutputBytes {
		return string(data[:maxOutputBytes]), true
	}
	return string(data), false
}

// ---------------------------------------------------------------------------
// run_script — execute a multi-line shell script
// ---------------------------------------------------------------------------

type RunScriptInput struct {
	Script    string            `json:"script"`
	Shell     string            `json:"shell"` // "bash", "sh", "zsh", "pwsh", "powershell"
	Cwd       string            `json:"cwd"`
	EnvOverrides map[string]string `json:"env_overrides"`
	TimeoutMs int               `json:"timeout_ms"`
}

type RunScriptOutput struct {
	ExitCode   int    `json:"exit_code"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	Truncated  bool   `json:"truncated"`
	TimedOut   bool   `json:"timed_out"`
	DurationMs int64  `json:"duration_ms"`
	ShellUsed  string `json:"shell_used"`
}

type RunScriptTool struct{}

func (t *RunScriptTool) Name() string { return "run_script" }
func (t *RunScriptTool) Description() string {
	return "Executes a multi-line shell script. Supports shell selection, timeouts, and env overrides."
}

func (t *RunScriptTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in RunScriptInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Script == "" {
		return nil, errors.New("script required")
	}

	// Safety: check each line of the script against denylist
	for _, line := range strings.Split(in.Script, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if blocked, pattern := isDenied(line); blocked {
			return nil, fmt.Errorf("script line blocked by safety denylist (matched: %s): %s", pattern, line)
		}
	}

	// Resolve shell
	shell, shellArg := resolveShell(in.Shell)

	// Timeout
	timeoutMs := in.TimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultTimeoutMs
	}
	timeout := time.Duration(timeoutMs) * time.Millisecond
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, shell, shellArg, in.Script)

	// Working directory
	if in.Cwd != "" {
		absPath, err := filepath.Abs(in.Cwd)
		if err != nil {
			return nil, fmt.Errorf("invalid cwd: %w", err)
		}
		if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
			return nil, fmt.Errorf("cwd does not exist or is not a directory: %s", in.Cwd)
		}
		cmd.Dir = absPath
	}

	// Environment
	if len(in.EnvOverrides) > 0 {
		env := os.Environ()
		for k, v := range in.EnvOverrides {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	out := RunScriptOutput{
		DurationMs: duration.Milliseconds(),
		ShellUsed:  shell,
	}

	if cmdCtx.Err() == context.DeadlineExceeded {
		out.TimedOut = true
		out.ExitCode = -1
	} else if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			out.ExitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to run script: %w", err)
		}
	}

	out.Stdout, out.Truncated = capOutput(stdout.Bytes())
	stderrStr, stderrTrunc := capOutput(stderr.Bytes())
	out.Stderr = stderrStr
	if stderrTrunc {
		out.Truncated = true
	}

	return json.Marshal(out)
}

func resolveShell(requested string) (string, string) {
	switch strings.ToLower(requested) {
	case "bash":
		if p, err := exec.LookPath("bash"); err == nil {
			return p, "-c"
		}
	case "zsh":
		if p, err := exec.LookPath("zsh"); err == nil {
			return p, "-c"
		}
	case "sh":
		if p, err := exec.LookPath("sh"); err == nil {
			return p, "-c"
		}
	case "pwsh", "powershell":
		// Try pwsh first (cross-platform PowerShell), then powershell (Windows)
		if p, err := exec.LookPath("pwsh"); err == nil {
			return p, "-Command"
		}
		if p, err := exec.LookPath("powershell"); err == nil {
			return p, "-Command"
		}
	case "fish":
		if p, err := exec.LookPath("fish"); err == nil {
			return p, "-c"
		}
	}

	// Default: pick the best available shell for the platform
	if runtime.GOOS == "windows" {
		if p, err := exec.LookPath("pwsh"); err == nil {
			return p, "-Command"
		}
		if p, err := exec.LookPath("powershell"); err == nil {
			return p, "-Command"
		}
		return "cmd", "/C"
	}

	// Unix: prefer bash > zsh > sh
	for _, sh := range []string{"bash", "zsh", "sh"} {
		if p, err := exec.LookPath(sh); err == nil {
			return p, "-c"
		}
	}

	return "/bin/sh", "-c"
}

// ---------------------------------------------------------------------------
// which — detect if a binary exists and return its path
// ---------------------------------------------------------------------------

type WhichInput struct {
	Name string `json:"name"`
}

type WhichOutput struct {
	Found   bool   `json:"found"`
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
}

type WhichTool struct{}

func (t *WhichTool) Name() string { return "which" }
func (t *WhichTool) Description() string {
	return "Checks if a binary is installed and returns its path."
}

func (t *WhichTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in WhichInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Name == "" {
		return nil, errors.New("name required")
	}

	path, err := exec.LookPath(in.Name)
	if err != nil {
		return json.Marshal(WhichOutput{Found: false})
	}

	out := WhichOutput{
		Found: true,
		Path:  path,
	}

	// Best-effort: try to get version info
	out.Version = detectVersion(ctx, path, in.Name)

	return json.Marshal(out)
}

func detectVersion(ctx context.Context, path, name string) string {
	// Try common version flags
	for _, flag := range []string{"--version", "-version", "version", "-V"} {
		verCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		cmd := exec.CommandContext(verCtx, path, flag)
		output, err := cmd.CombinedOutput()
		cancel()

		if err == nil && len(output) > 0 {
			// Return first non-empty line
			for _, line := range strings.Split(string(output), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					// Cap at a reasonable length
					if len(line) > 200 {
						line = line[:200]
					}
					return line
				}
			}
		}
	}

	return ""
}

// ---------------------------------------------------------------------------
// set_env — set an environment variable for the current process
// ---------------------------------------------------------------------------

type SetEnvInput struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type SetEnvOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type SetEnvTool struct{}

func (t *SetEnvTool) Name() string { return "set_env" }
func (t *SetEnvTool) Description() string {
	return "Sets an environment variable for the current process."
}

func (t *SetEnvTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in SetEnvInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	if in.Key == "" {
		return nil, errors.New("key required")
	}

	// Safety: warn if setting a sensitive key
	if isSensitiveEnvKey(in.Key) {
		// Still allow it, but note the risk
		if err := os.Setenv(in.Key, in.Value); err != nil {
			return nil, err
		}
		return json.Marshal(SetEnvOutput{
			Success: true,
			Message: fmt.Sprintf("warning: '%s' is a sensitive key — value will be redacted from get_env", in.Key),
		})
	}

	if err := os.Setenv(in.Key, in.Value); err != nil {
		return nil, err
	}

	return json.Marshal(SetEnvOutput{Success: true})
}

// ---------------------------------------------------------------------------
// get_env — read environment variables with redaction for secrets
// ---------------------------------------------------------------------------

type GetEnvInput struct {
	Keys           []string `json:"keys"`            // specific keys to read; empty = return all safe vars
	IncludeSensitive bool   `json:"include_sensitive"` // if true, return sensitive values unredacted
}

type EnvEntry struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Redacted bool   `json:"redacted"`
}

type GetEnvOutput struct {
	Variables []EnvEntry `json:"variables"`
	Count     int        `json:"count"`
}

type GetEnvTool struct{}

func (t *GetEnvTool) Name() string { return "get_env" }
func (t *GetEnvTool) Description() string {
	return "Reads environment variables. Sensitive values are redacted by default."
}

func (t *GetEnvTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GetEnvInput
	if err := json.Unmarshal(input, &in); err != nil {
		return nil, err
	}

	var entries []EnvEntry

	if len(in.Keys) > 0 {
		// Return specific keys
		for _, key := range in.Keys {
			val, exists := os.LookupEnv(key)
			if !exists {
				continue
			}

			entry := EnvEntry{Key: key, Value: val}

			if isSensitiveEnvKey(key) && !in.IncludeSensitive {
				entry.Value = redactValue(val)
				entry.Redacted = true
			}

			entries = append(entries, entry)
		}
	} else {
		// Return all env vars (redact sensitive ones)
		for _, envLine := range os.Environ() {
			parts := strings.SplitN(envLine, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key, val := parts[0], parts[1]

			entry := EnvEntry{Key: key, Value: val}

			if isSensitiveEnvKey(key) && !in.IncludeSensitive {
				entry.Value = redactValue(val)
				entry.Redacted = true
			}

			entries = append(entries, entry)
		}
	}

	return json.Marshal(GetEnvOutput{
		Variables: entries,
		Count:     len(entries),
	})
}

func isSensitiveEnvKey(key string) bool {
	upper := strings.ToUpper(key)

	// Check exact matches
	if sensitiveEnvKeys[upper] {
		return true
	}

	// Check common patterns
	sensitivePatterns := []string{
		"SECRET", "PASSWORD", "PASSWD", "TOKEN",
		"API_KEY", "APIKEY", "PRIVATE_KEY", "CREDENTIAL",
		"AUTH",
	}
	for _, pat := range sensitivePatterns {
		if strings.Contains(upper, pat) {
			return true
		}
	}

	return false
}

func redactValue(val string) string {
	if len(val) <= 4 {
		return "****"
	}
	return val[:2] + strings.Repeat("*", len(val)-4) + val[len(val)-2:]
}
