# PRD: `devenv session`

## Overview

The `session` command manages Claude Code agent sessions on the active droplet. Each session is a named tmux session running Claude in a specific project worktree. Sessions persist on the droplet independently of the local connection — the core of the mobile workflow: start a session, disconnect, get notified when Claude needs input, attach and respond.

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

---

## Session Model

- Each session = one tmux session on the droplet
- Session name: `<project>-<branch>` (e.g. `myapp-feature`)
- Claude runs as the main process in the tmux window
- The `DEVENV_SESSION` environment variable is set in the tmux session — used by hooks to identify and update session state

---

## `devenv session start <project> <branch>`

Starts a new Claude session in the specified worktree.

```
devenv session start myapp feature

Starting session myapp-feature...  ✓
Session is running. Attach with: devenv session attach myapp-feature
```

### Behavior

1. Verifies the worktree `~/projects/<project>-<branch>` exists on the droplet
2. Verifies no tmux session named `<project>-<branch>` already exists
3. SSHes into the droplet and creates a detached tmux session:
   ```bash
   tmux new-session -d \
     -s "<project>-<branch>" \
     -e "DEVENV_SESSION=<project>-<branch>" \
     -c "~/projects/<project>-<branch>" \
     "claude"
   ```
4. Writes initial session state file on the droplet:
   `~/.local/share/devenv/sessions/<session>.json` with `status: "running"`
5. Returns immediately — does not attach

### Flags

| Flag | Description |
|---|---|
| `--attach` | Attach to the session immediately after starting |

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| No active droplet | `Error: no active droplet. Run 'devenv up' first.` | 1 |
| Worktree doesn't exist | `Error: worktree myapp/feature not found. Run 'devenv worktree new myapp feature' first.` | 1 |
| Session already exists | `Error: session myapp-feature already exists. Attach with 'devenv session attach myapp-feature'.` | 1 |

---

## `devenv session list`

Lists all active Claude sessions on the droplet with status and resource usage.

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
| CPU, MEM | `/proc/<pid>/stat` via SSH, where `<pid>` is the tmux session's foreground process PID |

If no state file exists for a tmux session (e.g. session started manually), status shows as `unknown`.

---

## `devenv session attach <session>`

Attaches to an existing session interactively.

```
devenv session attach myapp-feature
```

### Behavior

- SSHes into the droplet with `-t` (force TTY allocation)
- Runs `tmux attach-session -t <session>`
- Hands the terminal off completely — the local process blocks until the user detaches or the SSH session ends
- When the user detaches from tmux (`Ctrl+B D`), SSH exits and the local terminal is restored
- The session continues running on the droplet after detach
- Uses `syscall.Exec` to replace the current process with SSH — clean signal handling

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
Stopping myapp-feature...  ✓
```

### Behavior

- Runs `tmux kill-session -t <session>` on the droplet
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
  Path:      ~/projects/myapp-feature
  Status:    waiting for input
  Question:  "Should I overwrite the existing migration? [y/N]"
  Since:     3m ago
  CPU:       0%
  MEM:       420MB
  Started:   2026-03-04T16:22:01Z
```

---

## Session State File

Written to `~/.local/share/devenv/sessions/<session-name>.json` on the droplet by the Claude Code hooks.

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
| PostToolUse / AskUserQuestion hook fires | `waiting` | question text |
| PreToolUse hook fires (Claude resumes) | `running` | null |
| `devenv session stop` | file deleted | — |

---

## Hook Integration

`devenv session start` sets `DEVENV_SESSION` in the tmux environment. The hooks written to `~/.claude/settings.json` by cloud-init reference this variable.

The `settings.json` hook spec (managed by `devenv up` via cloud-init):

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "AskUserQuestion",
        "hooks": [
          {
            "type": "command",
            "command": "devenv notify send --title 'Claude needs input' --message \"$CLAUDE_TOOL_INPUT_QUESTION\" --session \"$DEVENV_SESSION\""
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "devenv session mark-running --session \"$DEVENV_SESSION\""
          }
        ]
      }
    ]
  }
}
```

### `devenv notify send --session`

When `--session` is passed, `devenv notify send` writes the "waiting" state to the session file as a side effect in addition to dispatching the notification. This keeps the two concerns in one hook command for the `AskUserQuestion` case.

### `devenv session mark-running`

Internal subcommand (not user-facing). Called by the PreToolUse hook to reset session status to `running` and clear the `question` field. Exits 0 silently — never blocks Claude.

---

## Implementation Notes

- `devenv session attach` uses `syscall.Exec` — same pattern as `devenv ssh` — so the local process is fully replaced by SSH and signals propagate correctly
- Session names are the ground truth — `devenv session list` queries tmux for existence and reads state files only for status enrichment
- Sessions started manually (not via `devenv session start`) appear in `list` with `status: unknown` — they are not hidden
- CPU/MEM polling uses a single SSH connection per `list`/`show` call — no persistent agent on the droplet
- The `mark-running` PreToolUse hook fires before every tool use, including the first — this is intentional and harmless (writing `running` when already `running` is a no-op)
