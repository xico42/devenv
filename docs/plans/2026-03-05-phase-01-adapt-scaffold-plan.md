# Phase 1: Adapt Scaffold — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Adapt the Phase 0 scaffold to the local-execution architecture by removing `internal/remote`, extending config with projects/notify/projects_dir, adding `RepoPath()`, and adding session state persistence.

**Architecture:** Sequential refactor — delete unused SSH package, extend existing config and state packages with new sub-files (Approach B: one file per concern). All changes are additive to existing packages except the `internal/remote` deletion.

**Tech Stack:** Go 1.26, TOML (BurntSushi/toml), JSON (encoding/json), standard library only (net/url for URL parsing)

**Worktree:** `~/.config/superpowers/worktrees/remote-dev/phase-01-adapt-scaffold`

---

### Task 1: Delete `internal/remote` and remove `golang.org/x/crypto`

**Files:**
- Delete: `internal/remote/remote.go`
- Delete: `internal/remote/remote_test.go`
- Modify: `go.mod` (via `go mod tidy`)

**Step 1: Delete the remote package**

```bash
rm -rf internal/remote
```

**Step 2: Run `go mod tidy` to remove unused dependency**

```bash
go mod tidy
```

**Step 3: Verify `golang.org/x/crypto` is gone from `go.mod`**

```bash
grep "golang.org/x/crypto" go.mod
```

Expected: no output (exit code 1)

**Step 4: Verify tests still pass**

```bash
go test ./...
```

Expected: all packages pass, `internal/remote` no longer listed

**Step 5: Commit**

```bash
git add -A && git commit -m "refactor: remove internal/remote and golang.org/x/crypto

Workload commands execute locally — SSH remote execution is no longer needed."
```

---

### Task 2: Add `ProjectConfig` struct and `RepoPath()` function

**Files:**
- Create: `internal/config/project.go`
- Test: `internal/config/project_test.go`

**Step 1: Write the failing tests for `RepoPath`**

Create `internal/config/project_test.go`:

```go
package config_test

import (
	"testing"

	"github.com/xico42/devenv/internal/config"
)

func TestRepoPath(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "scp-style SSH",
			url:  "git@github.com:user/myapp.git",
			want: "github.com/user/myapp",
		},
		{
			name: "scp-style SSH without .git",
			url:  "git@github.com:user/myapp",
			want: "github.com/user/myapp",
		},
		{
			name: "scp-style SSH nested path",
			url:  "git@gitlab.com:corp/group/api.git",
			want: "gitlab.com/corp/group/api",
		},
		{
			name: "SSH URL-style",
			url:  "ssh://git@github.com/user/myapp.git",
			want: "github.com/user/myapp",
		},
		{
			name: "SSH URL-style without .git",
			url:  "ssh://git@github.com/user/myapp",
			want: "github.com/user/myapp",
		},
		{
			name: "HTTPS",
			url:  "https://github.com/user/myapp.git",
			want: "github.com/user/myapp",
		},
		{
			name: "HTTPS without .git",
			url:  "https://github.com/user/myapp",
			want: "github.com/user/myapp",
		},
		{
			name: "HTTPS nested path",
			url:  "https://gitlab.com/corp/group/api.git",
			want: "gitlab.com/corp/group/api",
		},
		{
			name:    "empty string",
			url:     "",
			wantErr: true,
		},
		{
			name:    "unparseable",
			url:     "not-a-url",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.RepoPath(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RepoPath(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("RepoPath(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/...
```

Expected: FAIL — `config.RepoPath` undefined

**Step 3: Write the implementation**

Create `internal/config/project.go`:

