# PRD: `devenv worktree` (Phase 2 — Workload)

## Overview

The `worktree` command manages git worktrees for configured projects. Each worktree is an independent checkout at a given branch, enabling parallel Claude agent sessions without interference.

**Plane:** Workload (runs anywhere — no SSH, no droplet required). Commands execute `git` directly on the machine where devenv is invoked.

---

## Command Interface

```
devenv worktree <subcommand> [flags]
```

### Subcommands

| Subcommand | Description |
|---|---|
| `list [project]` | List worktrees (all projects, or a single project) |
| `new <project> <branch>` | Create a new worktree for a project |
| `delete <project> <branch>` | Delete a worktree |
| `shell <project> <branch>` | Open an interactive shell in a worktree |
| `env <project> <branch>` | (Re)generate `.env` from template |

---

## Directory Layout

Worktrees are grouped under a dedicated `__worktrees` directory next to the main clone. The double underscore avoids collisions with repo names.

```
~/projects/
  github.com/
    user/
      myapp/                       <- main clone (default branch)
      myapp__worktrees/
        feature/                   <- worktree for branch "feature"
        fix-123/                   <- worktree for branch "fix-123"
        feature-login/             <- worktree for branch "feature/login" (/ -> -)
      api/                         <- (different project — not a worktree of myapp)
  gitlab.com/
    corp/
      backend/
        api/                       <- main clone
        api__worktrees/
          experiment/              <- worktree for branch "experiment"
```

Worktrees are named by branch (flattened) and live under `<clone_dir>__worktrees/`. The main clone itself is the `default_branch` checkout and is managed by `devenv project clone`, not by this command.

- **`clone_dir`**: the main clone directory (e.g. `~/projects/github.com/user/myapp`)
- **`worktrees_root`**: `<clone_dir>__worktrees` (e.g. `~/projects/github.com/user/myapp__worktrees`)
- **Worktree path**: `<worktrees_root>/<branch>`

Branch names containing `/` are flattened to `-` in the directory name:
`feature/login` -> `feature-login`

---

## `devenv worktree list [project]`

Lists worktrees. If `project` is omitted, lists all worktrees across all configured projects.

```
devenv worktree list

  PROJECT     BRANCH        PATH                                                     SESSION
  myapp       main          ~/projects/github.com/user/myapp                         --
  myapp       feature       ~/projects/github.com/user/myapp__worktrees/feature      myapp-feature (running)
  myapp       fix-123       ~/projects/github.com/user/myapp__worktrees/fix-123      --
  work-api    develop       ~/projects/gitlab.com/corp/backend/api                   --
```

Runs `git worktree list --porcelain` for each project clone. SESSION column is populated from active tmux sessions (checks if a tmux session named `<config_name>-<branch>` exists).

---

## `devenv worktree new <project> <branch>`

Creates a new worktree for the given project at the given branch.

```
devenv worktree new myapp feature

Creating worktree myapp/feature...  done
  Path: ~/projects/github.com/user/myapp__worktrees/feature
```

### Behavior

- Resolves the clone directory from config: `<projects_dir>/<repo_path>` (see `prd-phase-02-cmd-project.md`)
- Requires that the project is already cloned (clone directory exists)
- Derives the worktrees root: `<clone_dir>__worktrees`
- Creates the `__worktrees` directory if it doesn't exist
- Derives the worktree path: `<worktrees_root>/<branch>`
- Runs from inside the main clone:
  ```bash
  cd <clone_dir>
  git worktree add <worktrees_root>/<branch> <branch>
  ```
- If the branch doesn't exist locally or remotely: creates it (`git worktree add -b <branch>`)
- If the worktree directory already exists: error
- After creating the worktree, processes `.env.template` if one exists (see `prd-phase-02-env.md`)

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| Project not cloned | `Error: myapp is not cloned. Run 'devenv project clone myapp' first.` | 1 |
| Worktree already exists | `Error: worktree myapp/feature already exists.` | 1 |
| git error | `Error: failed to create worktree: <git error>` | 1 |

---

## `devenv worktree delete <project> <branch>`

Deletes a worktree.

```
devenv worktree delete myapp feature

Delete worktree myapp/feature? [y/N] y
Deleting worktree myapp/feature...  done
```

### Behavior

- Runs `git worktree remove <worktrees_root>/<branch>` from inside the main clone
- If an active tmux session exists for this worktree: error with hint to stop the session first
- `--force` flag skips the confirmation prompt and kills any active session before deleting
- The main clone cannot be deleted with this command

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| Worktree doesn't exist | `Error: worktree myapp/feature not found.` | 1 |
| Active session exists | `Error: session myapp-feature is running. Stop it first or use --force.` | 1 |
| Uncommitted changes | git will error; message passed through as-is | 1 |

---

## `devenv worktree shell <project> <branch>`

Opens an interactive shell in the worktree directory.

```
devenv worktree shell myapp feature
```

### Behavior

- Changes to the worktree path and execs `$SHELL`
- Starts an interactive shell — not tmux, not Claude, just a plain shell
- Returns to the previous context when the shell exits
- If the worktree doesn't exist: error with hint to run `devenv worktree new`
- Uses `syscall.Exec` to replace the current process with the shell

This is intentionally a plain shell for ad-hoc inspection and manual commands. For persistent Claude sessions, use `devenv session start`.

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| Worktree doesn't exist | `Error: worktree myapp/feature not found. Run 'devenv worktree new myapp feature' first.` | 1 |
| Project not cloned | `Error: myapp is not cloned. Run 'devenv project clone myapp' first.` | 1 |

---

## `devenv worktree env <project> <branch>`

(Re)generates `.env` from the project's environment template. See `prd-phase-02-env.md` for full details.

```
devenv worktree env myapp feature

Processing .env.template...  done
  Env: ~/projects/github.com/user/myapp__worktrees/feature/.env (3 ports allocated)
  Ports: api=34821 db=18293 redis=42107
```

### Flags

| Flag | Description |
|---|---|
| `--dry-run` | Print generated `.env` to stdout without writing |

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| Worktree doesn't exist | `Error: worktree myapp/feature not found.` | 1 |
| No template found | `Error: no .env.template found for myapp (checked repo and config).` | 1 |
| Template syntax error | `Error: template error: <details>` | 1 |

---

## Path Resolution Summary

Given config name `myapp` with repo `git@github.com:user/myapp.git`:

| Concept | Path |
|---|---|
| repo_path | `github.com/user/myapp` |
| clone_dir | `~/projects/github.com/user/myapp` |
| worktrees_root | `~/projects/github.com/user/myapp__worktrees` |
| worktree_dir (branch `feature`) | `~/projects/github.com/user/myapp__worktrees/feature` |

Session names use the **config name** (not filesystem path): `myapp-feature`. This is the short, human-readable name used in tmux and CLI commands. The filesystem path is derived from the repo URL.

---

## Implementation Notes

- All operations are local filesystem + git commands — no SSH, no droplet state check
- `git worktree list --porcelain` is used for reliable machine-parseable output
- The `__worktrees` directory is created automatically by `worktree new` if it doesn't exist
- The main clone directory is treated as read-only by this command — it is managed by `devenv project clone`
- `projects_dir` is resolved from config at runtime (`~` expanded to `$HOME`)
- Path derivation uses `RepoPath()` from `internal/config` (see `prd-phase-02-cmd-project.md`)
- tmux session existence check for the SESSION column and delete safety check uses `tmux has-session -t <name>` via `os/exec` — if tmux is not running, all session checks return "no session" gracefully
