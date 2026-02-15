package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// helpers shared across git tools
// ---------------------------------------------------------------------------

const gitTimeout = 30 * time.Second

// runGit executes a git command and returns stdout, stderr, exit code.
func runGit(ctx context.Context, dir string, args ...string) (string, string, int, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if cmdCtx.Err() == context.DeadlineExceeded {
			return stdout.String(), stderr.String(), -1, fmt.Errorf("git command timed out")
		} else {
			return "", "", -1, fmt.Errorf("failed to run git: %w", err)
		}
	}

	return stdout.String(), stderr.String(), exitCode, nil
}

// resolveDir returns the given dir or cwd.
func resolveDir(dir string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	return os.Getwd()
}

// ---------------------------------------------------------------------------
// git_status
// ---------------------------------------------------------------------------

type GitStatusInput struct {
	Dir string `json:"dir"`
}

type GitFileStatus struct {
	Path     string `json:"path"`
	Status   string `json:"status"`   // "M", "A", "D", "??" etc.
	Staged   string `json:"staged"`   // staging area indicator
	Unstaged string `json:"unstaged"` // working tree indicator
}

type GitStatusOutput struct {
	Branch       string          `json:"branch"`
	Upstream     string          `json:"upstream,omitempty"`
	Ahead        int             `json:"ahead"`
	Behind       int             `json:"behind"`
	Files        []GitFileStatus `json:"files"`
	IsClean      bool            `json:"is_clean"`
	IsRepo       bool            `json:"is_repo"`
	RawPorcelain string          `json:"raw_porcelain"`
}

type GitStatusTool struct{}

func (t *GitStatusTool) Name() string        { return "git_status" }
func (t *GitStatusTool) Description() string {
	return "Returns porcelain status, branch, and upstream info for a git repo."
}

func (t *GitStatusTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitStatusInput
	json.Unmarshal(input, &in)

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	out := GitStatusOutput{IsRepo: true}

	// Branch + tracking info
	stdout, _, _, err := runGit(ctx, dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		out.IsRepo = false
		return json.Marshal(out)
	}

	out.RawPorcelain = stdout

	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "# branch.head ") {
			out.Branch = strings.TrimPrefix(line, "# branch.head ")
		} else if strings.HasPrefix(line, "# branch.upstream ") {
			out.Upstream = strings.TrimPrefix(line, "# branch.upstream ")
		} else if strings.HasPrefix(line, "# branch.ab ") {
			ab := strings.TrimPrefix(line, "# branch.ab ")
			parts := strings.Fields(ab)
			for _, p := range parts {
				if strings.HasPrefix(p, "+") {
					out.Ahead, _ = strconv.Atoi(p[1:])
				} else if strings.HasPrefix(p, "-") {
					out.Behind, _ = strconv.Atoi(p[1:])
				}
			}
		} else if !strings.HasPrefix(line, "#") {
			// Parse file entries
			fs := parseStatusLine(line)
			if fs.Path != "" {
				out.Files = append(out.Files, fs)
			}
		}
	}

	out.IsClean = len(out.Files) == 0
	return json.Marshal(out)
}

func parseStatusLine(line string) GitFileStatus {
	// porcelain v2 format:
	//   1 XY ... path
	//   2 XY ... path\torigPath   (renames)
	//   ? path                     (untracked)
	//   ! path                     (ignored)

	if strings.HasPrefix(line, "? ") {
		return GitFileStatus{
			Path:   strings.TrimPrefix(line, "? "),
			Status: "??",
			Staged: " ", Unstaged: "?",
		}
	}
	if strings.HasPrefix(line, "! ") {
		return GitFileStatus{
			Path:   strings.TrimPrefix(line, "! "),
			Status: "!!",
			Staged: " ", Unstaged: "!",
		}
	}

	// "1 XY ..." or "2 XY ..."
	if (strings.HasPrefix(line, "1 ") || strings.HasPrefix(line, "2 ")) && len(line) > 4 {
		xy := line[2:4]
		// Find the path — it's the last space-separated field
		fields := strings.Fields(line)
		path := ""
		if len(fields) > 0 {
			path = fields[len(fields)-1]
		}
		return GitFileStatus{
			Path:     path,
			Status:   xy,
			Staged:   string(xy[0]),
			Unstaged: string(xy[1]),
		}
	}

	return GitFileStatus{}
}