```go
package config

import (
	"fmt"
	"net/url"
	"strings"
)

// ProjectConfig holds per-project settings.
type ProjectConfig struct {
	Repo          string `toml:"repo"`
	DefaultBranch string `toml:"default_branch"`
	EnvTemplate   string `toml:"env_template"`
}

// RepoPath parses a git remote URL and returns the directory path.
// Examples:
//
//	"git@github.com:user/myapp.git"       -> "github.com/user/myapp"
//	"git@gitlab.com:corp/group/api.git"   -> "gitlab.com/corp/group/api"
//	"https://github.com/user/myapp.git"   -> "github.com/user/myapp"
//	"ssh://git@github.com/user/myapp.git" -> "github.com/user/myapp"
func RepoPath(repoURL string) (string, error) {
	if repoURL == "" {
		return "", fmt.Errorf("empty repo URL")
	}

	var host, path string

	// SCP-style: git@host:path (no scheme, has colon but not ://)
	if strings.Contains(repoURL, ":") && !strings.Contains(repoURL, "://") {
		// Split on first ":"
		at := strings.Index(repoURL, ":")
		hostPart := repoURL[:at]
		path = repoURL[at+1:]

		// Extract host from user@host
		if idx := strings.Index(hostPart, "@"); idx >= 0 {
			host = hostPart[idx+1:]
		} else {
			host = hostPart
		}
	} else {
		u, err := url.Parse(repoURL)
		if err != nil {
			return "", fmt.Errorf("parsing repo URL %q: %w", repoURL, err)
		}
		if u.Host == "" {
			return "", fmt.Errorf("repo URL %q has no host", repoURL)
		}
		host = u.Hostname()
		path = u.Path
	}

	// Clean up path
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, ".git")

	if path == "" {
		return "", fmt.Errorf("repo URL %q has no path", repoURL)
	}

	return host + "/" + path, nil
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/config/...
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/project.go internal/config/project_test.go
git commit -m "feat: add ProjectConfig and RepoPath() git URL parser"
```

---

### Task 3: Add `NotifyConfig` types

**Files:**
- Create: `internal/config/notify.go`
- Test: `internal/config/notify_test.go`

**Step 1: Write the failing tests**

Create `internal/config/notify_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xico42/devenv/internal/config"
)

func TestLoad_NotifyTelegram(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notify]
provider = "telegram"

[notify.telegram]
bot_token = "123:ABC"
chat_id = "456"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Notify.Provider != "telegram" {
		t.Errorf("Provider = %q, want %q", cfg.Notify.Provider, "telegram")
	}
	if cfg.Notify.Telegram.BotToken != "123:ABC" {
		t.Errorf("BotToken = %q, want %q", cfg.Notify.Telegram.BotToken, "123:ABC")
	}
	if cfg.Notify.Telegram.ChatID != "456" {
		t.Errorf("ChatID = %q, want %q", cfg.Notify.Telegram.ChatID, "456")
	}
}

func TestLoad_NotifySlack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notify]
provider = "slack"

[notify.slack]
webhook_url = "https://hooks.slack.com/services/T/B/X"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Notify.Provider != "slack" {
		t.Errorf("Provider = %q, want %q", cfg.Notify.Provider, "slack")
	}
	if cfg.Notify.Slack.WebhookURL != "https://hooks.slack.com/services/T/B/X" {
		t.Errorf("WebhookURL = %q, want expected", cfg.Notify.Slack.WebhookURL)
	}
}

func TestLoad_NotifyDiscord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notify]
provider = "discord"

[notify.discord]
webhook_url = "https://discord.com/api/webhooks/123/abc"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Notify.Provider != "discord" {
		t.Errorf("Provider = %q, want %q", cfg.Notify.Provider, "discord")
	}
	if cfg.Notify.Discord.WebhookURL != "https://discord.com/api/webhooks/123/abc" {
		t.Errorf("WebhookURL = %q, want expected", cfg.Notify.Discord.WebhookURL)
	}
}

func TestLoad_NotifyWebhook(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[notify]
provider = "webhook"

[notify.webhook]
url = "https://example.com/hook"
method = "PUT"
body_template = "{\"msg\": \"{{.Message}}\"}"

[notify.webhook.headers]
Authorization = "Bearer token123"
Content-Type = "application/json"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Notify.Provider != "webhook" {
		t.Errorf("Provider = %q, want %q", cfg.Notify.Provider, "webhook")
	}
	if cfg.Notify.Webhook.URL != "https://example.com/hook" {
		t.Errorf("URL = %q, want expected", cfg.Notify.Webhook.URL)
	}
	if cfg.Notify.Webhook.Method != "PUT" {
		t.Errorf("Method = %q, want %q", cfg.Notify.Webhook.Method, "PUT")
	}
	if cfg.Notify.Webhook.BodyTemplate != `{"msg": "{{.Message}}"}` {
		t.Errorf("BodyTemplate = %q, want expected", cfg.Notify.Webhook.BodyTemplate)
	}
	if cfg.Notify.Webhook.Headers["Authorization"] != "Bearer token123" {
		t.Errorf("Authorization header = %q, want expected", cfg.Notify.Webhook.Headers["Authorization"])
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/...
```

