# Design: `devenv worktree` Command

Date: 2026-03-06
PRD: `docs/prds/prd-phase-02-cmd-worktree.md`

## Overview

Implements the `devenv worktree` command with five subcommands: `list`, `new`, `delete`, `shell`, and `env`. All operations are local (no SSH, no droplet). Business logic lives in `internal/worktree`; tmux operations live in a new shared `internal/tmux` package.

---

## Package Structure

```
internal/
  tmux/
    runner.go        — Runner interface + RealRunner (os/exec)
    client.go        — Client struct with typed methods
    client_test.go
  worktree/
    worktree.go      — Service struct, WorktreeRunner interface, types
    worktree_test.go
cmd/
  worktree.go        — 5 Cobra subcommands, thin wiring to Service
```

---

## `internal/tmux`

Shared tmux abstraction used by `worktree` now and `session` later. Modelled after libtmux: a low-level subprocess seam (`Runner`) plus a typed high-level client (`Client`).

### `Runner` interface

```go
type Runner interface {
    Run(args ...string) (stdout, stderr string, exitCode int, err error)
}
```

`RealRunner` implements `Runner` via `os/exec`. This is the only place in the codebase that executes `tmux`.

### `Client`

```go
type Client struct{ runner Runner }

func NewClient(r Runner) *Client
func (c *Client) HasSession(name string) (bool, error)   // exit 1 = absent, not an error
func (c *Client) KillSession(name string) error
func (c *Client) NewSession(name, dir string) error      // for future session command
func (c *Client) ListSessions() ([]string, error)        // for future session command
```

`HasSession` treats tmux exit code 1 as "session absent" (not an error). All other non-zero exit codes are returned as errors. `RealRunner.Run` captures stderr and includes it in error messages.

---

## `internal/worktree`

### Types

```go
type WorktreeInfo struct {
    Path   string
    Branch string // empty = detached HEAD
}
```

### `WorktreeRunner` interface

```go
type WorktreeRunner interface {
    Add(cloneDir, worktreePath, branch string) error          // git worktree add <path> <branch>
    AddNewBranch(cloneDir, worktreePath, branch string) error // git worktree add -b <branch> <path>
    Remove(cloneDir, worktreePath string) error               // git worktree remove <path>
    List(cloneDir string) ([]WorktreeInfo, error)             // git worktree list --porcelain
}
```

`RealWorktreeRunner` implements this via `os/exec`. `List` parses `--porcelain` output.

### `Service`

```go
type Service struct {
    cfg  *config.Config
    git  WorktreeRunner
    tmux *tmux.Client
}

func NewService(cfg *config.Config, git WorktreeRunner, tmux *tmux.Client) *Service
```

### Sentinel errors

```go
var (
    ErrNotCloned        = errors.New("project not cloned")
    ErrWorktreeExists   = errors.New("worktree already exists")
    ErrWorktreeNotFound = errors.New("worktree not found")
    ErrSessionRunning   = errors.New("session is running")
)
```

`cmd` layer uses `errors.Is` to map these to PRD-specified user-facing messages.

### Path derivation

Given config name `myapp` with `repo = "git@github.com:user/myapp.git"` and `projects_dir = "~/projects"`:

| Concept | Value |
|---|---|
| `repoPath` | `github.com/user/myapp` |
| `cloneDir` | `~/projects/github.com/user/myapp` |
| `worktreesRoot` | `~/projects/github.com/user/myapp__worktrees` |
| `worktreePath` (branch `feature/login`) | `~/projects/github.com/user/myapp__worktrees/feature-login` |

Branch flattening: `/` → `-`. Applied to directory names only; session names use the config name.

### Template discovery (shared helper)

Used by both `worktree new` and `worktree env`:

```go
func resolveTemplate(worktreePath string, projCfg config.ProjectConfig) (content, source string, err error)
```

Resolution order:
1. `<worktreePath>/.env.template` — repo-local file (wins if both present), source = `"repo-local"`
2. `projCfg.EnvTemplate` — config-provided external path, source = the file path
3. If neither: returns `("", "", nil)` — callers decide whether to error or skip

---

## Data Flow per Subcommand

### `worktree new <project> <branch>`

1. Resolve `cloneDir` from config; check it exists → `ErrNotCloned` if absent
2. Derive `worktreesRoot` and `worktreePath`; check `worktreePath` absent → `ErrWorktreeExists` if present
3. Try `git.Add(cloneDir, worktreePath, branch)`; on "invalid reference" error, retry with `git.AddNewBranch`
4. Call `resolveTemplate`; if template found, run `envtemplate.Process` and write `.env` (failure is a warning, not fatal)
5. Return path for cmd to print

### `worktree list [project]`

1. Iterate configured projects (all or named)
2. For each: call `git.List(cloneDir)` → `[]WorktreeInfo`
3. For each entry: `tmux.HasSession("<configName>-<branch>")` → SESSION column
4. Print via `tabwriter`: `PROJECT | BRANCH | PATH | SESSION`

### `worktree delete <project> <branch>`

1. Resolve `worktreePath`; check it exists → `ErrWorktreeNotFound` if absent
2. Check `tmux.HasSession`; if running and not `--force` → `ErrSessionRunning`
3. If `--force` and session running: `tmux.KillSession`
4. Confirmation prompt `[y/N]` at cmd layer (skipped with `--force`)
5. `git.Remove(cloneDir, worktreePath)`

### `worktree shell <project> <branch>`

1. Resolve `worktreePath`; check it exists → `ErrWorktreeNotFound` if absent
2. `os.Chdir(worktreePath)` + `syscall.Exec($SHELL, [$SHELL], os.Environ())` — replaces process
3. Both chdir and exec at cmd layer (process-level, not business logic)

### `worktree env <project> <branch>`

1. Resolve `worktreePath`; check it exists → `ErrWorktreeNotFound` if absent
2. Call `resolveTemplate`; if no template found → error
3. Run `envtemplate.Process(content, source, ctx)`
4. `--dry-run`: print to stdout; otherwise write `.env` to worktree

---

## Error Handling in `cmd` Layer

```go
switch {
case errors.Is(err, worktree.ErrNotCloned):
    fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s is not cloned. Run 'devenv project clone %s' first.\n", project, project)
case errors.Is(err, worktree.ErrWorktreeExists):
    fmt.Fprintf(cmd.ErrOrStderr(), "Error: worktree %s/%s already exists.\n", project, branch)
case errors.Is(err, worktree.ErrWorktreeNotFound):
    fmt.Fprintf(cmd.ErrOrStderr(), "Error: worktree %s/%s not found.\n", project, branch)
case errors.Is(err, worktree.ErrSessionRunning):
    fmt.Fprintf(cmd.ErrOrStderr(), "Error: session %s-%s is running. Stop it first or use --force.\n", project, branch)
}
```

---

## Testing Strategy

| Package | Mock seam | What's tested |
|---|---|---|
| `internal/tmux` | `mockRunner` implementing `Runner` | `HasSession` exit-code logic, `KillSession`, output parsing |
| `internal/worktree` | `mockWorktreeRunner` + `tmux.NewClient(mockTmuxRunner)` | All `Service` methods: path derivation, branch flattening, error mapping, template discovery |
| `cmd/worktree` | — (thin layer) | Covered by service tests |

`Service` tests construct `tmux.NewClient(mockTmuxRunner)` — exercising real `Client` logic with controlled subprocess output. No separate tmux client interface needed.

Coverage target: ≥80% aggregate (enforced by `make coverage`).
