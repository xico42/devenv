# Session Status Plugin Design

## Summary

Replace `mark-running` and `notify send` with a unified `devenv plugin handle-claude` command that reads Claude Code hook JSON from stdin and manages session status. Add desktop notifications via beeep when sessions need attention. Distribute as a Claude Code plugin with marketplace support from the devenv repo itself.

## CLI Surface

```
devenv plugin handle-claude
```

- Reads JSON from stdin (Claude Code hook payload)
- Dispatches based on `hook_event_name`:
  - `PreToolUse` — sets session status to `running`, clears annotation
  - `Notification` — sets session status to `waiting`, annotation from `message` field, fires desktop notification
  - `Stop` — sets session status to `waiting`, annotation from `last_assistant_message` (truncated), fires desktop notification
- Session identified from `$DEVENV_SESSION` env var
- Fail-open: exits 0 on any error
- Hidden command (internal, called by hooks)

### Statuses

Fixed enum: `running`, `waiting`. Validated on input. Extensible later.

### Tmux session rename

When status changes to anything other than `running`, prefix the tmux session name with `⚡`. Remove the prefix when status returns to `running`. Marker defined as a constant in `semconv`.

## Package Layout

```
# Repo root
.claude-plugin/
  marketplace.json              # marketplace registry

plugins/claude/
  .claude-plugin/
    plugin.json                 # plugin manifest
  hooks/
    hooks.json                  # wires Notification, Stop, PreToolUse → devenv plugin handle-claude
  README.md                     # setup instructions

cmd/plugin.go                   # cobra command: devenv plugin handle-claude

internal/session/status.go      # SetStatus(name, status, annotation) + tmux rename logic
internal/notify/notify.go       # thin wrapper around beeep for desktop notifications
internal/semconv/semconv.go     # updated: annotation constant, status prefix marker
```

### Marketplace config (`.claude-plugin/marketplace.json`)

```json
{
  "name": "devenv-plugins",
  "owner": { "name": "xico42" },
  "plugins": [
    {
      "name": "devenv-session-status",
      "source": "./plugins/claude",
      "description": "Tracks agent session status and sends desktop notifications"
    }
  ]
}
```

### Plugin hook config (`plugins/claude/hooks/hooks.json`)

```json
{
  "hooks": {
    "PreToolUse": [{ "hooks": [{ "type": "command", "command": "devenv plugin handle-claude" }] }],
    "Notification": [{ "hooks": [{ "type": "command", "command": "devenv plugin handle-claude" }] }],
    "Stop": [{ "hooks": [{ "type": "command", "command": "devenv plugin handle-claude" }] }]
  }
}
```

## Data Flow

```
Claude hook fires (Notification/Stop/PreToolUse)
  │
  ├─ stdin: JSON with hook_event_name, message, last_assistant_message, etc.
  │
  ▼
devenv plugin handle-claude
  │
  ├─ Reads JSON from stdin
  ├─ Reads $DEVENV_SESSION for session name
  │
  ├─ hook_event_name == "PreToolUse"
  │   └─ session.SetStatus(name, "running", "")
  │       ├─ Sets @devenv_status = "running"
  │       ├─ Clears @devenv_annotation
  │       └─ Removes ⚡ prefix from tmux session name (if present)
  │
  ├─ hook_event_name == "Notification"
  │   └─ session.SetStatus(name, "waiting", message)
  │       ├─ Sets @devenv_status = "waiting"
  │       ├─ Sets @devenv_annotation = message
  │       ├─ Adds ⚡ prefix to tmux session name
  │       └─ notify.Send("devenv", truncated_message)
  │
  └─ hook_event_name == "Stop"
      └─ session.SetStatus(name, "waiting", truncated_last_assistant_message)
          ├─ (same as above)
          └─ notify.Send("devenv", truncated_message)

TUI (refreshes every 3s)
  │
  ├─ Reads @devenv_status and @devenv_annotation from tmux
  └─ Displays annotation preview (truncated to ~60 chars) in session list
```

## Deletions

### Files removed

- `cmd/notify.go` — entire notify command
- `internal/config/notify.go` — all provider config structs
- `internal/config/notify_test.go` — notify config tests
- `internal/session/session_test.go` — `TestMarkRunning_*` tests
- `docs/prds/prd-phase-03-cmd-notify.md` — notify PRD

### Code removed

- `cmd/session.go` — `mark-running` subcommand registration
- `internal/session/session.go` — `MarkRunning()` method

### Files modified

- `internal/config/config.go` — remove `Notify NotifyConfig` field from `Config` struct, remove `case "notify"` in field lookup
- `cmd/config.go` — remove notify display in `config show`, remove `case "notify"` in `config get`
- `cmd/config_test.go` — remove notify-related test cases
- `cmd/root_test.go` — remove any notify references
- `internal/semconv/semconv.go` — rename `TmuxOptionQuestion` → `TmuxOptionAnnotation`

## TUI Changes

When a session is waiting, display annotation as a third line:

```
⚡ myapp / feature-auth
  WAITING FOR INPUT                shell
  Claude needs your permission to use Bash
```

- `delegate.go` — render annotation as a third line, truncated to ~60 chars, dimmed style
- `items.go` — rename `Question` field to `Annotation`
- `model.go` — update option name from `TmuxOptionQuestion` to `TmuxOptionAnnotation`
- Waiting sessions sort above running ones within the agent group

## Dependencies

- `github.com/gen2brain/beeep` — cross-platform desktop notifications (1.7k stars, actively maintained, native APIs with shell fallbacks)

## Testing

- `cmd/plugin_test.go` — test `handle-claude` with stdin JSON for each event type, missing `$DEVENV_SESSION`, malformed JSON, unknown event names
- `internal/session/status_test.go` — test `SetStatus()`: sets tmux options, renames session with/without prefix, idempotent prefix handling, clears annotation on `running`
- `internal/notify/notify_test.go` — test `Send()` behind an interface so tests don't fire real notifications
- Coverage must stay at or above 80% (`make coverage`)