Expected: FAIL — `cfg.Notify` undefined (field doesn't exist on `Config` yet)

**Step 3: Write the implementation**

Create `internal/config/notify.go`:

```go
package config

// NotifyConfig holds notification provider settings.
type NotifyConfig struct {
	Provider string                `toml:"provider"`
	Telegram TelegramNotifyConfig `toml:"telegram"`
	Slack    SlackNotifyConfig    `toml:"slack"`
	Discord  DiscordNotifyConfig  `toml:"discord"`
	Webhook  WebhookNotifyConfig  `toml:"webhook"`
}

// TelegramNotifyConfig holds Telegram bot settings.
type TelegramNotifyConfig struct {
	BotToken string `toml:"bot_token"`
	ChatID   string `toml:"chat_id"`
}

// SlackNotifyConfig holds Slack webhook settings.
type SlackNotifyConfig struct {
	WebhookURL string `toml:"webhook_url"`
}

// DiscordNotifyConfig holds Discord webhook settings.
type DiscordNotifyConfig struct {
	WebhookURL string `toml:"webhook_url"`
}

// WebhookNotifyConfig holds generic webhook settings.
type WebhookNotifyConfig struct {
	URL          string            `toml:"url"`
	Method       string            `toml:"method"`
	Headers      map[string]string `toml:"headers"`
	BodyTemplate string            `toml:"body_template"`
}
```

Note: the tests will still fail after this step because `Config.Notify` doesn't exist yet. That's added in Task 4.

**Step 4: Commit the notify types (tests will pass after Task 4)**

```bash
git add internal/config/notify.go internal/config/notify_test.go
git commit -m "feat: add notification config types

Tests depend on Config struct changes in the next commit."
```

---

### Task 4: Extend `Config` and `DefaultsConfig` structs

**Files:**
- Modify: `internal/config/config.go:16-31` (Config and DefaultsConfig structs)
- Modify: `internal/config/config.go:42-67` (Load function — add tilde expansion)
- Extend: `internal/config/config_test.go` (add new test cases)

**Step 1: Write the failing tests for new config fields**

Add to `internal/config/config_test.go`:

```go
func TestLoad_Projects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[projects.myapp]
repo = "git@github.com:user/myapp.git"
default_branch = "main"

[projects.api]
repo = "git@github.com:user/api.git"
default_branch = "develop"
env_template = "~/.config/devenv/templates/api.env.template"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Projects) != 2 {
		t.Fatalf("len(Projects) = %d, want 2", len(cfg.Projects))
	}
	myapp := cfg.Projects["myapp"]
	if myapp.Repo != "git@github.com:user/myapp.git" {
		t.Errorf("myapp.Repo = %q, want expected", myapp.Repo)
	}
	if myapp.DefaultBranch != "main" {
		t.Errorf("myapp.DefaultBranch = %q, want %q", myapp.DefaultBranch, "main")
	}
	api := cfg.Projects["api"]
	if api.DefaultBranch != "develop" {
		t.Errorf("api.DefaultBranch = %q, want %q", api.DefaultBranch, "develop")
	}
	// EnvTemplate should have ~ expanded
	home, _ := os.UserHomeDir()
	wantTpl := home + "/.config/devenv/templates/api.env.template"
	if api.EnvTemplate != wantTpl {
		t.Errorf("api.EnvTemplate = %q, want %q", api.EnvTemplate, wantTpl)
	}
}

func TestLoad_ProjectsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Projects != nil {
		t.Errorf("Projects = %v, want nil for missing file", cfg.Projects)
	}
}

func TestLoad_ProjectsDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[defaults]
projects_dir = "~/projects"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	home, _ := os.UserHomeDir()
	want := home + "/projects"
	if cfg.Defaults.ProjectsDir != want {
		t.Errorf("ProjectsDir = %q, want %q", cfg.Defaults.ProjectsDir, want)
	}
}

func TestLoad_ProjectsDirDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	home, _ := os.UserHomeDir()
	want := home + "/projects"
	if cfg.Defaults.ProjectsDir != want {
		t.Errorf("ProjectsDir = %q, want %q (default)", cfg.Defaults.ProjectsDir, want)
	}
}

func TestLoad_GitIdentityFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[defaults]
git_identity_file = "~/.ssh/id_ed25519"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	home, _ := os.UserHomeDir()
	want := home + "/.ssh/id_ed25519"
	if cfg.Defaults.GitIdentityFile != want {
		t.Errorf("GitIdentityFile = %q, want %q", cfg.Defaults.GitIdentityFile, want)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/...
```

Expected: FAIL — `cfg.Projects` and `cfg.Notify` undefined

**Step 3: Update the Config and DefaultsConfig structs**

Modify `internal/config/config.go`. The `Config` struct (lines 16-21) becomes:

```go
// Config holds all devenv configuration.
type Config struct {
	Defaults DefaultsConfig             `toml:"defaults"`
	Profiles map[string]ProfileConfig   `toml:"profiles"`
	Projects map[string]ProjectConfig   `toml:"projects"`
	Notify   NotifyConfig               `toml:"notify"`

	path string // runtime only, not serialized
}
```

The `DefaultsConfig` struct (lines 24-31) becomes:

```go
// DefaultsConfig holds default values applied to every droplet.
type DefaultsConfig struct {
	Token            string `toml:"token"`
	SSHKeyID         string `toml:"ssh_key_id"`
	Region           string `toml:"region"`
	Size             string `toml:"size"`
	TailscaleAuthKey string `toml:"tailscale_auth_key"`
	Image            string `toml:"image"`
	ProjectsDir      string `toml:"projects_dir"`
	GitIdentityFile  string `toml:"git_identity_file"`
}
```

**Step 4: Add tilde expansion helper and update `Load()`**

Add this helper function to `internal/config/config.go` (before `Load`):

```go
// expandTilde replaces a leading "~/" with the user's home directory.
func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expanding ~: %w", err)
	}
	return home + path[1:], nil
}
```

Add `"strings"` to the imports in `config.go`.

Update `Load()` — after the TOML unmarshal and default image logic, add tilde expansion:

```go
func Load(path string) (*Config, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home dir: %w", err)
		}
		path = filepath.Join(home, ".config", "devenv", "config.toml")
	}

	cfg := &Config{path: path}
	cfg.Defaults.Image = defaultImage

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := cfg.expandPaths(); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	if cfg.Defaults.Image == "" {
		cfg.Defaults.Image = defaultImage
	}
	if err := cfg.expandPaths(); err != nil {
		return nil, err
	}
	return cfg, nil
}
```

Add the `expandPaths` method:

```go
const defaultProjectsDir = "~/projects"

// expandPaths resolves ~ in all path fields and applies defaults.
func (c *Config) expandPaths() error {
	if c.Defaults.ProjectsDir == "" {
		c.Defaults.ProjectsDir = defaultProjectsDir
	}
	var err error
	if c.Defaults.ProjectsDir, err = expandTilde(c.Defaults.ProjectsDir); err != nil {
		return err
	}
	if c.Defaults.GitIdentityFile, err = expandTilde(c.Defaults.GitIdentityFile); err != nil {
		return err
	}
	for name, p := range c.Projects {
		if p.EnvTemplate, err = expandTilde(p.EnvTemplate); err != nil {
			return err
		}
		c.Projects[name] = p
	}
	return nil
}
```

**Step 5: Run all tests**

```bash
go test ./internal/config/...
```

Expected: PASS (all existing + new tests)

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: extend Config with Projects, Notify, ProjectsDir, GitIdentityFile

Adds tilde expansion for ProjectsDir, GitIdentityFile, and EnvTemplate paths.
ProjectsDir defaults to ~/projects."
```

---

### Task 5: Add session state CRUD

**Files:**
- Create: `internal/state/session.go`
- Test: `internal/state/session_test.go`

**Step 1: Write the failing tests**

Create `internal/state/session_test.go`:

```go
package state_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/xico42/devenv/internal/state"
)

