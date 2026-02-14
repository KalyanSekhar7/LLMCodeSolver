Below is a **comprehensive, “Claude Code–class” toolset** you can implement in Go. You won’t need all of them on day 1, but this is the full menu you’ll eventually want for a strong coding agent runtime.

I’m listing them in **tool categories**, with **what each tool does**, and key notes (inputs/outputs + safety).

---

## 1) Workspace and Project Discovery

### `workspace_info`

* Returns: repo root path, current working dir, OS/arch, available shells, env hints (language runtimes installed).
* Why: agent adapts to Go vs Python vs Node repos.

### `list_dir`

* Inputs: `path`, `recursive` (bool), `max_depth`, `include_hidden`
* Returns: directories/files, sizes, modified time
* Why: repo exploration, quick navigation.

### `glob`

* Inputs: `patterns[]`, `path`
* Returns: matched file paths
* Why: “find all *.go files”.

### `project_detect`

* Detects: package manager(s), build system, test commands, lint commands (heuristic).
* Returns: recommended commands (e.g., `go test ./...`, `pytest`, `npm test`).
* Why: makes the agent self-starting.

---

## 2) File Read/Write/Edit Tools

### `read_file`

* Inputs: `path`, `start_line`, `end_line`, `max_bytes`
* Returns: content + metadata (encoding, size)
* Why: core context loading.

### `read_many_files`

* Inputs: `paths[]`, optional line ranges
* Returns: map path→content
* Why: reduces tool chatter.

### `write_file`

* Inputs: `path`, `content`, `create_dirs` (bool), `mode` (e.g. 0644)
* Returns: success + checksum
* Why: create/replace file.

### `append_file`

* Inputs: `path`, `content`
* Why: add to logs/docs quickly.

### `delete_file`

* Inputs: `path`, `confirm_token` (optional)
* Why: clean up old files (guard it).

### `move_file` / `copy_file`

* Inputs: `src`, `dst`, `overwrite`
* Why: refactors, re-org.

### `edit_file_ranges`

* Inputs: `path`, edits: `[{start_line,end_line,replacement}]`
* Why: safer than full overwrite; aligns with “surgical edits”.

### `apply_unified_diff`

* Inputs: diff string
* Returns: applied files + failures
* Why: allow diff-based edits as an option.

### `file_stat`

* Inputs: `path`
* Returns: size, modified time, permissions, is_binary guess
* Why: skip huge/binary files.

### `hash_file`

* Inputs: `path`, algo
* Why: integrity tracking.

---

## 3) Search, Indexing, and Code Intelligence

### `search_text`

* Inputs: `query` (regex/plain), `paths_glob`, `case_sensitive`, `max_results`
* Returns: matches with file/line snippets
* Why: “find where function is used”.

### `search_symbol`

* Inputs: `symbol`, `language`, `scope`
* Returns: definition candidates + references (best-effort)
* Why: navigation like IDE-lite.

### `ripgrep`

* A specialized wrapper around `rg` if available; fallback to Go search.
* Why: speed.

### `ctags_generate` / `ctags_query` (optional)

* Why: faster symbol lookup.

### `ast_parse` (language-specific)

* Inputs: `path`
* Returns: outline (functions/types/imports)
* Why: lets model understand structure without full file dumps.

### `code_outline`

* Inputs: `paths[]`
* Returns: summarized outlines
* Why: context compression.

---

## 4) Shell / Command Execution

### `run_command`

* Inputs: `cmd`, `args[]`, `cwd`, `env_overrides`, `timeout_ms`
* Returns: exit code, stdout, stderr
* Safety: allowlist/denylist, timeouts, output caps.
* Why: build/test/lint, git, package managers.

### `run_script`

* Inputs: `script`, `shell` (bash/sh/pwsh), `cwd`
* Why: multi-line operations (guard carefully).

### `which`

* Inputs: binary name
* Returns: path or not found
* Why: detect tools installed.

### `set_env` / `get_env` (optional)

* Safety: don’t leak secrets back to model unless explicitly permitted.

---

## 5) Git Tools (Very Important)

### `git_status`

* Returns: porcelain status, branch, upstream info

### `git_diff`

* Inputs: `staged` bool, `pathspec`, `unified` lines
* Returns: diff text

### `git_diff_cached`

* Staged diff

### `git_log`

* Inputs: `n`, `path`, `since`, `until`
* Returns: commits + messages

### `git_show`

* Inputs: commit sha, path
* Returns: file/commit details

### `git_blame`

* Inputs: path, range
* Why: understand why code exists.

### `git_checkout` / `git_switch_branch`

* Safety: guard; maybe only allow new branch creation.

### `git_create_branch`

* Inputs: name, from_ref

### `git_add`

* Inputs: paths

### `git_commit`

* Inputs: message
* Safety: maybe disabled by default.

### `git_reset`

* Safety: extremely guarded.

### `git_restore`

* Undo changes for a path.

### `git_apply`

* Applies patch (similar to `apply_unified_diff` but via git apply)

### `git_stash_push` / `git_stash_pop` (optional)

### `git_remote_info`

* Remotes, urls (careful with credentials)

---

## 6) Build, Test, Lint, Format (Language-Aware)