// ---------------------------------------------------------------------------
// git_diff
// ---------------------------------------------------------------------------

type GitDiffInput struct {
	Dir      string `json:"dir"`
	Staged   bool   `json:"staged"`
	Pathspec string `json:"pathspec"`
	Unified  int    `json:"unified"` // context lines (default 3)
}

type GitDiffOutput struct {
	Diff     string `json:"diff"`
	NumFiles int    `json:"num_files"`
	Empty    bool   `json:"empty"`
}

type GitDiffTool struct{}

func (t *GitDiffTool) Name() string        { return "git_diff" }
func (t *GitDiffTool) Description() string {
	return "Returns diff text for staged or unstaged changes."
}

func (t *GitDiffTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitDiffInput
	json.Unmarshal(input, &in)

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	args := []string{"diff"}

	if in.Staged {
		args = append(args, "--cached")
	}

	if in.Unified > 0 {
		args = append(args, fmt.Sprintf("-U%d", in.Unified))
	}

	args = append(args, "--stat")

	if in.Pathspec != "" {
		args = append(args, "--", in.Pathspec)
	}

	// First get stat to count files
	statOut, _, _, _ := runGit(ctx, dir, args...)

	// Now get the actual diff
	diffArgs := []string{"diff"}
	if in.Staged {
		diffArgs = append(diffArgs, "--cached")
	}
	if in.Unified > 0 {
		diffArgs = append(diffArgs, fmt.Sprintf("-U%d", in.Unified))
	}
	if in.Pathspec != "" {
		diffArgs = append(diffArgs, "--", in.Pathspec)
	}

	diffOut, _, _, _ := runGit(ctx, dir, diffArgs...)

	numFiles := 0
	for _, line := range strings.Split(statOut, "\n") {
		if strings.Contains(line, "|") {
			numFiles++
		}
	}

	return json.Marshal(GitDiffOutput{
		Diff:     diffOut,
		NumFiles: numFiles,
		Empty:    strings.TrimSpace(diffOut) == "",
	})
}

// ---------------------------------------------------------------------------
// git_diff_cached — convenience alias for staged diff
// ---------------------------------------------------------------------------

type GitDiffCachedInput struct {
	Dir      string `json:"dir"`
	Pathspec string `json:"pathspec"`
	Unified  int    `json:"unified"`
}

type GitDiffCachedTool struct{}

func (t *GitDiffCachedTool) Name() string        { return "git_diff_cached" }
func (t *GitDiffCachedTool) Description() string {
	return "Returns staged (cached) diff text."
}

func (t *GitDiffCachedTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitDiffCachedInput
	json.Unmarshal(input, &in)

	// Delegate to git_diff with staged=true
	wrapped := GitDiffInput{
		Dir:      in.Dir,
		Staged:   true,
		Pathspec: in.Pathspec,
		Unified:  in.Unified,
	}

	payload, _ := json.Marshal(wrapped)
	tool := &GitDiffTool{}
	return tool.Execute(ctx, payload)
}

// ---------------------------------------------------------------------------
// git_log
// ---------------------------------------------------------------------------

type GitLogInput struct {
	Dir    string `json:"dir"`
	N      int    `json:"n"` // number of commits
	Path   string `json:"path"`
	Since  string `json:"since"` // e.g. "2024-01-01"
	Until  string `json:"until"`
	Author string `json:"author"`
	Grep   string `json:"grep"` // filter by message
}

type GitCommit struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
	Body    string `json:"body,omitempty"`
}

type GitLogOutput struct {
	Commits []GitCommit `json:"commits"`
	Count   int         `json:"count"`
}

type GitLogTool struct{}

