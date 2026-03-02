# Tools Implementation Status

This document tracks the implementation status of all tools specified in `tools.md`.

**Legend:**
- ✅ **Implemented** - Fully implemented and available
- ⏳ **Partial** - Partially implemented or needs enhancement
- ❌ **Not Implemented** - Not yet implemented

---

## 1) Workspace and Project Discovery

| Tool | Status | Notes |
|------|--------|-------|
| `workspace_info` | ✅ | Implemented in `workspace.go` |
| `list_dir` | ✅ | Implemented in `workspace.go` |
| `glob` | ✅ | Implemented in `workspace.go` — multi-pattern, recursive `**` support |
| `project_detect` | ✅ | Implemented in `workspace.go` |

**Category Progress:** 4/4 (100%)

---

## 2) File Read/Write/Edit Tools

| Tool | Status | Notes |
|------|--------|-------|
| `read_file` | ✅ | Implemented in `file.go` |
| `read_many_files` | ✅ | Implemented in `file.go` |
| `write_file` | ✅ | Implemented in `file.go` |
| `append_file` | ✅ | Implemented in `file.go` |
| `delete_file` | ✅ | Implemented in `file.go` with safety guard |
| `move_file` | ⏳ | Struct defined in `file.go`, Execute method not implemented |
| `copy_file` | ✅ | Implemented in `file.go` |
| `edit_file_ranges` | ✅ | Implemented in `file.go` |
| `apply_unified_diff` | ✅ | Implemented in `file.go` using `patch` command |
| `file_stat` | ✅ | Implemented in `file.go` |
| `hash_file` | ✅ | Implemented in `file.go` (supports SHA256 and MD5) |

**Category Progress:** 10/11 (91%)

---

## 3) Search, Indexing, and Code Intelligence

| Tool | Status | Notes |
|------|--------|-------|
| `search_text` | ✅ | Implemented in `search.go` — regex/plain search with glob filter |
| `search_symbol` | ✅ | Implemented in `search.go` — multi-language definition + reference lookup |
| `ripgrep` | ✅ | Implemented in `search.go` — uses `rg` if available, Go fallback |
| `ctags_generate` | ✅ | Implemented in `search.go` — generates ctags index |
| `ctags_query` | ✅ | Implemented in `search.go` — queries ctags index for symbols |
| `ast_parse` | ✅ | Implemented in `search.go` — Go AST parser + generic regex fallback |
| `code_outline` | ✅ | Implemented in `search.go` — multi-file structural outlines |

**Category Progress:** 7/7 (100%)

---

## 4) Shell / Command Execution

| Tool | Status | Notes |
|------|--------|-------|
| `run_command` | ✅ | Implemented in `shell.go` — denylist, timeout, output caps, env overrides |
| `run_script` | ✅ | Implemented in `shell.go` — multi-line scripts, shell selection (bash/zsh/sh/pwsh/fish) |
| `which` | ✅ | Implemented in `shell.go` — returns path + best-effort version detection |
| `set_env` | ✅ | Implemented in `shell.go` — warns on sensitive keys |
| `get_env` | ✅ | Implemented in `shell.go` — auto-redacts secrets, pattern-based detection |

**Category Progress:** 5/5 (100%)

---

## 5) Git Tools (Very Important)

| Tool | Status | Notes |
|------|--------|-------|
| `git_status` | ✅ | Implemented in `git.go` — porcelain v2 parsing, branch, upstream, ahead/behind |
| `git_diff` | ✅ | Implemented in `git.go` — staged/unstaged, pathspec, unified context lines |
| `git_diff_cached` | ✅ | Implemented in `git.go` — convenience wrapper over git_diff |
| `git_log` | ✅ | Implemented in `git.go` — filters: n, path, since, until, author, grep |
| `git_show` | ✅ | Implemented in `git.go` — commit details or file content at ref |
| `git_blame` | ✅ | Implemented in `git.go` — porcelain blame with line range support |
| `git_checkout` | ✅ | Implemented in `git.go` — alias for git_switch_branch |
| `git_switch_branch` | ✅ | Implemented in `git.go` — switch or create+switch |
| `git_create_branch` | ✅ | Implemented in `git.go` — from HEAD or specified ref |
| `git_add` | ✅ | Implemented in `git.go` — specific paths or --all |
| `git_commit` | ✅ | Implemented in `git.go` — returns SHA, supports allow_empty |
| `git_reset` | ✅ | Implemented in `git.go` — soft/mixed/hard, hard requires confirm_hard |
| `git_restore` | ✅ | Implemented in `git.go` — discard changes, staged/source options |
| `git_apply` | ✅ | Implemented in `git.go` — apply patch with dry-run check option |
| `git_stash_push` | ✅ | Implemented in `git.go` — message + specific file stashing |
| `git_stash_pop` | ✅ | Implemented in `git.go` — pop by index |
| `git_remote_info` | ✅ | Implemented in `git.go` — lists remotes with credential redaction |

