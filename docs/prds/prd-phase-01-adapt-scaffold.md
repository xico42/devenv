# PRD: Adapt Scaffold (Phase 1)

## Overview

Adapt the Phase 0 scaffold to reflect the local-execution architecture. This is a small, sequential change that unblocks all Phase 2 work.

## What changes

### Remove `internal/remote`

The `internal/remote` package (SSH command execution) is no longer needed. Workload commands (`project`, `worktree`, `session`) execute locally — they run `git`, `tmux`, and filesystem commands directly on the machine where devenv is invoked.

- Delete `internal/remote/remote.go`
- Delete `internal/remote/remote_test.go`
- Remove `golang.org/x/crypto` from `go.mod` (only used by `internal/remote`)
- Run `go mod tidy`

### Extend `internal/config`

Add three new sections to the config struct:

#### `projects_dir`

```go
type DefaultsConfig struct {
    // ... existing fields ...
    ProjectsDir      string `toml:"projects_dir"`      // default: "~/projects"
    GitIdentityFile  string `toml:"git_identity_file"`  // optional, for SCP to droplet
}
```

`ProjectsDir` is resolved at load time: `~` expands to `$HOME`. Default value: `~/projects`.

#### `[projects]` section

```go
type Config struct {
    Defaults  DefaultsConfig             `toml:"defaults"`
    Profiles  map[string]ProfileConfig   `toml:"profiles"`
    Projects  map[string]ProjectConfig   `toml:"projects"`   // NEW
    Notify    NotifyConfig               `toml:"notify"`      // NEW
}

type ProjectConfig struct {
    Repo          string `toml:"repo"`
    DefaultBranch string `toml:"default_branch"`
    EnvTemplate   string `toml:"env_template"`   // path to .env.template (~ expanded)
}
```

Example config:
```toml
[projects.myapp]
repo = "git@github.com:user/myapp.git"
default_branch = "main"

[projects.api]
repo = "git@github.com:user/api.git"
default_branch = "develop"

[projects.work-api]
repo = "git@github.com:corp/api.git"
default_branch = "develop"
env_template = "~/.config/devenv/templates/work-api.env.template"
```

#### `[notify]` section

```go
type NotifyConfig struct {
    Provider string                 `toml:"provider"`    // "telegram", "slack", "discord", "webhook"
    Telegram TelegramNotifyConfig   `toml:"telegram"`
    Slack    SlackNotifyConfig      `toml:"slack"`
    Discord  DiscordNotifyConfig    `toml:"discord"`
    Webhook  WebhookNotifyConfig    `toml:"webhook"`
}

type TelegramNotifyConfig struct {
    BotToken string `toml:"bot_token"`
    ChatID   string `toml:"chat_id"`
}

type SlackNotifyConfig struct {
    WebhookURL string `toml:"webhook_url"`
}

type DiscordNotifyConfig struct {
    WebhookURL string `toml:"webhook_url"`
}

type WebhookNotifyConfig struct {
    URL          string            `toml:"url"`
    Method       string            `toml:"method"`        // default: "POST"
    Headers      map[string]string `toml:"headers"`
    BodyTemplate string            `toml:"body_template"` // Go template, default: `{"text": "{{.Message}}"}`
}
```

### Extend `internal/state`

Add session state types for reading/writing `~/.local/share/devenv/sessions/<name>.json`:

```go
type SessionState struct {
    Session   string    `json:"session"`
    Project   string    `json:"project"`
    Branch    string    `json:"branch"`
    Status    string    `json:"status"`     // "running", "waiting"
    Question  string    `json:"question"`   // set when status == "waiting"
    UpdatedAt time.Time `json:"updated_at"`
    StartedAt time.Time `json:"started_at"`
}

func LoadSession(dir, name string) (*SessionState, error)
func SaveSession(dir string, s *SessionState) error
func ClearSession(dir, name string) error
func ListSessions(dir string) ([]SessionState, error)
```

The `dir` parameter defaults to `~/.local/share/devenv/sessions/` but is injectable for testing.

### Add `RepoPath` helper

Add a function to derive filesystem paths from git remote URLs:

```go
// RepoPath parses a git remote URL and returns the directory path.
// Examples:
//   "git@github.com:user/myapp.git"       -> "github.com/user/myapp"
//   "git@gitlab.com:corp/group/api.git"    -> "gitlab.com/corp/group/api"
//   "https://github.com/user/myapp.git"    -> "github.com/user/myapp"
//   "ssh://git@github.com/user/myapp.git"  -> "github.com/user/myapp"
func RepoPath(repoURL string) (string, error)
```

Rules: strip `.git` suffix, strip leading `/` from path, preserve all path components. Error on unparseable URLs.

This function lives in `internal/config` and is used by `devenv project clone` and `devenv worktree` to derive filesystem paths from repo URLs.

### Update tests

- Add tests for `ProjectConfig` loading (empty, single, multiple projects)
- Add tests for `NotifyConfig` loading (each provider type)
- Add tests for `ProjectsDir` expansion (`~` -> `$HOME`)
- Add tests for `RepoPath()` (SSH scp-style, SSH URL-style, HTTPS, with/without `.git`, nested paths)
- Add tests for `SessionState` CRUD operations
- Remove `internal/remote` tests
- Verify `make test`, `make lint`, `make coverage` all pass

## Acceptance Criteria

- `internal/remote/` directory does not exist
- `golang.org/x/crypto` is not in `go.mod`
- `config.Load()` correctly parses `[projects]`, `[notify]`, and `projects_dir`
- `config.RepoPath()` correctly parses all supported git URL formats
- `state.LoadSession/SaveSession/ClearSession/ListSessions` work with `t.TempDir()`
- `make test` passes
- `make lint` passes
- `make coverage` meets 80% threshold
