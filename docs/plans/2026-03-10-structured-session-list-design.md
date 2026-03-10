# Structured Session List Design

**Date:** 2026-03-10

## Problem

Session names change when `SetStatus` adds or removes the `⚡ ` prefix. Two places depend on resolving the actual tmux session name from the original (unprefixed) name stored in `DEVENV_SESSION`:

1. `SetStatus` in `session/session.go` — must find the actual session to set options and rename it.
2. `refreshCmd` in `tui/model.go` — must match sessions to worktrees for display.

The current `SetStatus` probes with `HasSession("⚡ " + name)` — a separate round-trip that is fragile and tightly coupled to the naming convention. The TUI refresh also makes N×2 extra `GetOption` calls (status + annotation per session) after listing names, and classifies sessions by name heuristics (`~sh` suffix).

## Solution

Store the original session name as `@devenv_canonical_name` and session type as `@devenv_session_type` on every devenv tmux session at creation time. Replace the `ListSessions() []string` API with `ListSessions() []SessionRecord`, fetching all options in a single `tmux list-sessions -F ...` call. Use `CanonicalName` for stable session lookup in `SetStatus` and for direct keying in TUI refresh. Use `SessionType` for classification instead of name heuristics.

## Semconv Additions

```go
TmuxOptionCanonicalName = "@devenv_canonical_name"
TmuxOptionSessionType   = "@devenv_session_type"

SessionTypeAgent = "agent"
SessionTypeShell = "shell"
```

## Data Model

New `SessionRecord` struct in `internal/tmux/client.go`:

```go
type SessionRecord struct {
    Name          string // current tmux session name (may have "⚡ " prefix for agents)
    CanonicalName string // @devenv_canonical_name — original name, never changes
    SessionType   string // @devenv_session_type — "agent" or "shell"
    Status        string // @devenv_status
    Annotation    string // @devenv_annotation
    StartedAt     string // @devenv_started_at (raw RFC3339 string)
}
```

Note: shell sessions retain the `~sh` suffix in their tmux name for usability in tmux's session picker, but `CanonicalName` is stored as `project-branch` (without `~sh`) so it aligns with `semconv.SessionName` lookups.

## Component Changes

### `internal/tmux/client.go`

`ListSessions` signature changes from `([]string, error)` to `([]SessionRecord, error)`.

Implementation: one `tmux list-sessions` call with a tab-delimited format string:

```
#{session_name}\t#{@devenv_canonical_name}\t#{@devenv_session_type}\t#{@devenv_status}\t#{@devenv_annotation}\t#{@devenv_started_at}
```

Each output line is split on `\t` into six fields. Sessions without devenv options have empty fields and are ignored by callers.

### `internal/session/session.go`

**`Start`** — store canonical name and session type immediately after session creation:

```go
_ = s.tmux.SetOption(name, semconv.TmuxOptionCanonicalName, name)
_ = s.tmux.SetOption(name, semconv.TmuxOptionSessionType, semconv.SessionTypeAgent)
```

**`List`** — replace per-session `GetOption` calls with a single `ListSessions()` call, populating `SessionInfo` directly from record fields.

**`Show`** — same: list once, find by canonical name.

**`SetStatus`** — replace `HasSession` probe with a list scan by canonical name:

```go
records, _ := s.tmux.ListSessions()
actualName := ""
for _, r := range records {
    if r.CanonicalName == name {
        actualName = r.Name
        break
    }
}
if actualName == "" { return nil }
_ = s.tmux.SetOption(actualName, ...)
// add/remove prefix on actualName as before
```

### `internal/tui/actions.go`

Shell session creation sets both options after `NewSession`:

```go
_ = tmuxClient.SetOption(shellName, semconv.TmuxOptionCanonicalName, sessionName) // "project-branch"
_ = tmuxClient.SetOption(shellName, semconv.TmuxOptionSessionType, semconv.SessionTypeShell)
```

### `internal/tui/model.go`

Replace the two-step (list names → per-session GetOption) with a single structured list, dispatching on `SessionType`:

```go
records, err := tmuxClient.ListSessions()
for _, r := range records {
    switch r.SessionType {
    case semconv.SessionTypeShell:
        data.shellSessions[r.CanonicalName] = true
    case semconv.SessionTypeAgent:
        data.agentSessions[r.CanonicalName] = agentInfo{
            status:     r.Status,
            annotation: r.Annotation,
        }
    }
}
```

No name heuristics. Non-devenv sessions (empty `SessionType`) are skipped implicitly.

### `internal/tui/items.go`

`HasShell` lookup changes from `shellName` to `sessionName`:

```go
HasShell: data.shellSessions[sessionName], // sessionName = semconv.SessionName(project, branch)
```

## Testing

- **`tmux/client_test.go`** — update `ListSessions` tests for the tab-delimited format; verify all six fields are parsed; verify empty `SessionType` for non-devenv sessions.
- **`session/session_test.go`** — mock runner returns tab-delimited lines; test `Start` writes `@devenv_canonical_name` and `@devenv_session_type`; test `SetStatus` resolves by canonical name when session has the `⚡ ` prefix.
- **`tui/model_test.go`** — update `refreshCmd` test to use tab-delimited mock output with `SessionType` fields.
- **Coverage** — `make coverage` must remain ≥ 80%.