You can implement these as wrappers that call `run_command`, but with standard conventions and auto-detection.

### `detect_build_system`

* Returns recommended commands and config files detected.

### `run_tests`

* Inputs: `target` (unit/integration), `paths`, `timeout`
* Output: summarized test results + raw logs
* Examples:

  * Go: `go test ./...`
  * Python: `pytest -q`
  * Node: `npm test`

### `run_lint`

* Go: `golangci-lint`
* Python: `ruff`, `flake8`
* JS: `eslint`

### `run_format`

* Go: `gofmt`
* Python: `ruff format` / `black`
* JS: `prettier`

### `run_typecheck`

* TS: `tsc -p .`
* Python: `mypy`
* Go: `go vet` / `staticcheck`

### `run_build`

* Go: `go build ./...`
* etc.

---

## 7) Dependency / Package Tools

Again wrappers around command execution, but structured.

### `deps_install`

* Detects and runs appropriate install command:

  * `uv sync`, `pip install -r`
  * `npm ci`
  * `go mod download`

### `deps_list`

* Go: `go list -m all`
* Python: `pip freeze`
* Node: `npm ls --depth=0`

### `deps_add`

* Add dependency:

  * Go: `go get`
  * Node: `npm i`
  * Python: `uv add` / `pip install`

### `deps_update`

* Update dependencies.

---

## 8) Documentation and Repo Understanding

### `readme_summary`

* Reads README/CONTRIBUTING, summarizes key commands & architecture.

### `scan_docs`

* Inputs: doc paths/globs
* Output: structured summary.

### `extract_todos`

* Finds TODO/FIXME markers.

### `changelog_lookup`

* Reads CHANGELOG, release notes.

---

## 9) Config and Secrets Hygiene

### `detect_secrets`

* Scan for obvious secrets patterns (best-effort).
* Safety: important to avoid leaking.

### `redact_output`

* Tool to sanitize logs before sending to model (you run this automatically).

### `env_allowlist`

* Returns only safe env vars to model.

---

## 10) Networking Tools (Usually Disabled)

In many “Claude Code style” systems, outbound network is restricted.

If you enable it:

### `http_get` / `http_post`

* Safety: strict domain allowlist.
* Why: fetch docs, APIs.

But: **I’d keep network tools OFF by default** and rely on local repo + tests.

---

## 11) Container/Runtime Tools (Optional but Powerful)

### `docker_build`

### `docker_run`

### `docker_compose_up`

### `container_logs`

Useful if your agent is managing containers from inside a container (can get messy).

---

## 12) Patches, Plans, and Governance Tools (Quality + Safety)

These aren’t “system tools” but they massively improve usability.

### `propose_plan`

* Model outputs structured plan (steps + files to touch).
* You show it to user.

### `require_approval`

* A gating tool: “apply changes only after approval”
* (Even if you want automation, having this option is huge.)

### `change_summary`

* Summarize what changed since last step (can be computed from git diff).

### `risk_check`

* Blocks destructive actions (delete large dirs, reset, etc.).

---

## 13) Observability and Telemetry (Engineering)

### `log_event`

* Records steps: tool calls, durations, success/fail.

### `save_session`

* Persists conversation + state.

### `load_session`

### `metrics_snapshot`

* Useful later for performance.

---

# Recommended “Full” Tool List (Single Flat List)

If you want a straight comprehensive list without categories:

* workspace_info
* project_detect
* list_dir
* glob
* read_file
* read_many_files
* file_stat
* write_file
* append_file
* edit_file_ranges
* move_file
* copy_file
* delete_file
* hash_file
* apply_unified_diff
* search_text
* search_symbol
* code_outline
* ast_parse (per language)
* which
* run_command
* run_script
* git_status
* git_diff
* git_diff_cached
* git_log
* git_show
* git_blame
* git_create_branch
* git_switch_branch / git_checkout
* git_add
* git_commit
* git_restore
* git_reset (guarded)
* git_apply
* git_stash_push / git_stash_pop
* detect_build_system
* run_tests
* run_lint
* run_format
* run_typecheck
* run_build
* deps_install
* deps_list
* deps_add
* deps_update
* readme_summary
* scan_docs
* extract_todos
* detect_secrets
* redact_output
* env_allowlist
* (optional) http_get / http_post (domain allowlist)
* (optional) docker_build / docker_run / compose_up / logs
* propose_plan
* require_approval
* change_summary
* risk_check
* log_event
* save_session
* load_session
* metrics_snapshot

---

# Practical Advice (So This Doesn’t Become a Huge Project)

Even though you asked for “all tools”, implement them in **phases**:

**Phase 1 (core agent):**

* list_dir, read_file, write_file/edit_file_ranges, search_text, run_command, git_status, git_diff, run_tests

**Phase 2 (quality + navigation):**

* read_many_files, code_outline, git_blame/log/show, run_format/lint, deps_install/list

**Phase 3 (production):**

* approvals, risk_check, secrets redaction, sessions, metrics

This gives you “Claude Code vibes” quickly without drowning in scope.

---

If you want, I can next give you a **Go tool interface design** (a `Tool` struct + JSON schema + dispatcher) so you can implement these cleanly and plug into OpenAI/Anthropic tool calling with minimal glue.