**Category Progress:** 17/17 (100%)

---

## 6) Build, Test, Lint, Format (Language-Aware)

| Tool | Status | Notes |
|------|--------|-------|
| `detect_build_system` | ⏳ | Partially implemented in `project_detect` tool |
| `run_tests` | ❌ | Not implemented |
| `run_lint` | ❌ | Not implemented |
| `run_format` | ❌ | Not implemented |
| `run_typecheck` | ❌ | Not implemented |
| `run_build` | ❌ | Not implemented |

**Category Progress:** 0/6 (0%)

---

## 7) Dependency / Package Tools

| Tool | Status | Notes |
|------|--------|-------|
| `deps_install` | ❌ | Not implemented |
| `deps_list` | ❌ | Not implemented |
| `deps_add` | ❌ | Not implemented |
| `deps_update` | ❌ | Not implemented |

**Category Progress:** 0/4 (0%)

---

## 8) Documentation and Repo Understanding

| Tool | Status | Notes |
|------|--------|-------|
| `readme_summary` | ❌ | Not implemented |
| `scan_docs` | ❌ | Not implemented |
| `extract_todos` | ❌ | Not implemented |
| `changelog_lookup` | ❌ | Not implemented |

**Category Progress:** 0/4 (0%)

---

## 9) Config and Secrets Hygiene

| Tool | Status | Notes |
|------|--------|-------|
| `detect_secrets` | ❌ | Not implemented |
| `redact_output` | ❌ | Not implemented |
| `env_allowlist` | ❌ | Not implemented |

**Category Progress:** 0/3 (0%)

---

## 10) Networking Tools (Usually Disabled)

| Tool | Status | Notes |
|------|--------|-------|
| `http_get` | ❌ | Not implemented (recommended to keep OFF) |
| `http_post` | ❌ | Not implemented (recommended to keep OFF) |

**Category Progress:** 0/2 (0%)

---

## 11) Container/Runtime Tools (Optional but Powerful)

| Tool | Status | Notes |
|------|--------|-------|
| `docker_build` | ❌ | Not implemented |
| `docker_run` | ❌ | Not implemented |
| `docker_compose_up` | ❌ | Not implemented |
| `container_logs` | ❌ | Not implemented |

**Category Progress:** 0/4 (0%)

---

## 12) Patches, Plans, and Governance Tools (Quality + Safety)

| Tool | Status | Notes |
|------|--------|-------|
| `propose_plan` | ❌ | Not implemented |
| `require_approval` | ❌ | Not implemented |
| `change_summary` | ❌ | Not implemented |
| `risk_check` | ❌ | Not implemented |

**Category Progress:** 0/4 (0%)

---

## 13) Observability and Telemetry (Engineering)

| Tool | Status | Notes |
|------|--------|-------|
| `log_event` | ❌ | Not implemented |
| `save_session` | ❌ | Not implemented |
| `load_session` | ❌ | Not implemented |
| `metrics_snapshot` | ❌ | Not implemented |

**Category Progress:** 0/4 (0%)

---

## Overall Summary

**Total Tools:** 77
- **Implemented:** 43 (56%)
- **Partial:** 2 (3%)
- **Not Implemented:** 32 (42%)

---

## Implementation Phases (from tools.md)

### Phase 1 (Core Agent) - Priority
- ✅ list_dir
- ✅ read_file
- ✅ write_file
- ✅ edit_file_ranges
- ✅ search_text
- ✅ run_command
- ✅ git_status
- ✅ git_diff
- ❌ run_tests

**Phase 1 Progress:** 8/9 (89%)

### Phase 2 (Quality + Navigation)
- ✅ read_many_files
- ✅ code_outline
- ✅ git_blame
- ✅ git_log
- ✅ git_show
- ❌ run_format
- ❌ run_lint
- ❌ deps_install
- ❌ deps_list

**Phase 2 Progress:** 5/9 (56%)

### Phase 3 (Production)
- ❌ require_approval
- ❌ risk_check
- ❌ redact_output (secrets)
- ❌ save_session
- ❌ load_session
- ❌ metrics_snapshot

**Phase 3 Progress:** 0/6 (0%)

---

## Recommendations

### Immediate Priorities (Phase 1 Completion)
1. Implement `run_tests` — last Phase 1 item

### Quick Wins
1. Complete `move_file` implementation (struct exists)
2. ~~Implement `glob`~~ (done)

### High-Value Next
1. **Build/Test/Lint tools** — wrappers over run_command
2. **Dependency tools** — package management

---

*Last Updated: 2026-02-14*
