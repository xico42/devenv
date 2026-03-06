# Design: Adapt Scaffold (Phase 1)

## Overview

Adapt the Phase 0 scaffold to the local-execution architecture. Remove the SSH remote package, extend config with projects/notifications, add session state persistence, and add a git URL-to-path helper.

## Approach

Split new code into sub-files by concern (Approach B from brainstorming). Keeps the codebase navigable as it grows while maintaining the existing package boundaries.

## Deletions

- Delete `internal/remote/remote.go` and `internal/remote/remote_test.go`
- Remove `golang.org/x/crypto` from `go.mod` via `go mod tidy`
- No other package imports `internal/remote`; clean removal

## Config extensions

### Modified: `internal/config/config.go`

Extend existing structs:

```go
type Config struct {
    Defaults DefaultsConfig             `toml:"defaults"`
    Profiles map[string]ProfileConfig   `toml:"profiles"`
    Projects map[string]ProjectConfig   `toml:"projects"`
    Notify   NotifyConfig               `toml:"notify"`
    path     string
}

type DefaultsConfig struct {
    // ... existing fields ...
    ProjectsDir     string `toml:"projects_dir"`
    GitIdentityFile string `toml:"git_identity_file"`
}
```

`ProjectsDir` defaults to `~/projects` and gets `~` expanded to `$HOME` at load time. `GitIdentityFile` is optional (used by `devenv up` to SCP a key to the droplet).

### New: `internal/config/project.go`

```go
type ProjectConfig struct {
    Repo          string `toml:"repo"`
    DefaultBranch string `toml:"default_branch"`
    EnvTemplate   string `toml:"env_template"`
}
```

`EnvTemplate` gets `~` expanded at load time.

Also contains `RepoPath()` (see below).

### New: `internal/config/notify.go`

All notification config structs — pure data, no methods:

- `NotifyConfig` — top-level with `Provider` string and sub-structs
- `TelegramNotifyConfig` — `BotToken`, `ChatID`
- `SlackNotifyConfig` — `WebhookURL`
- `DiscordNotifyConfig` — `WebhookURL`
- `WebhookNotifyConfig` — `URL`, `Method` (default "POST"), `Headers`, `BodyTemplate`

## `RepoPath()` function

Lives in `internal/config/project.go`.

```go
func RepoPath(repoURL string) (string, error)
```

Parses a git remote URL and returns a relative directory path:

| Input | Output |
|---|---|
| `git@github.com:user/myapp.git` | `github.com/user/myapp` |
| `ssh://git@github.com/user/myapp.git` | `github.com/user/myapp` |
| `https://github.com/user/myapp.git` | `github.com/user/myapp` |
| `git@gitlab.com:corp/group/api.git` | `gitlab.com/corp/group/api` |

Rules: strip `.git` suffix, strip leading `/` from path, preserve all path components. Error on empty or unparseable input.

Implementation: detect SCP-style (`user@host:path`) via string splitting on `:`. Otherwise `net/url.Parse()` for `ssh://` and `https://` schemes.

## Session state

### New: `internal/state/session.go`

```go
const (
    SessionRunning = "running"
    SessionWaiting = "waiting"
)

type SessionState struct {
    Session   string    `json:"session"`
    Project   string    `json:"project"`
    Branch    string    `json:"branch"`
    // Status is the session's current state.
    // Valid values: SessionRunning ("running"), SessionWaiting ("waiting").
    Status    string    `json:"status"`
    Question  string    `json:"question"`
    UpdatedAt time.Time `json:"updated_at"`
    StartedAt time.Time `json:"started_at"`
}

func LoadSession(dir, name string) (*SessionState, error)
func SaveSession(dir string, s *SessionState) error
func ClearSession(dir, name string) error
func ListSessions(dir string) ([]SessionState, error)
```

Storage: each session is `<dir>/<name>.json`. Default dir: `~/.local/share/devenv/sessions/`.

Behavior:
- `LoadSession` — missing file returns `nil, nil` (not found ≠ empty defaults)
- `SaveSession` — creates dir (0o700), writes indented JSON (0o600), uses `s.Session` as filename
- `ClearSession` — `os.Remove`, nil if already missing
- `ListSessions` — reads all `.json` files in dir, returns slice (empty dir → empty slice, nil error)

`SaveSession` does not validate status values; constants provide canonical values but validation is the caller's responsibility.

## Testing

| File | Coverage |
|---|---|
| `internal/config/project_test.go` | `RepoPath`: SSH scp-style, SSH URL, HTTPS, with/without `.git`, nested paths, empty, unparseable |
| `internal/config/config_test.go` (extend) | `ProjectConfig` loading (empty/single/multiple), `ProjectsDir` expansion, `GitIdentityFile` |
| `internal/config/notify_test.go` | `NotifyConfig` for each provider (Telegram, Slack, Discord, Webhook with headers/template) |
| `internal/state/session_test.go` | `LoadSession` (missing/existing), `SaveSession` (creates dir, roundtrip), `ClearSession` (existing/missing), `ListSessions` (empty/multiple) |

Deleted: `internal/remote/remote_test.go`

All tests use `t.TempDir()`. Verification: `make test`, `make lint`, `make coverage` (80% threshold).