func (t *GitLogTool) Name() string        { return "git_log" }
func (t *GitLogTool) Description() string {
	return "Returns commit log with optional filters (count, path, date range, author, message grep)."
}

func (t *GitLogTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitLogInput
	json.Unmarshal(input, &in)

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	if in.N <= 0 {
		in.N = 20
	}

	// Use a delimiter to parse structured output
	delim := "---COMMIT_DELIM---"
	// format: sha | author | date | subject | body
	format := strings.Join([]string{"%H", "%an", "%ai", "%s", "%b"}, "%x00") + delim

	args := []string{"log", fmt.Sprintf("-n%d", in.N), fmt.Sprintf("--format=%s", format)}

	if in.Since != "" {
		args = append(args, "--since="+in.Since)
	}
	if in.Until != "" {
		args = append(args, "--until="+in.Until)
	}
	if in.Author != "" {
		args = append(args, "--author="+in.Author)
	}
	if in.Grep != "" {
		args = append(args, "--grep="+in.Grep)
	}

	if in.Path != "" {
		args = append(args, "--", in.Path)
	}

	stdout, _, _, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	var commits []GitCommit
	entries := strings.Split(stdout, delim)

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, "\x00", 5)
		if len(parts) < 4 {
			continue
		}

		c := GitCommit{
			SHA:     parts[0],
			Author:  parts[1],
			Date:    parts[2],
			Subject: parts[3],
		}
		if len(parts) > 4 {
			c.Body = strings.TrimSpace(parts[4])
		}
		commits = append(commits, c)
	}

	return json.Marshal(GitLogOutput{
		Commits: commits,
		Count:   len(commits),
	})
}

// ---------------------------------------------------------------------------
// git_show
// ---------------------------------------------------------------------------

type GitShowInput struct {
	Dir  string `json:"dir"`
	Ref  string `json:"ref"` // commit SHA, tag, etc.
	Path string `json:"path"`
}

type GitShowOutput struct {
	Content string `json:"content"`
	Stat    string `json:"stat,omitempty"`
}

type GitShowTool struct{}

func (t *GitShowTool) Name() string        { return "git_show" }
func (t *GitShowTool) Description() string {
	return "Shows commit details or file content at a given ref."
}

func (t *GitShowTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitShowInput
	json.Unmarshal(input, &in)

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	if in.Ref == "" {
		in.Ref = "HEAD"
	}

	out := GitShowOutput{}

	if in.Path != "" {
		// Show file at ref
		target := in.Ref + ":" + in.Path
		stdout, _, exitCode, _ := runGit(ctx, dir, "show", target)
		if exitCode != 0 {
			return nil, fmt.Errorf("git show %s failed", target)
		}
		out.Content = stdout
	} else {
		// Show commit details
		stdout, _, exitCode, _ := runGit(ctx, dir, "show", "--stat", in.Ref)
		if exitCode != 0 {
			return nil, fmt.Errorf("git show %s failed", in.Ref)
		}
		out.Content = stdout

		statOut, _, _, _ := runGit(ctx, dir, "show", "--stat", "--format=", in.Ref)
		out.Stat = strings.TrimSpace(statOut)
	}

	return json.Marshal(out)
}

// ---------------------------------------------------------------------------
// git_blame
// ---------------------------------------------------------------------------

