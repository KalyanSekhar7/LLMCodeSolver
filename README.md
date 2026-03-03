# LLM Code Solver

A terminal-based interactive AI coding agent that runs entirely inside a Docker container.
Point it at any GitHub repo, and it builds an isolated environment with the right language runtime,
dependencies, and 43 developer tools — then drops you into an interactive session where an LLM
can read, search, edit,test and run code on your behalf.

---

## How It Works

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           MACHINE (Host)                                   │
│                                                                            │
│   1. Configure repo ──► 2. Generate Dockerfile ──► 3. Build image          │
│      (YAML)                (Python scripts)           (docker build)       │
│                                                                            │
│   orchestration_config.yaml                                                │
│   ┌──────────────────────┐      ┌────────────────┐    ┌───────────────┐    │
│   │ type: "python"       │──►   │ create_docker  │──► │  Dockerfile   │    │
│   │ repo: scikit-learn   │      │ file.py        │    │  (generated)  │    │
│   │ url: github.com/...  │      └────────────────┘    └───────┬───────┘    │
│   └──────────────────────┘                                    │            │
│                                                               ▼            │
│                                                     docker build + run     │
│                                                               │            │
└───────────────────────────────────────────────────────────────┼────────────┘
                                                                │
                    ┌───────────────────────────────────────────┼────────────┐
                    │              DOCKER CONTAINER              ▼            │
                    │                                                        │
                    │   /usr/local/bin/llm-agent  (21 MB static Go binary)   │
                    │   ┌──────────────────────────────────────────────┐     │
                    │   │  Terminal UI          >>>  user prompt       │     │
                    │   │  ┌────────────────────────────────────────┐  │     │
                    │   │  │  Agent Loop                           │  │     │
                    │   │  │  ┌──────────┐  ┌───────────────────┐  │  │     │
                    │   │  │  │ Anthropic │  │  43 Tools         │  │  │     │
                    │   │  │  │ Claude    │◄►│  file, git, shell │  │  │     │
                    │   │  │  │ API       │  │  search, workspace│  │  │     │
                    │   │  │  └──────────┘  └───────────────────┘  │  │     │
                    │   │  └────────────────────────────────────────┘  │     │
                    │   └──────────────────────────────────────────────┘     │
                    │                                                        │
                    │   /working_directory/testbed/repo  (cloned + built)    │
                    └────────────────────────────────────────────────────────┘
```

### Agent Loop Flow

```
    User types a prompt
            │
            ▼
    ┌───────────────┐
    │  Send to LLM  │◄─────────────────────────┐
    │  (streaming)  │                           │
    └───────┬───────┘                           │
            │                                   │
            ▼                                   │
    ┌───────────────┐     Yes    ┌────────────┐ │
    │  Tool calls?  │──────────► │ Execute    │ │
    │               │            │ each tool  │ │
    └───────┬───────┘            └─────┬──────┘ │
            │ No                       │        │
            ▼                          │        │
    ┌───────────────┐           Feed results    │
    │  Display      │           back to LLM ────┘
    │  response     │
    └───────────────┘
```

### What You See

```
┌────────────────────────────────────────────────────────────────┐
│ LLM Code Solver                                               │
│                                                               │
│ Working dir:  /workspace                                      │
│ Model:        claude-sonnet-4-20250514                        │
│ Max tokens:   16384 per response                              │
│ Tools:        43 available                                    │
│                                                               │
│ Session: no tokens used yet                                   │
│                                                               │
│ Ctrl+C interrupt  Ctrl+D exit  /help this  /clear reset       │
└────────────────────────────────────────────────────────────────┘

>>> Fix the bug in src/math_utils.py

  ● Thinking...
    Let me read the file first to understand the current code...

  ■ Tool: read_file
    path: src/math_utils.py
    ┌──────────────────────────────────────────────────────────────────────────┐
    │ def add(a, b):                                                         │
    │     return a + b                                                       │
    └──────────────────────────────────────────────────────────────────────────┘

  ■ Tool: edit_file_ranges
    path: src/math_utils.py

  ■ Tool: run_command
    cmd: python src/main.py
    ┌──────────────────────────────────────────────────────────────────────────┐
    │ 5                                                                      │
    └──────────────────────────────────────────────────────────────────────────┘

  ✔ Done (3 tool calls, 8.2s, 1523→847 tokens)
  Session: 1523 in + 847 out = 2370 tokens │ 3 tool calls │ 1 prompts

>>>
```

---

## Prerequisites

| Tool | Version | Check |
|------|---------|-------|
| **Go** | 1.24+ | `go version` |
| **Python** | 3.11+ | `python3 --version` |
| **Docker** | 20+ | `docker --version` |
| **Anthropic API key** | — | [console.anthropic.com](https://console.anthropic.com/) |

---

## Setup

### 1. Clone the repo

```bash
git clone https://github.com/your-org/Orchestration.git
cd Orchestration
```

### 2. Set up Python (host-side Dockerfile generator)

```bash
cd Python
python3 -m venv .venv
source .venv/bin/activate    # macOS/Linux
pip install -e .
```

### 3. Set up your API key

Create `Python/.env`:

```bash
echo 'ANTHROPIC_API_KEY=sk-ant-api03-your-key-here' > Python/.env
```

### 4. Build the Go agent binary

```bash
cd Go-tools

