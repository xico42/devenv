# Design: `devenv session` command

## Overview

The `session` command manages agent sessions. Each session is a named tmux session running an agent (default: `claude`) in a specific project worktree. Sessions persist independently of the terminal connection.

**Plane:** Workload (runs anywhere, no SSH, no droplet required).

## Decisions

### Architecture: service layer in `internal/session`

A `session.Service` struct owns the business logic. The cmd layer is thin — Cobra wiring, config resolution, output formatting. Matches the pattern established by `internal/worktree` and `internal/project`.

The service takes `*tmux.Client` and a sessions directory path. It does **not** take `*config.Config` — the cmd layer resolves agent config and worktree paths, passing primitives to the service.

### Shared conventions: `internal/semconv`

Pure functions with zero internal dependencies, encoding the project's naming and path conventions. Extracted from private helpers currently in `internal/worktree`.

Functions:
- `FlattenBranch(branch string) string` — `feature/login` → `feature-login`
- `SessionName(project, branch string) string` — `myapp` + `feature/login` → `myapp-feature-login`
- `CloneDir(projectsDir, repoPath string) string`
- `WorktreesRoot(projectsDir, repoPath string) string`
- `WorktreePath(projectsDir, repoPath, branch string) string`

Constants:
- `SessionEnvVar = "DEVENV_SESSION"`
- `DefaultAgentCmd = "claude"`

The `internal/worktree` package is refactored to call these instead of its private helpers.

### Agent harness configuration

The agent command is configurable, not hardcoded to `claude`. Config uses a nested `[agent]` section under `[defaults]` and `[projects.<name>]`:

```toml
[defaults.agent]
cmd = "claude"
args = ["--dangerously-skip-permissions"]

[defaults.agent.env]
CLAUDE_CONFIG_DIR = "/custom/path"

[projects.myapp.agent]
cmd = "aider"
args = ["--model", "opus"]

[projects.myapp.agent.env]
AIDER_MODEL = "opus"
```

Go struct:

```go
type AgentConfig struct {
    Cmd  string            `toml:"cmd"`
    Args []string          `toml:"args"`
    Env  map[string]string `toml:"env"`
}
```

Added to both `DefaultsConfig` and `ProjectConfig`.

Resolution: per-project `cmd` and `args` replace global if set. `env` merges (project wins on key conflict).

### No CPU/MEM stats

Deferred. `list` and `show` omit CPU and MEM columns. These require `/proc` parsing and new tmux client methods — unnecessary for the core lifecycle.

### No `--export-env` flag

Deferred. `session start` does not read `.env` files from the worktree. The `Env` map contains only agent config env vars plus `DEVENV_SESSION`.

### Flattened session names

Branch slashes are flattened in session names: `feature/login` → `myapp-feature-login`. Consistent with worktree directory naming.

## Components

### `internal/semconv`

New package. Pure functions, zero dependencies on other `internal/` packages. Defines naming and path conventions shared across `worktree`, `session`, and future commands.

### `internal/config` changes

Add `AgentConfig` struct. Embed it in `DefaultsConfig.Agent` and `ProjectConfig.Agent`. Add `ResolveAgent(project string) AgentConfig` helper that merges per-project over defaults.

### `internal/tmux` additions

Two new methods on `Client`:

```go
func (c *Client) NewSessionWithEnv(name, dir string, env map[string]string, cmd string) error
func (c *Client) ExecAttach(name string) error
```

`NewSessionWithEnv` builds `tmux new-session -d -s <name> -c <dir> -e K=V ... <cmd>`.

`ExecAttach` uses `syscall.Exec` to replace the process with `tmux attach-session -t <name>`.

### `internal/session`

```go
type Service struct {
    tmux        *tmux.Client
    sessionsDir string
}

type StartRequest struct {
    Project string
    Branch  string
    Path    string
    Cmd     string
    Env     map[string]string
    Attach  bool
}

type SessionInfo struct {
    Name      string
    Project   string
    Branch    string
    Status    string    // "running", "waiting", "unknown"
    Question  string
    Path      string
    StartedAt time.Time
    UpdatedAt time.Time
}

func (s *Service) Start(req StartRequest) error
func (s *Service) List() ([]SessionInfo, error)
func (s *Service) Show(name string) (*SessionInfo, error)
func (s *Service) Stop(name string) error
func (s *Service) Attach(name string) error
func (s *Service) MarkRunning(name string) error
```

Behaviors:
- **Start** — verifies path exists, checks no duplicate tmux session, calls `tmux.NewSessionWithEnv`, writes initial state file with `status: "running"`. Optionally attaches.
- **List** — queries tmux for all sessions, enriches with state files. No state file → `status: "unknown"`.
- **Show** — single session detail. Error if tmux session not found.
- **Stop** — kills tmux session, deletes state file. Error if session not found.
- **Attach** — verifies session exists, `syscall.Exec` into tmux attach. No return on success.
- **MarkRunning** — loads state file, sets `status: "running"`, clears `question`. No-op if file missing or `--session` empty. Never fails loudly (exit 0 always).

### `cmd/session.go`

Six subcommands:

```
devenv session start <project> <branch> [--attach]
devenv session list
devenv session show <session>
devenv session attach <session>
devenv session stop <session> [--force]
devenv session mark-running --session <name>    (hidden)
```

Cmd layer responsibilities:
- Resolve agent config (per-project → defaults fallback, merge env)
- Resolve worktree path via `worktree.Service.WorktreePath()`
- Join `agent.cmd` + `agent.args` into the command string
- Build env map: agent config env + `DEVENV_SESSION`
- `--force` on stop: skip confirmation; without it, prompt `y/N` if status is `running`
- `mark-running`: hidden (`Hidden: true`), reads `--session` flag
- Output formatting: tabwriter for list, key-value for show
- Error translation: sentinel errors → user-friendly messages

## Session status lifecycle

| Event | Status | Question | Set by |
|---|---|---|---|
| `session start` | `running` | `""` | This command |
| Agent needs input | `waiting` | question text | `notify send --session` (future, not this PR) |
| Agent resumes | `running` | `""` (cleared) | `session mark-running` |
| `session stop` | *(deleted)* | — | This command |
| Manual tmux session | `unknown` | — | No state file |

The `waiting` status is set by `devenv notify send` (PostToolUse hook), which is a separate Phase 2 command. Until it's implemented, sessions will only show `running` or `unknown`.

## Testing

- **`internal/semconv`** — table-driven tests on pure functions.
- **`internal/tmux`** — new methods tested via existing `mockRunner`. `ExecAttach` tests verify correct args without calling `syscall.Exec`.
- **`internal/session`** — mock `tmux.Client` via `Runner` interface, real temp dir for state files. Covers: start happy path, duplicate session, missing path, list with mixed state, stop + cleanup, mark-running no-op, mark-running resets status.
- **`cmd/session.go`** — agent config resolution tested at config level. Command output tested if pattern exists from other commands.
- **Coverage** — `make coverage` must stay at or above 80%.