func TestLoadSession_Missing(t *testing.T) {
	dir := t.TempDir()
	s, err := state.LoadSession(dir, "nonexistent")
	if err != nil {
		t.Fatalf("LoadSession() error = %v, want nil", err)
	}
	if s != nil {
		t.Errorf("LoadSession() = %v, want nil for missing session", s)
	}
}

func TestSaveAndLoadSession(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	s := &state.SessionState{
		Session:   "my-session",
		Project:   "myapp",
		Branch:    "feature/auth",
		Status:    state.SessionRunning,
		UpdatedAt: now,
		StartedAt: now,
	}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	got, err := state.LoadSession(dir, "my-session")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got == nil {
		t.Fatal("LoadSession() returned nil")
	}
	if got.Session != s.Session {
		t.Errorf("Session = %q, want %q", got.Session, s.Session)
	}
	if got.Project != s.Project {
		t.Errorf("Project = %q, want %q", got.Project, s.Project)
	}
	if got.Branch != s.Branch {
		t.Errorf("Branch = %q, want %q", got.Branch, s.Branch)
	}
	if got.Status != state.SessionRunning {
		t.Errorf("Status = %q, want %q", got.Status, state.SessionRunning)
	}
	if !got.StartedAt.Equal(s.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, s.StartedAt)
	}
}

