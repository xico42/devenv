# PRD: `devenv session` (Phase 2 — Workload)

## Overview

The `session` command manages Claude Code agent sessions. Each session is a named tmux session running Claude in a specific project worktree. Sessions persist independently of the terminal connection — the core of the mobile workflow: start a session, disconnect, get notified when Claude needs input, attach and respond.

**Plane:** Workload (runs anywhere — no SSH, no droplet required). Commands execute `tmux` directly on the machine where devenv is invoked.

---

## Command Interface

```
devenv session <subcommand> [flags]
```

### Subcommands

| Subcommand | Description |
|---|---|
| `start <project> <branch>` | Start a new Claude session in a worktree |
| `list` | List all active sessions with status and resource usage |
| `attach <session>` | Attach to an existing session interactively |
| `stop <session>` | Kill a session |
| `show <session>` | Show full details for a single session |
| `mark-running` | Internal: reset session status to running (called by hook) |

---

## Session Model

- Each session = one tmux session
- Session name: `<project>-<branch>` (e.g. `myapp-feature`)
- Claude runs as the main process in the tmux window
- The `DEVENV_SESSION` environment variable is set in the tmux session — used by hooks to identify and update session state

---

## `devenv session start <project> <branch>`

Starts a new Claude session in the specified worktree.

```
devenv session start myapp feature

Starting session myapp-feature...  done
Session is running. Attach with: devenv session attach myapp-feature
```

### Behavior

1. Resolves the worktree path from config: `<projects_dir>/<repo_path>__worktrees/<branch>` (see `prd-phase-02-cmd-worktree.md` for path resolution)
2. Verifies the worktree directory exists
3. Verifies no tmux session named `<project>-<branch>` already exists
4. Creates a detached tmux session:
   ```bash
   tmux new-session -d \
     -s "<project>-<branch>" \
     -e "DEVENV_SESSION=<project>-<branch>" \
     -c "<worktree_path>" \
     "claude"
   ```
5. Writes initial session state file:
   `~/.local/share/devenv/sessions/<session>.json` with `status: "running"`
6. Returns immediately — does not attach

### Flags

| Flag | Description |
|---|---|
| `--attach` | Attach to the session immediately after starting |
| `--export-env` | Read `<worktree>/.env` and set all variables in the tmux session environment (see `prd-phase-02-env.md`) |

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| Worktree doesn't exist | `Error: worktree myapp/feature not found. Run 'devenv worktree new myapp feature' first.` | 1 |
| Session already exists | `Error: session myapp-feature already exists. Attach with 'devenv session attach myapp-feature'.` | 1 |
| tmux not available | `Error: tmux is not installed.` | 1 |

---

## `devenv session list`

Lists all active Claude sessions with status and resource usage.

```
devenv session list

  SESSION          PROJECT    BRANCH        STATUS              CPU    MEM
  myapp-feature    myapp      feature       waiting for input    0%    420MB
  api-experiment   api        experiment    running             14%    380MB
  myapp-fix-123    myapp      fix-123       running              8%    390MB
```

### Data sources

| Column | Source |
|---|---|
| SESSION, PROJECT, BRANCH | Parsed from tmux session names (`tmux list-sessions`) |
| STATUS | `~/.local/share/devenv/sessions/<name>.json` written by hook |
| CPU, MEM | `/proc/<pid>/stat` where `<pid>` is the tmux session's foreground process PID |

If no state file exists for a tmux session (e.g. session started manually), status shows as `unknown`.

---

## `devenv session attach <session>`

Attaches to an existing session interactively.

```
devenv session attach myapp-feature
```

### Behavior

- Runs `tmux attach-session -t <session>` directly
- Uses `syscall.Exec` to replace the current process — clean signal handling
- When the user detaches from tmux (`Ctrl+B d`), the local terminal is restored
- The session continues running after detach

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| Session not found | `Error: session myapp-feature not found.` | 1 |

---

## `devenv session stop <session>`

Kills a session and cleans up its state file.

```
devenv session stop myapp-feature

Stop session myapp-feature? [y/N] y
Stopping myapp-feature...  done
```

### Behavior

- Runs `tmux kill-session -t <session>`
- Deletes `~/.local/share/devenv/sessions/<session>.json`
- If the session status is `running` (Claude mid-task): warns before prompting, unless `--force`

### Flags

| Flag | Description |
|---|---|
| `--force` | Skip confirmation prompt; kill even if session is mid-task |

---

## `devenv session show <session>`

Shows full details for a single session.

```
devenv session show myapp-feature

  Session:   myapp-feature
  Project:   myapp
  Branch:    feature
  Path:      ~/projects/github.com/user/myapp__worktrees/feature
  Status:    waiting for input
  Question:  "Should I overwrite the existing migration? [y/N]"
  Since:     3m ago
  CPU:       0%
  MEM:       420MB
  Started:   2026-03-04T16:22:01Z
```

---

## `devenv session mark-running`

Internal subcommand (not user-facing). Called by the PreToolUse hook to reset session status to `running` and clear the `question` field.

```
devenv session mark-running --session "$DEVENV_SESSION"
```

Exits 0 silently — never blocks Claude. If `--session` is empty or the state file doesn't exist, exits 0 (no-op).

---

## Session State File

Written to `~/.local/share/devenv/sessions/<session-name>.json` by devenv commands and Claude Code hooks.

### Schema

```json
{
  "session":    "myapp-feature",
  "project":    "myapp",
  "branch":     "feature",
  "status":     "running",
  "question":   null,
  "updated_at": "2026-03-04T16:22:01Z",
  "started_at": "2026-03-04T16:22:01Z"
}
```

### Status transitions

| Event | Status | `question` |
|---|---|---|
| `devenv session start` | `running` | null |
| PostToolUse / AskUserQuestion hook fires (`devenv notify send --session`) | `waiting` | question text |
| PreToolUse hook fires (`devenv session mark-running`) | `running` | null |
| `devenv session stop` | file deleted | -- |

---

## Implementation Notes

- All tmux operations use `os/exec` to run `tmux` commands directly — no SSH
- `devenv session attach` uses `syscall.Exec` to replace the devenv process with `tmux attach-session` — clean signal handling, same pattern as `devenv worktree shell`
- Session names are the ground truth — `devenv session list` queries tmux for existence and reads state files only for status enrichment
- Sessions started manually (not via `devenv session start`) appear in `list` with `status: unknown` — they are not hidden
- CPU/MEM reads `/proc/<pid>/stat` directly — no persistent agent, no daemon
- The `mark-running` PreToolUse hook fires before every tool use, including the first — this is intentional and harmless (writing `running` when already `running` is a no-op)
- `projects_dir` is resolved from config at runtime