# Build for your local machine (for development)
go build -o agent ./cmd/agent/

# Cross-compile for Docker (Linux container)
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../build/llm-agent ./cmd/agent/
```

---

## Running

### Option A: Quick Test (no repo, just the agent)

The fastest way to test. Uses `Dockerfile.test` which creates a tiny workspace
with sample files.

```bash
# From the project root
cd Orchestration

# 1. Cross-compile for Linux (if not already done)
cd Go-tools
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../build/llm-agent ./cmd/agent/
cd ..

# 2. Build the test image
docker build --platform linux/amd64 -f Dockerfile.test -t solver:test .

# 3. Run it (API key from .env file)
docker run -it --platform linux/amd64 --env-file Python/.env solver:test
```

You'll get an interactive prompt. Try:

```
>>> list all files in this directory
>>> read hello.py and explain what it does
>>> create a python script that calculates fibonacci numbers
>>> run python hello.py
>>> show git status
```

### Option B: Real Repo Environment

Generate a Dockerfile for any GitHub repo, with the correct language runtime
and dependencies pre-installed.

**Step 1:** Edit `Python/orchestration_config.yaml`:

```yaml
type: "python"                                    # python | go | rust | javascript
repository:
  name: scikit-learn
  url: https://github.com/scikit-learn/scikit-learn
```

**Step 2:** Generate the Dockerfile:

```bash
cd Python
source .venv/bin/activate
cd ..
PYTHONPATH=. python Python/main.py
# → Creates "Dockerfile" in current directory
```

**Step 3:** Build and run:

```bash
# Build (this clones the repo and installs dependencies — may take a while)
docker build -t solver:scikit-learn .

# Run with the agent
docker run -it --env-file Python/.env solver:scikit-learn llm-agent
```

### Option C: Run Locally (no Docker)

For development or quick testing without Docker:

```bash
cd Go-tools
ANTHROPIC_API_KEY="sk-ant-..." go run ./cmd/agent/
```

The agent will use your current directory as the workspace.

---

## Supported Languages

The Dockerfile generator auto-detects:

| Language | Detected from | Base image |
|----------|--------------|------------|
| **Python** | `pyproject.toml`, `setup.py`, `requirements.txt`, `Pipfile`, `environment.yml` | `python:3.x` |
| **Go** | `go.mod`, `Gopkg.toml`, `glide.yaml` | `golang:1.x` |
| **Rust** | `Cargo.toml` | `rust:1.x` |
| **JavaScript** | `package.json`, `yarn.lock`, `pnpm-lock.yaml` | `node:x` |

---

## Tools (43 total)

The agent has direct access to these tools inside the container:

| Category | Count | Tools |
|----------|-------|-------|
| **File** | 11 | `read_file` `read_many_files` `write_file` `append_file` `delete_file` `move_file` `copy_file` `edit_file_ranges` `apply_unified_diff` `file_stat` `hash_file` |
| **Git** | 17 | `git_status` `git_diff` `git_diff_cached` `git_log` `git_show` `git_blame` `git_create_branch` `git_switch_branch` `git_checkout` `git_add` `git_commit` `git_reset` `git_restore` `git_apply` `git_stash_push` `git_stash_pop` `git_remote_info` |
| **Shell** | 5 | `run_command` `run_script` `which` `set_env` `get_env` |
| **Search** | 7 | `search_text` `search_symbol` `ripgrep` `ctags_generate` `ctags_query` `ast_parse` `code_outline` |
| **Workspace** | 4 | `workspace_info` `list_dir` `glob` `project_detect` |

---

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `ANTHROPIC_API_KEY` | Yes* | — | Anthropic API key |
| `OPENAI_API_KEY` | Yes* | — | OpenAI API key (if using OpenAI) |
| `LLM_PROVIDER` | No | `anthropic` | `anthropic` or `openai` |
| `LLM_MODEL` | No | `claude-sonnet-4-20250514` | Model to use |
| `WORK_DIR` | No | current directory | Working directory inside container |
| `MAX_TURNS` | No | `50` | Max LLM round-trips per prompt |

*One of `ANTHROPIC_API_KEY` or `OPENAI_API_KEY` is required.

---

## Keyboard Shortcuts & Commands

| Shortcut | Action |
|----------|--------|
| `Ctrl+C` | Interrupt current agent run (returns to prompt) |
| `Ctrl+D` | Exit the session |
| `/help` | Show the reference banner |
| `/clear` | Reset conversation history |
| `/diff` | Show uncommitted changes *(coming soon)* |
| `/status` | Show git status *(coming soon)* |
| `/commit` | Commit changes *(coming soon)* |
| `/revert` | Revert last agent changes *(coming soon)* |

---


## License

See [LICENSE](LICENSE).