func TestSaveSession_WithQuestion(t *testing.T) {
	dir := t.TempDir()
	s := &state.SessionState{
		Session:   "waiting-session",
		Project:   "myapp",
		Branch:    "main",
		Status:    state.SessionWaiting,
		Question:  "Should I refactor the auth module?",
		UpdatedAt: time.Now().UTC().Truncate(time.Second),
		StartedAt: time.Now().UTC().Truncate(time.Second),
	}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	got, err := state.LoadSession(dir, "waiting-session")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got.Status != state.SessionWaiting {
		t.Errorf("Status = %q, want %q", got.Status, state.SessionWaiting)
	}
	if got.Question != s.Question {
		t.Errorf("Question = %q, want %q", got.Question, s.Question)
	}
}

func TestSaveSession_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "sessions")
	s := &state.SessionState{
		Session: "test",
		Status:  state.SessionRunning,
	}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatalf("SaveSession() should create dir, got error = %v", err)
	}
	got, err := state.LoadSession(dir, "test")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got == nil {
		t.Fatal("LoadSession() returned nil after save to nested dir")
	}
}

func TestClearSession(t *testing.T) {
	dir := t.TempDir()

	// Clear on missing session must not error
	if err := state.ClearSession(dir, "nonexistent"); err != nil {
		t.Fatalf("ClearSession() on missing error = %v", err)
	}

	// Save then clear
	s := &state.SessionState{Session: "to-clear", Status: state.SessionRunning}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatalf("SaveSession() error = %v", err)
	}
	if err := state.ClearSession(dir, "to-clear"); err != nil {
		t.Fatalf("ClearSession() error = %v", err)
	}
	got, err := state.LoadSession(dir, "to-clear")
	if err != nil {
		t.Fatalf("LoadSession() after clear error = %v", err)
	}
	if got != nil {
		t.Errorf("LoadSession() after clear = %v, want nil", got)
	}
}

