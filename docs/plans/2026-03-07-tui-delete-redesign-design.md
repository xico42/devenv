# TUI Delete Redesign — Design

## Problem

The current delete confirmation requires typing the exact branch name, which is tedious. It also always kills both agent and shell sessions with no granular choice. Session state is tracked in JSON files that can become orphaned if tmux sessions are killed externally.

## Solution

Two coupled changes:

1. Replace session state files with tmux `@` user-defined options (single source of truth)
2. Replace type-to-confirm dialog with an arrow-key navigable list of contextual choices

## Tmux State Layer

### Replace `internal/state/session.go`

Session state moves from `~/.local/share/devenv/sessions/*.json` to tmux `@` options set directly on tmux sessions.

**Properties stored on each agent tmux session:**

| Property | Example | Set by |
|---|---|---|
| `@devenv_status` | `"running"` / `"waiting"` | Claude Code hooks |
| `@devenv_question` | `"Which DB driver?"` | Claude Code hooks |
| `@devenv_started_at` | `"2026-03-07T10:30:00Z"` | Session creation |

Option name constants live in `internal/semconv/semconv.go` (e.g. `semconv.TmuxOptionStatus`, `semconv.TmuxOptionQuestion`, `semconv.TmuxOptionStartedAt`).

Shell sessions (`*~sh`) need no custom properties — their existence is sufficient.

### TmuxClient interface additions

```go
GetOption(session, option string) (string, error)
SetOption(session, option, value string) error
```

### Querying state

- Agent running? `tmuxClient.HasSession("myapp-feat")`
- Agent status? `tmuxClient.GetOption("myapp-feat", semconv.TmuxOptionStatus)`
- Shell running? `tmuxClient.HasSession("myapp-feat~sh")`

No reconciliation needed — if a tmux session is gone, there is no orphaned state.

### What gets removed

- `SessionState` struct
- `state.SaveSession()`, `LoadSession()`, `ClearSession()`, `ListSessions()`
- `~/.local/share/devenv/sessions/` directory concept
- `sessionsDir` field from TUI model

### Hook updates

- `devenv session mark-running` calls `tmux set-option` instead of `state.SaveSession()`
- `devenv notify send` (agent waiting) calls `tmux set-option` instead of updating state file
- Session creation sets `@devenv_started_at`

`internal/state/state.go` (droplet state) is unchanged.

## Confirm Dialog Redesign

### Interaction model

Arrow-key navigable list replaces the text input. Navigation matches the main list: `j`/`k`/arrows for movement, `Enter` to select, `Esc`/`q` to cancel.

### Contextual choices

Options shown depend on what's active for the selected worktree.

**Both sessions active:**

```
Delete -- myapp/feat
----
  ! Active sessions detected:
    * Agent session (running)
    * Shell session (running)

  > Delete everything (worktree + all sessions)
    Delete agent session only
    Delete shell session only
    Cancel
```

**Agent session only:**

```
Delete -- myapp/feat
----
  ! Active sessions detected:
    * Agent session (waiting)

  > Delete everything (worktree + agent session)
    Delete agent session only
    Cancel
```

**No active sessions:**

```
Delete -- myapp/feat
----
  > Delete worktree
    Cancel
```

### Main worktree protection

Pressing `d` on the main worktree shows a status message: "Cannot delete the main worktree". No dialog opens.

### Post-delete behavior

- Deleting only a session (not the worktree): item moves from `groupAgent` to `groupWorktree` on refresh
- Deleting everything: item moves to `groupProject` (or disappears if project has other worktrees)

## Changes by area

| Area | Change |
|---|---|
| `internal/tui/confirm.go` | Text input becomes navigable choice list |
| `internal/tui/actions.go` | Build choices from tmux state; handle each choice; block main worktree |
| `internal/tui/model.go` | Remove `sessionsDir`; refresh queries tmux |
| `internal/tui/keys.go` | j/k/arrows/Enter/Esc in confirm dialog |
| `internal/semconv/semconv.go` | Add `TmuxOptionStatus`, `TmuxOptionQuestion`, `TmuxOptionStartedAt` constants |
| `TmuxClient` interface | Add `GetOption`, `SetOption` |
| `cmd/session.go` | Hooks use `tmux set-option` |
| `cmd/worktree.go` | CLI delete drops state file cleanup |
| `internal/state/session.go` | Remove entirely |
| Tests | Mock tmux `@` options instead of state files |

## Scope

- Local sessions only (remote deferred)
- Cloud-init template unchanged
- Worktree creation flow unchanged
- List view grouping logic unchanged (different data source, same groups)