type GitBlameInput struct {
	Dir       string `json:"dir"`
	Path      string `json:"path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

type BlameLine struct {
	SHA    string `json:"sha"`
	Author string `json:"author"`
	Date   string `json:"date"`
	Line   int    `json:"line"`
	Content string `json:"content"`
}

type GitBlameOutput struct {
	Lines []BlameLine `json:"lines"`
}

type GitBlameTool struct{}

func (t *GitBlameTool) Name() string        { return "git_blame" }
func (t *GitBlameTool) Description() string {
	return "Shows line-by-line authorship for a file or line range."
}

func (t *GitBlameTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitBlameInput
	json.Unmarshal(input, &in)

	if in.Path == "" {
		return nil, errors.New("path required")
	}

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	args := []string{"blame", "--porcelain"}

	if in.StartLine > 0 && in.EndLine > 0 {
		args = append(args, fmt.Sprintf("-L%d,%d", in.StartLine, in.EndLine))
	} else if in.StartLine > 0 {
		args = append(args, fmt.Sprintf("-L%d,", in.StartLine))
	}

	args = append(args, in.Path)

	stdout, _, exitCode, _ := runGit(ctx, dir, args...)
	if exitCode != 0 {
		return nil, fmt.Errorf("git blame failed for %s", in.Path)
	}

	lines := parseBlameOutput(stdout)
	return json.Marshal(GitBlameOutput{Lines: lines})
}

func parseBlameOutput(raw string) []BlameLine {
	var result []BlameLine
	lines := strings.Split(raw, "\n")

	var current BlameLine
	lineNum := 0

	for _, line := range lines {
		if len(line) >= 40 && isHex(line[:40]) {
			// Start of a new blame block: sha origLine finalLine [numLines]
			parts := strings.Fields(line)
			current = BlameLine{SHA: parts[0]}
			if len(parts) >= 3 {
				current.Line, _ = strconv.Atoi(parts[2])
				lineNum = current.Line
			}
		} else if strings.HasPrefix(line, "author ") {
			current.Author = strings.TrimPrefix(line, "author ")
		} else if strings.HasPrefix(line, "author-time ") {
			ts := strings.TrimPrefix(line, "author-time ")
			if sec, err := strconv.ParseInt(ts, 10, 64); err == nil {
				current.Date = time.Unix(sec, 0).Format("2006-01-02 15:04:05")
			}
		} else if strings.HasPrefix(line, "\t") {
			// Content line
			current.Content = strings.TrimPrefix(line, "\t")
			current.Line = lineNum
			result = append(result, current)
			lineNum++
		}
	}

	return result
}

func isHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// git_create_branch
// ---------------------------------------------------------------------------

type GitCreateBranchInput struct {
	Dir     string `json:"dir"`
	Name    string `json:"name"`
	FromRef string `json:"from_ref"`
}

type GitCreateBranchOutput struct {
	Success bool   `json:"success"`
	Branch  string `json:"branch"`
	Message string `json:"message,omitempty"`
}

type GitCreateBranchTool struct{}

func (t *GitCreateBranchTool) Name() string        { return "git_create_branch" }
func (t *GitCreateBranchTool) Description() string {
	return "Creates a new git branch from the current HEAD or a specified ref."
}

func (t *GitCreateBranchTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitCreateBranchInput
	json.Unmarshal(input, &in)

	if in.Name == "" {
		return nil, errors.New("name required")
	}

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	args := []string{"branch", in.Name}
	if in.FromRef != "" {
		args = append(args, in.FromRef)
	}

	_, stderr, exitCode, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return json.Marshal(GitCreateBranchOutput{
			Success: false,
			Branch:  in.Name,
			Message: strings.TrimSpace(stderr),
		})
	}

	return json.Marshal(GitCreateBranchOutput{
		Success: true,
		Branch:  in.Name,
	})
}

// ---------------------------------------------------------------------------
// git_switch_branch / git_checkout
// ---------------------------------------------------------------------------

type GitSwitchBranchInput struct {
	Dir    string `json:"dir"`
	Branch string `json:"branch"`
	Create bool   `json:"create"` // create + switch if true
}

type GitSwitchBranchOutput struct {
	Success  bool   `json:"success"`
	Branch   string `json:"branch"`
	Message  string `json:"message,omitempty"`
}

type GitSwitchBranchTool struct{}
type GitCheckoutTool struct{}

func (t *GitSwitchBranchTool) Name() string { return "git_switch_branch" }
func (t *GitCheckoutTool) Name() string     { return "git_checkout" }

func (t *GitSwitchBranchTool) Description() string {
	return "Switches to an existing branch, or creates and switches with create=true."
}
func (t *GitCheckoutTool) Description() string {
	return "Checks out a branch. Alias for git_switch_branch."
}

func gitSwitchExecute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitSwitchBranchInput
	json.Unmarshal(input, &in)

	if in.Branch == "" {
		return nil, errors.New("branch required")
	}

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	var args []string
	if in.Create {
		args = []string{"checkout", "-b", in.Branch}
	} else {
		args = []string{"checkout", in.Branch}
	}

	_, stderr, exitCode, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return json.Marshal(GitSwitchBranchOutput{
			Success: false,
			Branch:  in.Branch,
			Message: strings.TrimSpace(stderr),
		})
	}

	return json.Marshal(GitSwitchBranchOutput{
		Success: true,
		Branch:  in.Branch,
	})
}

func (t *GitSwitchBranchTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	return gitSwitchExecute(ctx, input)
}

func (t *GitCheckoutTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	return gitSwitchExecute(ctx, input)
}

// ---------------------------------------------------------------------------
// git_add
// ---------------------------------------------------------------------------

type GitAddInput struct {
	Dir   string   `json:"dir"`
	Paths []string `json:"paths"`
	All   bool     `json:"all"`
}

type GitAddOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type GitAddTool struct{}

func (t *GitAddTool) Name() string        { return "git_add" }
func (t *GitAddTool) Description() string {
	return "Stages files for commit."
}

func (t *GitAddTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitAddInput
	json.Unmarshal(input, &in)

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	if !in.All && len(in.Paths) == 0 {
		return nil, errors.New("paths required (or set all=true)")
	}

	var args []string
	if in.All {
		args = []string{"add", "-A"}
	} else {
		args = append([]string{"add", "--"}, in.Paths...)
	}

	_, stderr, exitCode, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return json.Marshal(GitAddOutput{
			Success: false,
			Message: strings.TrimSpace(stderr),
		})
	}

	return json.Marshal(GitAddOutput{Success: true})
}

// ---------------------------------------------------------------------------
// git_commit
// ---------------------------------------------------------------------------

type GitCommitInput struct {
	Dir     string `json:"dir"`
	Message string `json:"message"`
	AllowEmpty bool `json:"allow_empty"`
}

type GitCommitOutput struct {
	Success bool   `json:"success"`
	SHA     string `json:"sha,omitempty"`
	Message string `json:"message,omitempty"`
}

type GitCommitTool struct{}

func (t *GitCommitTool) Name() string        { return "git_commit" }
func (t *GitCommitTool) Description() string {
	return "Creates a commit with the given message. Operates on staged changes."
}

func (t *GitCommitTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitCommitInput
	json.Unmarshal(input, &in)

	if in.Message == "" {
		return nil, errors.New("message required")
	}

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	args := []string{"commit", "-m", in.Message}
	if in.AllowEmpty {
		args = append(args, "--allow-empty")
	}

	stdout, stderr, exitCode, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return json.Marshal(GitCommitOutput{
			Success: false,
			Message: strings.TrimSpace(stderr),
		})
	}

	// Extract SHA from output
	sha := ""
	shaOut, _, _, _ := runGit(ctx, dir, "rev-parse", "HEAD")
	sha = strings.TrimSpace(shaOut)

	msg := strings.TrimSpace(stdout)
	if msg == "" {
		msg = strings.TrimSpace(stderr)
	}

	return json.Marshal(GitCommitOutput{
		Success: true,
		SHA:     sha,
		Message: msg,
	})
}

// ---------------------------------------------------------------------------
// git_reset — extremely guarded
// ---------------------------------------------------------------------------

type GitResetInput struct {
	Dir          string `json:"dir"`
	Mode         string `json:"mode"` // "soft", "mixed", "hard"
	Ref          string `json:"ref"`
	ConfirmHard  bool   `json:"confirm_hard"` // must be true for --hard
}

type GitResetOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type GitResetTool struct{}

func (t *GitResetTool) Name() string        { return "git_reset" }
func (t *GitResetTool) Description() string {
	return "Resets the current branch. Hard resets require explicit confirm_hard=true."
}

func (t *GitResetTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitResetInput
	json.Unmarshal(input, &in)

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	mode := in.Mode
	if mode == "" {
		mode = "mixed"
	}

	// Safety: hard reset requires explicit confirmation
	if mode == "hard" && !in.ConfirmHard {
		return json.Marshal(GitResetOutput{
			Success: false,
			Message: "hard reset requires confirm_hard=true — this is destructive and cannot be undone",
		})
	}

	ref := in.Ref
	if ref == "" {
		ref = "HEAD"
	}

	args := []string{"reset", "--" + mode, ref}
	_, stderr, exitCode, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return json.Marshal(GitResetOutput{
			Success: false,
			Message: strings.TrimSpace(stderr),
		})
	}

	return json.Marshal(GitResetOutput{
		Success: true,
		Message: fmt.Sprintf("reset --%s to %s", mode, ref),
	})
}

// ---------------------------------------------------------------------------
// git_restore
// ---------------------------------------------------------------------------

type GitRestoreInput struct {
	Dir    string   `json:"dir"`
	Paths  []string `json:"paths"`
	Staged bool     `json:"staged"` // restore from index (unstage)
	Source string   `json:"source"` // ref to restore from
}

type GitRestoreOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type GitRestoreTool struct{}

func (t *GitRestoreTool) Name() string        { return "git_restore" }
func (t *GitRestoreTool) Description() string {
	return "Restores (discards) changes for specified files."
}

func (t *GitRestoreTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitRestoreInput
	json.Unmarshal(input, &in)

	if len(in.Paths) == 0 {
		return nil, errors.New("paths required")
	}

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	args := []string{"restore"}

	if in.Staged {
		args = append(args, "--staged")
	}
	if in.Source != "" {
		args = append(args, "--source="+in.Source)
	}

	args = append(args, "--")
	args = append(args, in.Paths...)

	_, stderr, exitCode, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return json.Marshal(GitRestoreOutput{
			Success: false,
			Message: strings.TrimSpace(stderr),
		})
	}

	return json.Marshal(GitRestoreOutput{Success: true})
}

// ---------------------------------------------------------------------------
// git_apply
// ---------------------------------------------------------------------------

type GitApplyInput struct {
	Dir   string `json:"dir"`
	Patch string `json:"patch"`
	Check bool   `json:"check"` // dry-run
}

type GitApplyOutput struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Message string `json:"message,omitempty"`
}

type GitApplyTool struct{}

func (t *GitApplyTool) Name() string        { return "git_apply" }
func (t *GitApplyTool) Description() string {
	return "Applies a patch via git apply."
}

func (t *GitApplyTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitApplyInput
	json.Unmarshal(input, &in)

	if in.Patch == "" {
		return nil, errors.New("patch required")
	}

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	args := []string{"apply"}
	if in.Check {
		args = append(args, "--check")
	}
	args = append(args, "-")

	cmdCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(in.Patch)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()

	if err != nil {
		return json.Marshal(GitApplyOutput{
			Success: false,
			Output:  stdout.String(),
			Message: strings.TrimSpace(stderr.String()),
		})
	}

	return json.Marshal(GitApplyOutput{
		Success: true,
		Output:  stdout.String(),
	})
}

// ---------------------------------------------------------------------------
// git_stash_push
// ---------------------------------------------------------------------------

type GitStashPushInput struct {
	Dir     string   `json:"dir"`
	Message string   `json:"message"`
	Paths   []string `json:"paths"` // specific files to stash
}

type GitStashOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

type GitStashPushTool struct{}

func (t *GitStashPushTool) Name() string        { return "git_stash_push" }
func (t *GitStashPushTool) Description() string {
	return "Stashes current working changes."
}

func (t *GitStashPushTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitStashPushInput
	json.Unmarshal(input, &in)

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	args := []string{"stash", "push"}

	if in.Message != "" {
		args = append(args, "-m", in.Message)
	}

	if len(in.Paths) > 0 {
		args = append(args, "--")
		args = append(args, in.Paths...)
	}

	stdout, stderr, exitCode, err := runGit(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return json.Marshal(GitStashOutput{
			Success: false,
			Message: strings.TrimSpace(stderr),
		})
	}

	msg := strings.TrimSpace(stdout)
	if msg == "" {
		msg = strings.TrimSpace(stderr)
	}

	return json.Marshal(GitStashOutput{
		Success: true,
		Message: msg,
	})
}

// ---------------------------------------------------------------------------
// git_stash_pop
// ---------------------------------------------------------------------------

type GitStashPopInput struct {
	Dir   string `json:"dir"`
	Index int    `json:"index"` // stash@{index}, default 0
}

type GitStashPopTool struct{}

func (t *GitStashPopTool) Name() string        { return "git_stash_pop" }
func (t *GitStashPopTool) Description() string {
	return "Pops the most recent stash (or stash at given index)."
}

func (t *GitStashPopTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitStashPopInput
	json.Unmarshal(input, &in)

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	ref := fmt.Sprintf("stash@{%d}", in.Index)
	stdout, stderr, exitCode, err := runGit(ctx, dir, "stash", "pop", ref)
	if err != nil {
		return nil, err
	}

	if exitCode != 0 {
		return json.Marshal(GitStashOutput{
			Success: false,
			Message: strings.TrimSpace(stderr),
		})
	}

	msg := strings.TrimSpace(stdout)
	if msg == "" {
		msg = strings.TrimSpace(stderr)
	}

	return json.Marshal(GitStashOutput{
		Success: true,
		Message: msg,
	})
}

// ---------------------------------------------------------------------------
// git_remote_info
// ---------------------------------------------------------------------------

type GitRemoteInfoInput struct {
	Dir string `json:"dir"`
}

type GitRemote struct {
	Name     string `json:"name"`
	FetchURL string `json:"fetch_url"`
	PushURL  string `json:"push_url"`
}

type GitRemoteInfoOutput struct {
	Remotes []GitRemote `json:"remotes"`
}

type GitRemoteInfoTool struct{}

func (t *GitRemoteInfoTool) Name() string        { return "git_remote_info" }
func (t *GitRemoteInfoTool) Description() string {
	return "Lists git remotes with fetch/push URLs (credentials are redacted)."
}

func (t *GitRemoteInfoTool) Execute(ctx context.Context, input []byte) ([]byte, error) {
	var in GitRemoteInfoInput
	json.Unmarshal(input, &in)

	dir, err := resolveDir(in.Dir)
	if err != nil {
		return nil, err
	}

	stdout, _, _, err := runGit(ctx, dir, "remote", "-v")
	if err != nil {
		return nil, err
	}

	remoteMap := make(map[string]*GitRemote)

	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// format: origin\thttps://github.com/user/repo.git (fetch)
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		name := parts[0]
		url := redactGitURL(parts[1])
		kind := parts[2] // "(fetch)" or "(push)"

		r, exists := remoteMap[name]
		if !exists {
			r = &GitRemote{Name: name}
			remoteMap[name] = r
		}

		if kind == "(fetch)" {
			r.FetchURL = url
		} else if kind == "(push)" {
			r.PushURL = url
		}
	}

	var remotes []GitRemote
	for _, r := range remoteMap {
		remotes = append(remotes, *r)
	}

	return json.Marshal(GitRemoteInfoOutput{Remotes: remotes})
}

// redactGitURL removes embedded credentials from git URLs.
// e.g. https://user:token@github.com/... -> https://***@github.com/...
func redactGitURL(rawURL string) string {
	if idx := strings.Index(rawURL, "@"); idx != -1 {
		prefix := rawURL[:idx]
		// Check if there's a :// before the @
		if schemeEnd := strings.Index(prefix, "://"); schemeEnd != -1 {
			return prefix[:schemeEnd+3] + "***" + rawURL[idx:]
		}
	}
	return rawURL
}
