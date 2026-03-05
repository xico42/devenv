# PRD: `devenv worktree`

## Overview

The `worktree` command manages git worktrees on the active droplet. Each worktree is an independent checkout of a project repo at a given branch, enabling parallel Claude agent sessions without interference.

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
| `shell <project> <branch>` | Open an interactive terminal in a worktree |

---

## Directory Layout

```
~/projects/
  myapp/             ← main clone (default branch)
  myapp-feature/     ← worktree for branch "feature"
  myapp-fix-123/     ← worktree for branch "fix-123"
  api/
  api-experiment/
```

Worktrees are named `<project>-<branch>` and live as siblings of the main clone under `~/projects/`. The main clone itself is the `default_branch` checkout and is managed by `devenv project clone`, not by this command.

Branch names containing `/` are flattened to `-` in the directory name:
`feature/login` → `myapp-feature-login`

---

## `devenv worktree list [project]`

Lists worktrees on the droplet. If `project` is omitted, lists all worktrees across all configured projects.

```
devenv worktree list

  PROJECT    BRANCH        PATH                        SESSION
  myapp      main          ~/projects/myapp            —
  myapp      feature       ~/projects/myapp-feature    myapp-feature (running)
  myapp      fix-123       ~/projects/myapp-fix-123    —
  api        develop       ~/projects/api              —
```

SSHes into the droplet and runs `git worktree list` for each project clone. SESSION column is populated from active tmux sessions (see `prd-phase-03-cmd-session.md`).

---

## `devenv worktree new <project> <branch>`

Creates a new worktree for the given project at the given branch.

```
devenv worktree new myapp feature

Creating worktree myapp/feature...  ✓
  Path: ~/projects/myapp-feature
```

### Behavior

- Requires that the project is already cloned (`~/projects/<project>` exists)
- Runs from inside the main clone:
  ```bash
  git worktree add ~/projects/<project>-<branch> <branch>
  ```
- If the branch doesn't exist locally or remotely: creates it (`git worktree add -b <branch>`)
- If `~/projects/<project>-<branch>` already exists: error

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
Deleting worktree myapp/feature...  ✓
```

### Behavior

- Runs `git worktree remove ~/projects/<project>-<branch>` from inside the main clone
- If an active Claude session exists for this worktree: error with hint to stop the session first
- `--force` flag skips the confirmation prompt and kills any active session before deleting
- The main clone (`~/projects/<project>`) cannot be deleted with this command — use `devenv project` for that

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| Worktree doesn't exist | `Error: worktree myapp/feature not found.` | 1 |
| Active session exists | `Error: session myapp-feature is running. Stop it first or use --force.` | 1 |
| Uncommitted changes | git will error; message passed through as-is | 1 |

---

## `devenv worktree shell <project> <branch>`

Opens an interactive terminal session inside the worktree directory.

```
devenv worktree shell myapp feature
```

### Behavior

- SSHes into the droplet with the working directory set to `~/projects/<project>-<branch>`
- Starts an interactive shell — not tmux, not Claude, just a plain shell
- Returns to local terminal when the SSH session ends
- If the worktree doesn't exist: error with hint to run `devenv worktree new`

This is intentionally a plain shell for ad-hoc inspection and manual commands. For persistent Claude sessions, use `devenv session start`.

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| No active droplet | `Error: no active droplet. Run 'devenv up' first.` | 1 |
| Worktree doesn't exist | `Error: worktree myapp/feature not found. Run 'devenv worktree new myapp feature' first.` | 1 |

---

## Implementation Notes

- All operations are SSH commands over the Tailscale IP
- The main clone directory (`~/projects/<project>`) is treated as read-only by this command — it is managed by `devenv project clone`
- `git worktree list --porcelain` is used for reliable machine-parseable output when listing