func TestListSessions_Empty(t *testing.T) {
	dir := t.TempDir()
	sessions, err := state.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0", len(sessions))
	}
}

func TestListSessions_Multiple(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().UTC().Truncate(time.Second)
	for _, name := range []string{"alpha", "beta"} {
		s := &state.SessionState{
			Session:   name,
			Project:   "proj",
			Status:    state.SessionRunning,
			StartedAt: now,
			UpdatedAt: now,
		}
		if err := state.SaveSession(dir, s); err != nil {
			t.Fatalf("SaveSession(%q) error = %v", name, err)
		}
	}
	sessions, err := state.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len(sessions) = %d, want 2", len(sessions))
	}
	names := map[string]bool{}
	for _, s := range sessions {
		names[s.Session] = true
	}
	if !names["alpha"] || !names["beta"] {
		t.Errorf("sessions = %v, want alpha and beta", names)
	}
}

func TestListSessions_MissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	sessions, err := state.ListSessions(dir)
	if err != nil {
		t.Fatalf("ListSessions() on missing dir error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0", len(sessions))
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/state/...
```

Expected: FAIL — `state.SessionState` undefined

**Step 3: Write the implementation**

Create `internal/state/session.go`:

```go
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Session status values.
const (
	SessionRunning = "running"
	SessionWaiting = "waiting"
)

// SessionState holds runtime information about a Claude Code session.
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

// LoadSession reads a session state file from dir. Returns nil, nil if
// the session does not exist.
func LoadSession(dir, name string) (*SessionState, error) {
	path := filepath.Join(dir, name+".json")
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading session %q: %w", name, err)
	}
	var s SessionState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing session %q: %w", name, err)
	}
	return &s, nil
}

// SaveSession writes a session state file to dir, creating the directory
// if needed. The filename is derived from s.Session.
func SaveSession(dir string, s *SessionState) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating sessions dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding session: %w", err)
	}
	path := filepath.Join(dir, s.Session+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing session %q: %w", s.Session, err)
	}
	return nil
}

// ClearSession removes a session state file. Returns nil if the file
// does not exist.
func ClearSession(dir, name string) error {
	path := filepath.Join(dir, name+".json")
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing session %q: %w", name, err)
	}
	return nil
}

// ListSessions reads all session state files from dir. Returns an empty
// slice if the directory does not exist or is empty.
func ListSessions(dir string) ([]SessionState, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}
	var sessions []SessionState
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		s, err := LoadSession(dir, name)
		if err != nil {
			return nil, err
		}
		if s != nil {
			sessions = append(sessions, *s)
		}
	}
	return sessions, nil
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/state/...
```

Expected: PASS

**Step 5: Commit**

```bash
git add internal/state/session.go internal/state/session_test.go
git commit -m "feat: add session state CRUD (LoadSession, SaveSession, ClearSession, ListSessions)"
```

---

### Task 6: Final verification

**Files:** None (verification only)

**Step 1: Run full test suite**

```bash
go test ./...
```

Expected: all packages pass

**Step 2: Run linter**

```bash
make lint
```

Expected: no issues

**Step 3: Run coverage check**

```bash
make coverage
```

Expected: coverage >= 80%, `OK` message

**Step 4: Verify `internal/remote` is gone**

```bash
ls internal/remote 2>&1
```

Expected: `ls: cannot access 'internal/remote': No such file or directory`

**Step 5: Verify `golang.org/x/crypto` is gone**

```bash
grep "golang.org/x/crypto" go.mod
```

Expected: no output

**Step 6: Build the binary**

```bash
make build
```

Expected: `devenv` binary built successfully
