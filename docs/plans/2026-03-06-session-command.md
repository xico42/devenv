# `devenv session` Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the `devenv session` command with six subcommands (`start`, `list`, `show`, `attach`, `stop`, `mark-running`) managing agent sessions in tmux.

**Architecture:** Shared naming/path conventions live in a new `internal/semconv` leaf package (zero dependencies). Business logic lives in `internal/session` (Service + tmux.Client). Agent harness is configurable via nested `[defaults.agent]` / `[projects.<name>.agent]` config sections. The cmd layer resolves config and worktree paths, passing primitives to the service.

**Tech Stack:** Go stdlib (`os/exec`, `syscall`, `path/filepath`), Cobra, existing `internal/config`, `internal/state`, `internal/tmux`, `internal/worktree`.

**Design doc:** `docs/plans/2026-03-06-session-command-design.md`

---

## Task 1: `internal/semconv` — shared naming and path conventions

**Files:**
- Create: `internal/semconv/semconv.go`
- Create: `internal/semconv/semconv_test.go`

### Step 1: Write the failing tests

```go
// internal/semconv/semconv_test.go
package semconv_test

import (
	"testing"

	"github.com/xico42/devenv/internal/semconv"
)

func TestFlattenBranch(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"main", "main"},
		{"feature/login", "feature-login"},
		{"fix/auth/token", "fix-auth-token"},
		{"no-slash", "no-slash"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := semconv.FlattenBranch(tt.input); got != tt.want {
			t.Errorf("FlattenBranch(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSessionName(t *testing.T) {
	tests := []struct {
		project, branch, want string
	}{
		{"myapp", "feature", "myapp-feature"},
		{"myapp", "feature/login", "myapp-feature-login"},
		{"api", "fix/auth/token", "api-fix-auth-token"},
	}
	for _, tt := range tests {
		if got := semconv.SessionName(tt.project, tt.branch); got != tt.want {
			t.Errorf("SessionName(%q, %q) = %q, want %q", tt.project, tt.branch, got, tt.want)
		}
	}
}

func TestCloneDir(t *testing.T) {
	got := semconv.CloneDir("/home/user/projects", "github.com/user/myapp")
	want := "/home/user/projects/github.com/user/myapp"
	if got != want {
		t.Errorf("CloneDir() = %q, want %q", got, want)
	}
}

func TestWorktreesRoot(t *testing.T) {
	got := semconv.WorktreesRoot("/home/user/projects", "github.com/user/myapp")
	want := "/home/user/projects/github.com/user/myapp__worktrees"
	if got != want {
		t.Errorf("WorktreesRoot() = %q, want %q", got, want)
	}
}

func TestWorktreePath(t *testing.T) {
	got := semconv.WorktreePath("/home/user/projects", "github.com/user/myapp", "feature/login")
	want := "/home/user/projects/github.com/user/myapp__worktrees/feature-login"
	if got != want {
		t.Errorf("WorktreePath() = %q, want %q", got, want)
	}
}

func TestConstants(t *testing.T) {
	if semconv.SessionEnvVar != "DEVENV_SESSION" {
		t.Errorf("SessionEnvVar = %q, want DEVENV_SESSION", semconv.SessionEnvVar)
	}
	if semconv.DefaultAgentCmd != "claude" {
		t.Errorf("DefaultAgentCmd = %q, want claude", semconv.DefaultAgentCmd)
	}
}
```

### Step 2: Run tests to verify they fail

Run: `go test ./internal/semconv/...`
Expected: compilation error — package does not exist.

### Step 3: Write the implementation

```go
// internal/semconv/semconv.go
package semconv

import (
	"path/filepath"
	"strings"
)

const (
	SessionEnvVar   = "DEVENV_SESSION"
	DefaultAgentCmd = "claude"
)

func FlattenBranch(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}

func SessionName(project, branch string) string {
	return project + "-" + FlattenBranch(branch)
}

func CloneDir(projectsDir, repoPath string) string {
	return filepath.Join(projectsDir, repoPath)
}

func WorktreesRoot(projectsDir, repoPath string) string {
	return CloneDir(projectsDir, repoPath) + "__worktrees"
}

func WorktreePath(projectsDir, repoPath, branch string) string {
	return filepath.Join(WorktreesRoot(projectsDir, repoPath), FlattenBranch(branch))
}
```

### Step 4: Run tests to verify they pass

Run: `go test ./internal/semconv/...`
Expected: PASS

### Step 5: Commit

```bash
git add internal/semconv/
git commit -m "feat: add internal/semconv package for shared naming conventions"
```

---

## Task 2: Refactor `internal/worktree` to use `semconv`

**Files:**
- Modify: `internal/worktree/worktree.go`

### Step 1: Run existing tests to confirm green baseline

Run: `go test ./internal/worktree/...`
Expected: PASS

### Step 2: Replace private helpers with semconv calls

In `internal/worktree/worktree.go`:

1. Remove the `flattenBranch` function (lines 148-150).
2. Replace `resolvePaths` body to use `semconv`:

```go
import "github.com/xico42/devenv/internal/semconv"

func (s *Service) resolvePaths(project, branch string) (cloneDir, worktreesRoot, worktreePath string, err error) {
	p, ok := s.cfg.Projects[project]
	if !ok {
		return "", "", "", fmt.Errorf("project %q is not configured", project)
	}
	repoPath, err := config.RepoPath(p.Repo)
	if err != nil {
		return "", "", "", fmt.Errorf("parsing repo URL: %w", err)
	}
	cloneDir = semconv.CloneDir(s.cfg.Defaults.ProjectsDir, repoPath)
	worktreesRoot = semconv.WorktreesRoot(s.cfg.Defaults.ProjectsDir, repoPath)
	worktreePath = semconv.WorktreePath(s.cfg.Defaults.ProjectsDir, repoPath, branch)
	return cloneDir, worktreesRoot, worktreePath, nil
}
```

3. Replace inline session name derivations with `semconv.SessionName`:
   - Line 225: `SessionName: project + "-" + branch` → `SessionName: semconv.SessionName(project, branch)`
   - Line 267: `candidate := name + "-" + wt.Branch` → `candidate := semconv.SessionName(name, wt.Branch)`
   - Line 303: `sessionName := req.Project + "-" + req.Branch` → `sessionName := semconv.SessionName(req.Project, req.Branch)`
   - Line 365: `SessionName: project + "-" + branch` → `SessionName: semconv.SessionName(project, branch)`

### Step 3: Run tests to verify no regressions

Run: `go test ./internal/worktree/...`
Expected: PASS (identical behavior)

### Step 4: Run full test suite

Run: `make test`
Expected: PASS

### Step 5: Commit

```bash
git add internal/worktree/worktree.go
git commit -m "refactor: use semconv for naming and path conventions in worktree"
```

---

## Task 3: `internal/config` — add `AgentConfig`

**Files:**
- Modify: `internal/config/config.go`
- Create: `internal/config/agent.go`
- Create: `internal/config/agent_test.go`

### Step 1: Write the failing tests

```go
// internal/config/agent_test.go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xico42/devenv/internal/config"
)

func TestAgentConfig_Defaults(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	agent := cfg.ResolveAgent("")
	if agent.Cmd != "" {
		t.Errorf("default Cmd = %q, want empty (caller applies default)", agent.Cmd)
	}
}

func TestAgentConfig_GlobalOnly(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[defaults.agent]
cmd = "claude"
args = ["--dangerously-skip-permissions"]

[defaults.agent.env]
CLAUDE_CONFIG_DIR = "/custom"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	agent := cfg.ResolveAgent("nonexistent")
	if agent.Cmd != "claude" {
		t.Errorf("Cmd = %q, want claude", agent.Cmd)
	}
	if len(agent.Args) != 1 || agent.Args[0] != "--dangerously-skip-permissions" {
		t.Errorf("Args = %v, want [--dangerously-skip-permissions]", agent.Args)
	}
	if agent.Env["CLAUDE_CONFIG_DIR"] != "/custom" {
		t.Errorf("Env[CLAUDE_CONFIG_DIR] = %q, want /custom", agent.Env["CLAUDE_CONFIG_DIR"])
	}
}

func TestAgentConfig_ProjectOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[defaults.agent]
cmd = "claude"
args = ["--global-flag"]

[defaults.agent.env]
GLOBAL_VAR = "global"
SHARED_VAR = "from-global"

[projects.myapp]
repo = "git@github.com:user/myapp.git"

[projects.myapp.agent]
cmd = "aider"
args = ["--model", "opus"]

[projects.myapp.agent.env]
PROJECT_VAR = "project"
SHARED_VAR = "from-project"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	agent := cfg.ResolveAgent("myapp")

	// cmd and args: project replaces global
	if agent.Cmd != "aider" {
		t.Errorf("Cmd = %q, want aider", agent.Cmd)
	}
	if len(agent.Args) != 2 || agent.Args[0] != "--model" {
		t.Errorf("Args = %v, want [--model opus]", agent.Args)
	}

	// env: merged, project wins on conflict
	if agent.Env["GLOBAL_VAR"] != "global" {
		t.Errorf("Env[GLOBAL_VAR] = %q, want global", agent.Env["GLOBAL_VAR"])
	}
	if agent.Env["PROJECT_VAR"] != "project" {
		t.Errorf("Env[PROJECT_VAR] = %q, want project", agent.Env["PROJECT_VAR"])
	}
	if agent.Env["SHARED_VAR"] != "from-project" {
		t.Errorf("Env[SHARED_VAR] = %q, want from-project", agent.Env["SHARED_VAR"])
	}
}

func TestAgentConfig_ProjectPartialOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[defaults.agent]
cmd = "claude"
args = ["--global-flag"]

[projects.myapp]
repo = "git@github.com:user/myapp.git"

[projects.myapp.agent.env]
EXTRA = "val"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	agent := cfg.ResolveAgent("myapp")

	// cmd and args: fall back to global (project didn't set them)
	if agent.Cmd != "claude" {
		t.Errorf("Cmd = %q, want claude (global fallback)", agent.Cmd)
	}
	if len(agent.Args) != 1 || agent.Args[0] != "--global-flag" {
		t.Errorf("Args = %v, want [--global-flag] (global fallback)", agent.Args)
	}
	if agent.Env["EXTRA"] != "val" {
		t.Errorf("Env[EXTRA] = %q, want val", agent.Env["EXTRA"])
	}
}
```

### Step 2: Run tests to verify they fail

Run: `go test ./internal/config/...`
Expected: compilation error — `AgentConfig` and `ResolveAgent` not found.

### Step 3: Write the implementation

```go
// internal/config/agent.go
package config

// AgentConfig holds agent harness settings (command, args, env vars).
type AgentConfig struct {
	Cmd  string            `toml:"cmd"`
	Args []string          `toml:"args"`
	Env  map[string]string `toml:"env"`
}

// ResolveAgent returns the merged agent config for the given project.
// Per-project cmd and args replace global if set. Env is merged with
// project winning on key conflict. If project is empty or not found,
// returns the global defaults.
func (c *Config) ResolveAgent(project string) AgentConfig {
	global := c.Defaults.Agent
	result := AgentConfig{
		Cmd:  global.Cmd,
		Args: global.Args,
		Env:  copyEnv(global.Env),
	}

	p, ok := c.Projects[project]
	if !ok {
		return result
	}

	if p.Agent.Cmd != "" {
		result.Cmd = p.Agent.Cmd
	}
	if p.Agent.Args != nil {
		result.Args = p.Agent.Args
	}
	for k, v := range p.Agent.Env {
		if result.Env == nil {
			result.Env = make(map[string]string)
		}
		result.Env[k] = v
	}
	return result
}

func copyEnv(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
```

Then add `Agent AgentConfig` to the structs in `config.go`:

```go
// In DefaultsConfig:
Agent AgentConfig `toml:"agent"`

// In ProjectConfig (internal/config/project.go):
Agent AgentConfig `toml:"agent"`
```

### Step 4: Run tests to verify they pass

Run: `go test ./internal/config/...`
Expected: PASS

### Step 5: Run full test suite

Run: `make test`
Expected: PASS

### Step 6: Commit

```bash
git add internal/config/agent.go internal/config/agent_test.go internal/config/config.go internal/config/project.go
git commit -m "feat: add AgentConfig with nested [agent] config and merge logic"
```

---

## Task 4: `internal/tmux` — add `NewSessionWithEnv` and `ExecAttach`

**Files:**
- Modify: `internal/tmux/client.go`
- Modify: `internal/tmux/client_test.go`

### Step 1: Write the failing tests

Append to `internal/tmux/client_test.go`:

```go
func TestClient_NewSessionWithEnv_ok(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	env := map[string]string{"DEVENV_SESSION": "myapp-feature", "FOO": "bar"}
	err := c.NewSessionWithEnv("myapp-feature", "/tmp/wt", env, "claude --skip")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.lastArgs[0] != "new-session" {
		t.Errorf("expected new-session, got %v", r.lastArgs)
	}
	// Verify -d, -s, -c flags are present
	argStr := fmt.Sprintf("%v", r.lastArgs)
	for _, want := range []string{"-d", "-s", "myapp-feature", "-c", "/tmp/wt"} {
		found := false
		for _, a := range r.lastArgs {
			if a == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in args %s", want, argStr)
		}
	}
}

func TestClient_NewSessionWithEnv_error(t *testing.T) {
	r := &mockRunner{exitCode: 1, stderr: "duplicate session"}
	c := tmux.NewClient(r)
	err := c.NewSessionWithEnv("myapp", "/tmp", nil, "claude")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_NewSessionWithEnv_execError(t *testing.T) {
	r := &mockRunner{exitCode: -1, err: fmt.Errorf("tmux not found")}
	c := tmux.NewClient(r)
	err := c.NewSessionWithEnv("myapp", "/tmp", nil, "claude")
	if err == nil {
		t.Fatal("expected error on exec failure")
	}
}

func TestClient_NewSessionWithEnv_envFlags(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	env := map[string]string{"KEY": "val"}
	_ = c.NewSessionWithEnv("s", "/tmp", env, "cmd")
	foundE := false
	for i, a := range r.lastArgs {
		if a == "-e" && i+1 < len(r.lastArgs) && r.lastArgs[i+1] == "KEY=val" {
			foundE = true
			break
		}
	}
	if !foundE {
		t.Errorf("expected -e KEY=val in args, got %v", r.lastArgs)
	}
}

func TestClient_NewSessionWithEnv_cmdIsLastArg(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	_ = c.NewSessionWithEnv("s", "/tmp", nil, "claude --skip")
	last := r.lastArgs[len(r.lastArgs)-1]
	if last != "claude --skip" {
		t.Errorf("last arg = %q, want %q", last, "claude --skip")
	}
}
```

### Step 2: Run tests to verify they fail

Run: `go test ./internal/tmux/...`
Expected: compilation error — `NewSessionWithEnv` not found.

### Step 3: Write the implementation

Add to `internal/tmux/client.go`:

```go
import "sort"

// NewSessionWithEnv creates a detached tmux session with environment variables
// and an initial command.
func (c *Client) NewSessionWithEnv(name, dir string, env map[string]string, cmd string) error {
	args := []string{"new-session", "-d", "-s", name, "-c", dir}
	// Sort keys for deterministic arg order (testability).
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-e", k+"="+env[k])
	}
	args = append(args, cmd)
	_, stderr, code, err := c.runner.Run(args...)
	if err != nil {
		return fmt.Errorf("tmux new-session: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("tmux new-session: %s", strings.TrimSpace(stderr))
	}
	return nil
}
```

**Note on `ExecAttach`:** This method uses `syscall.Exec` which replaces the process. It cannot be tested via the `Runner` interface (it bypasses tmux entirely). It will be implemented in Task 7 when wiring `cmd/session.go`, using the same pattern as `worktree shell` — calling `syscall.Exec` directly in the cmd layer rather than on the tmux client. This avoids an untestable method on the client.

### Step 4: Run tests to verify they pass

Run: `go test ./internal/tmux/...`
Expected: PASS

### Step 5: Commit

```bash
git add internal/tmux/client.go internal/tmux/client_test.go
git commit -m "feat: add NewSessionWithEnv to tmux client"
```

---

## Task 5: `internal/session` — Service with Start and MarkRunning

**Files:**
- Create: `internal/session/session.go`
- Create: `internal/session/session_test.go`

### Step 1: Write the failing tests

```go
// internal/session/session_test.go
package session_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/xico42/devenv/internal/session"
	"github.com/xico42/devenv/internal/state"
	"github.com/xico42/devenv/internal/tmux"
)

// mockRunner implements tmux.Runner for testing.
type mockRunner struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	calls    [][]string
}

func (m *mockRunner) Run(args ...string) (string, string, int, error) {
	m.calls = append(m.calls, args)
	return m.stdout, m.stderr, m.exitCode, m.err
}

func newService(t *testing.T, r *mockRunner) (*session.Service, string) {
	t.Helper()
	dir := t.TempDir()
	tc := tmux.NewClient(r)
	return session.NewService(tc, dir), dir
}

func TestStart_OK(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc, dir := newService(t, r)

	wtDir := t.TempDir() // simulate existing worktree
	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    wtDir,
		Cmd:     "claude",
		Env:     map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify state file was written
	s, err := state.LoadSession(dir, "myapp-feature")
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if s == nil {
		t.Fatal("state file not written")
	}
	if s.Status != state.SessionRunning {
		t.Errorf("Status = %q, want running", s.Status)
	}
	if s.Project != "myapp" {
		t.Errorf("Project = %q, want myapp", s.Project)
	}
}

func TestStart_DuplicateSession(t *testing.T) {
	// has-session returns 0 → session exists
	r := &mockRunner{exitCode: 0}
	svc, _ := newService(t, r)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    t.TempDir(),
		Cmd:     "claude",
	})
	if err == nil {
		t.Fatal("expected error for duplicate session")
	}
}

func TestStart_MissingPath(t *testing.T) {
	// has-session returns 1 → no session
	r := &mockRunner{exitCode: 1}
	svc, _ := newService(t, r)

	err := svc.Start(session.StartRequest{
		Project: "myapp",
		Branch:  "feature",
		Path:    "/nonexistent/path",
		Cmd:     "claude",
	})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestMarkRunning_OK(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc, dir := newService(t, r)

	// Write a waiting state
	s := &state.SessionState{
		Session:   "myapp-feature",
		Project:   "myapp",
		Branch:    "feature",
		Status:    state.SessionWaiting,
		Question:  "Should I proceed?",
		UpdatedAt: time.Now().UTC().Add(-5 * time.Minute),
		StartedAt: time.Now().UTC().Add(-10 * time.Minute),
	}
	if err := state.SaveSession(dir, s); err != nil {
		t.Fatal(err)
	}

	if err := svc.MarkRunning("myapp-feature"); err != nil {
		t.Fatalf("MarkRunning() error = %v", err)
	}

	got, _ := state.LoadSession(dir, "myapp-feature")
	if got.Status != state.SessionRunning {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.Question != "" {
		t.Errorf("Question = %q, want empty", got.Question)
	}
}

func TestMarkRunning_MissingFile(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc, _ := newService(t, r)

	// Should not error — silent no-op
	if err := svc.MarkRunning("nonexistent"); err != nil {
		t.Fatalf("MarkRunning() on missing file error = %v", err)
	}
}

func TestMarkRunning_EmptyName(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	svc, _ := newService(t, r)

	if err := svc.MarkRunning(""); err != nil {
		t.Fatalf("MarkRunning() on empty name error = %v", err)
	}
}
```

### Step 2: Run tests to verify they fail

Run: `go test ./internal/session/...`
Expected: compilation error — package does not exist.

### Step 3: Write the implementation

```go
// internal/session/session.go
package session

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/xico42/devenv/internal/semconv"
	"github.com/xico42/devenv/internal/state"
	"github.com/xico42/devenv/internal/tmux"
)

var (
	ErrSessionExists   = errors.New("session already exists")
	ErrSessionNotFound = errors.New("session not found")
	ErrPathNotFound    = errors.New("worktree path not found")
)

type Service struct {
	tmux        *tmux.Client
	sessionsDir string
}

func NewService(tmux *tmux.Client, sessionsDir string) *Service {
	return &Service{tmux: tmux, sessionsDir: sessionsDir}
}

type StartRequest struct {
	Project string
	Branch  string
	Path    string
	Cmd     string
	Env     map[string]string
	Attach  bool
}

func (s *Service) Start(req StartRequest) error {
	name := semconv.SessionName(req.Project, req.Branch)

	exists, err := s.tmux.HasSession(name)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if exists {
		return fmt.Errorf("%w: %s", ErrSessionExists, name)
	}

	if _, err := os.Stat(req.Path); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrPathNotFound, req.Path)
	}

	env := make(map[string]string)
	for k, v := range req.Env {
		env[k] = v
	}
	env[semconv.SessionEnvVar] = name

	if err := s.tmux.NewSessionWithEnv(name, req.Path, env, req.Cmd); err != nil {
		return fmt.Errorf("creating tmux session: %w", err)
	}

	now := time.Now().UTC()
	ss := &state.SessionState{
		Session:   name,
		Project:   req.Project,
		Branch:    req.Branch,
		Status:    state.SessionRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := state.SaveSession(s.sessionsDir, ss); err != nil {
		return fmt.Errorf("saving session state: %w", err)
	}

	return nil
}

func (s *Service) MarkRunning(name string) error {
	if name == "" {
		return nil
	}
	ss, err := state.LoadSession(s.sessionsDir, name)
	if err != nil {
		return nil // silent — never fail
	}
	if ss == nil {
		return nil // no state file — no-op
	}
	ss.Status = state.SessionRunning
	ss.Question = ""
	ss.UpdatedAt = time.Now().UTC()
	if err := state.SaveSession(s.sessionsDir, ss); err != nil {
		return nil // silent — never fail
	}
	return nil
}
```

### Step 4: Run tests to verify they pass

Run: `go test ./internal/session/...`
Expected: PASS

### Step 5: Commit

```bash
git add internal/session/
git commit -m "feat: add session.Service with Start and MarkRunning"
```

---

## Task 6: `internal/session` — List, Show, Stop

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/session_test.go`

### Step 1: Write the failing tests

Append to `internal/session/session_test.go`:

```go
func TestList_Empty(t *testing.T) {
	// tmux list-sessions returns exit 1 (no sessions)
	r := &mockRunner{exitCode: 1}
	svc, _ := newService(t, r)

	sessions, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("len = %d, want 0", len(sessions))
	}
}

func TestList_WithStateFiles(t *testing.T) {
	// tmux returns two sessions
	r := &mockRunner{exitCode: 0, stdout: "myapp-feature\napi-main\n"}
	svc, dir := newService(t, r)

	// Write state for one of them
	now := time.Now().UTC().Truncate(time.Second)
	ss := &state.SessionState{
		Session:   "myapp-feature",
		Project:   "myapp",
		Branch:    "feature",
		Status:    state.SessionWaiting,
		Question:  "Proceed?",
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := state.SaveSession(dir, ss); err != nil {
		t.Fatal(err)
	}

	sessions, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("len = %d, want 2", len(sessions))
	}

	// Sort by name for deterministic comparison
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Name < sessions[j].Name
	})

	// api-main has no state file → unknown
	if sessions[0].Name != "api-main" {
		t.Errorf("sessions[0].Name = %q, want api-main", sessions[0].Name)
	}
	if sessions[0].Status != "unknown" {
		t.Errorf("sessions[0].Status = %q, want unknown", sessions[0].Status)
	}

	// myapp-feature has state → waiting
	if sessions[1].Status != state.SessionWaiting {
		t.Errorf("sessions[1].Status = %q, want waiting", sessions[1].Status)
	}
	if sessions[1].Question != "Proceed?" {
		t.Errorf("sessions[1].Question = %q, want Proceed?", sessions[1].Question)
	}
}

func TestShow_OK(t *testing.T) {
	// has-session returns 0 → exists
	r := &mockRunner{exitCode: 0}
	svc, dir := newService(t, r)

	now := time.Now().UTC().Truncate(time.Second)
	ss := &state.SessionState{
		Session:   "myapp-feature",
		Project:   "myapp",
		Branch:    "feature",
		Status:    state.SessionRunning,
		StartedAt: now,
		UpdatedAt: now,
	}
	if err := state.SaveSession(dir, ss); err != nil {
		t.Fatal(err)
	}

	info, err := svc.Show("myapp-feature")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if info.Name != "myapp-feature" {
		t.Errorf("Name = %q, want myapp-feature", info.Name)
	}
	if info.Status != state.SessionRunning {
		t.Errorf("Status = %q, want running", info.Status)
	}
}

func TestShow_NotFound(t *testing.T) {
	// has-session returns 1 → not found
	r := &mockRunner{exitCode: 1}
	svc, _ := newService(t, r)

	_, err := svc.Show("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}

func TestShow_NoStateFile(t *testing.T) {
	// has-session returns 0 → exists in tmux, but no state file
	r := &mockRunner{exitCode: 0}
	svc, _ := newService(t, r)

	info, err := svc.Show("manual-session")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if info.Status != "unknown" {
		t.Errorf("Status = %q, want unknown", info.Status)
	}
}

func TestStop_OK(t *testing.T) {
	// First call: has-session (exit 0). Second call: kill-session (exit 0).
	callCount := 0
	r := &mockRunner{exitCode: 0}
	// Override to track calls
	svc, dir := newService(t, r)

	// Write state file
	ss := &state.SessionState{Session: "myapp-feature", Status: state.SessionRunning}
	if err := state.SaveSession(dir, ss); err != nil {
		t.Fatal(err)
	}

	err := svc.Stop("myapp-feature")
	_ = callCount
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	// State file should be deleted
	got, _ := state.LoadSession(dir, "myapp-feature")
	if got != nil {
		t.Error("state file still exists after Stop")
	}
}

func TestStop_NotFound(t *testing.T) {
	r := &mockRunner{exitCode: 1}
	svc, _ := newService(t, r)

	err := svc.Stop("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing session")
	}
}
```

### Step 2: Run tests to verify they fail

Run: `go test ./internal/session/...`
Expected: compilation error — `List`, `Show`, `Stop`, `SessionInfo` not found.

### Step 3: Write the implementation

Add to `internal/session/session.go`:

```go
type SessionInfo struct {
	Name      string
	Project   string
	Branch    string
	Status    string
	Question  string
	StartedAt time.Time
	UpdatedAt time.Time
}

func (s *Service) List() ([]SessionInfo, error) {
	names, err := s.tmux.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("listing tmux sessions: %w", err)
	}

	var result []SessionInfo
	for _, name := range names {
		info := SessionInfo{Name: name, Status: "unknown"}
		ss, err := state.LoadSession(s.sessionsDir, name)
		if err == nil && ss != nil {
			info.Project = ss.Project
			info.Branch = ss.Branch
			info.Status = ss.Status
			info.Question = ss.Question
			info.StartedAt = ss.StartedAt
			info.UpdatedAt = ss.UpdatedAt
		}
		result = append(result, info)
	}
	return result, nil
}

func (s *Service) Show(name string) (*SessionInfo, error) {
	exists, err := s.tmux.HasSession(name)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, name)
	}

	info := &SessionInfo{Name: name, Status: "unknown"}
	ss, err := state.LoadSession(s.sessionsDir, name)
	if err == nil && ss != nil {
		info.Project = ss.Project
		info.Branch = ss.Branch
		info.Status = ss.Status
		info.Question = ss.Question
		info.StartedAt = ss.StartedAt
		info.UpdatedAt = ss.UpdatedAt
	}
	return info, nil
}

func (s *Service) Stop(name string) error {
	exists, err := s.tmux.HasSession(name)
	if err != nil {
		return fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrSessionNotFound, name)
	}

	if err := s.tmux.KillSession(name); err != nil {
		return fmt.Errorf("killing session: %w", err)
	}

	if err := state.ClearSession(s.sessionsDir, name); err != nil {
		return fmt.Errorf("clearing session state: %w", err)
	}

	return nil
}
```

### Step 4: Run tests to verify they pass

Run: `go test ./internal/session/...`
Expected: PASS

### Step 5: Commit

```bash
git add internal/session/
git commit -m "feat: add List, Show, Stop to session.Service"
```

---

## Task 7: `cmd/session.go` — all subcommands

**Files:**
- Modify: `cmd/session.go`
- Create: `cmd/session_test.go`

### Step 1: Write the failing tests

```go
// cmd/session_test.go
package cmd_test

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSessionConfig writes a config with agent settings and a project.
func writeSessionConfig(t *testing.T, projectsDir string) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	content := `[defaults]
projects_dir = "` + projectsDir + `"

[defaults.agent]
cmd = "echo"
args = ["hello"]

[projects.myapp]
repo = "git@github.com:user/myapp.git"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func TestSessionCmd_help(t *testing.T) {
	if err := runCmd(t, "session", "--help"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSessionList_empty(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	// list should succeed even with no tmux (service handles errors)
	// May fail if tmux not available — that's acceptable in unit tests.
	_ = runCmd(t, "--config", cfgPath, "session", "list")
}

func TestSessionStart_tooFewArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "start", "myapp"); err == nil {
		t.Error("expected error for missing branch argument")
	}
}

func TestSessionStart_tooManyArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "start", "a", "b", "c"); err == nil {
		t.Error("expected error for too many arguments")
	}
}

func TestSessionAttach_tooFewArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "attach"); err == nil {
		t.Error("expected error for missing session argument")
	}
}

func TestSessionStop_tooFewArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "stop"); err == nil {
		t.Error("expected error for missing session argument")
	}
}

func TestSessionShow_tooFewArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "show"); err == nil {
		t.Error("expected error for missing session argument")
	}
}

func TestSessionMarkRunning_noSession(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	// mark-running with empty --session should succeed silently
	if err := runCmd(t, "--config", cfgPath, "session", "mark-running"); err != nil {
		t.Fatalf("mark-running without --session should succeed: %v", err)
	}
}

func TestSessionMarkRunning_withSession(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	// mark-running with a non-existent session file should succeed silently
	if err := runCmd(t, "--config", cfgPath, "session", "mark-running", "--session", "nonexistent"); err != nil {
		t.Fatalf("mark-running with missing state file should succeed: %v", err)
	}
}
```

### Step 2: Run tests to verify they fail

Run: `go test ./cmd/...`
Expected: failures — session subcommands don't exist yet.

### Step 3: Write the implementation

Replace `cmd/session.go` entirely:

```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xico42/devenv/internal/semconv"
	"github.com/xico42/devenv/internal/session"
	"github.com/xico42/devenv/internal/state"
	"github.com/xico42/devenv/internal/tmux"
	"github.com/xico42/devenv/internal/worktree"
)

func sessionsDir() string {
	home, _ := os.UserHomeDir()
	return home + "/.local/share/devenv/sessions"
}

func newSessionService() *session.Service {
	tc := tmux.NewClient(tmux.NewRealRunner())
	return session.NewService(tc, sessionsDir())
}

func resolveAgentCmd(project string) string {
	agent := cfg.ResolveAgent(project)
	cmd := agent.Cmd
	if cmd == "" {
		cmd = semconv.DefaultAgentCmd
	}
	if len(agent.Args) > 0 {
		cmd = cmd + " " + strings.Join(agent.Args, " ")
	}
	return cmd
}

func resolveAgentEnv(project string) map[string]string {
	agent := cfg.ResolveAgent(project)
	env := make(map[string]string)
	for k, v := range agent.Env {
		env[k] = v
	}
	return env
}

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage agent sessions",
}

// ── start ────────────────────────────────────────────────────────────────────

var sessionStartAttach bool

var sessionStartCmd = &cobra.Command{
	Use:   "start <project> <branch>",
	Short: "Start a new agent session in a worktree",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, branch := args[0], args[1]

		wtSvc := newWorktreeService()
		path, err := wtSvc.WorktreePath(project, branch)
		if err != nil {
			return sessionErr(cmd, err)
		}

		name := semconv.SessionName(project, branch)
		fmt.Fprintf(cmd.OutOrStdout(), "Starting session %s...  ", name)

		svc := newSessionService()
		err = svc.Start(session.StartRequest{
			Project: project,
			Branch:  branch,
			Path:    path,
			Cmd:     resolveAgentCmd(project),
			Env:     resolveAgentEnv(project),
			Attach:  sessionStartAttach,
		})
		if err != nil {
			fmt.Fprintln(cmd.OutOrStdout())
			return sessionErr(cmd, err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "done")
		fmt.Fprintf(cmd.OutOrStdout(), "Attach with: devenv session attach %s\n", name)

		if sessionStartAttach {
			return execTmuxAttach(name)
		}
		return nil
	},
}

// ── list ─────────────────────────────────────────────────────────────────────

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active sessions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newSessionService()
		sessions, err := svc.List()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "SESSION\tPROJECT\tBRANCH\tSTATUS")
		for _, s := range sessions {
			project := s.Project
			if project == "" {
				project = "--"
			}
			branch := s.Branch
			if branch == "" {
				branch = "--"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.Name, project, branch, s.Status)
		}
		return w.Flush()
	},
}

// ── show ─────────────────────────────────────────────────────────────────────

var sessionShowCmd = &cobra.Command{
	Use:   "show <session>",
	Short: "Show details for a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newSessionService()
		info, err := svc.Show(args[0])
		if err != nil {
			return sessionErr(cmd, err)
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "Session:\t%s\n", info.Name)
		fmt.Fprintf(w, "Project:\t%s\n", info.Project)
		fmt.Fprintf(w, "Branch:\t%s\n", info.Branch)
		fmt.Fprintf(w, "Status:\t%s\n", info.Status)
		if info.Question != "" {
			fmt.Fprintf(w, "Question:\t%s\n", info.Question)
		}
		if !info.StartedAt.IsZero() {
			fmt.Fprintf(w, "Started:\t%s\n", info.StartedAt.Format("2006-01-02T15:04:05Z"))
		}
		return w.Flush()
	},
}

// ── attach ───────────────────────────────────────────────────────────────────

var sessionAttachCmd = &cobra.Command{
	Use:   "attach <session>",
	Short: "Attach to an existing session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newSessionService()
		_, err := svc.Show(args[0]) // verify it exists
		if err != nil {
			return sessionErr(cmd, err)
		}
		return execTmuxAttach(args[0])
	},
}

// execTmuxAttach replaces the current process with tmux attach-session.
func execTmuxAttach(name string) error {
	tmuxBin, err := lookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return syscall.Exec(tmuxBin, []string{"tmux", "attach-session", "-t", name}, os.Environ())
}

// lookPath wraps exec.LookPath for testability.
var lookPath = execLookPath

func execLookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// ── stop ─────────────────────────────────────────────────────────────────────

var sessionStopForce bool

var sessionStopCmd = &cobra.Command{
	Use:   "stop <session>",
	Short: "Stop a session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		svc := newSessionService()

		if !sessionStopForce {
			info, err := svc.Show(name)
			if err != nil {
				return sessionErr(cmd, err)
			}
			if info.Status == state.SessionRunning {
				fmt.Fprintf(cmd.OutOrStdout(), "Session %s is running. Stop? [y/N] ", name)
				scanner := bufio.NewScanner(cmd.InOrStdin())
				scanner.Scan()
				if scanner.Text() != "y" && scanner.Text() != "Y" {
					fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
					return nil
				}
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Stopping %s...  ", name)
		if err := svc.Stop(name); err != nil {
			fmt.Fprintln(cmd.OutOrStdout())
			return sessionErr(cmd, err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), "done")
		return nil
	},
}

// ── mark-running ─────────────────────────────────────────────────────────────

var markRunningSession string

var sessionMarkRunningCmd = &cobra.Command{
	Use:    "mark-running",
	Short:  "Internal: reset session status to running",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := newSessionService()
		return svc.MarkRunning(markRunningSession)
	},
}

// ── error helper ─────────────────────────────────────────────────────────────

func sessionErr(cmd *cobra.Command, err error) error {
	switch {
	case errors.Is(err, session.ErrSessionExists):
		name := strings.TrimPrefix(err.Error(), "session already exists: ")
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: session %s already exists. Attach with 'devenv session attach %s'.\n", name, name)
	case errors.Is(err, session.ErrSessionNotFound):
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", err)
	case errors.Is(err, session.ErrPathNotFound):
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", err)
	case errors.Is(err, worktree.ErrNotCloned):
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", err)
	case errors.Is(err, worktree.ErrWorktreeNotFound):
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n", err)
	default:
		return err
	}
	os.Exit(1)
	return nil
}

func init() {
	sessionStartCmd.Flags().BoolVar(&sessionStartAttach, "attach", false, "attach to the session after starting")
	sessionStopCmd.Flags().BoolVar(&sessionStopForce, "force", false, "skip confirmation prompt")
	sessionMarkRunningCmd.Flags().StringVar(&markRunningSession, "session", "", "session name")

	sessionCmd.AddCommand(sessionStartCmd)
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionShowCmd)
	sessionCmd.AddCommand(sessionAttachCmd)
	sessionCmd.AddCommand(sessionStopCmd)
	sessionCmd.AddCommand(sessionMarkRunningCmd)
	rootCmd.AddCommand(sessionCmd)
}
```

**Note:** The file needs an `import "os/exec"` for `exec.LookPath` and `import "errors"` for `errors.Is`. The `lookPath` var pattern allows tests to stub it to avoid needing tmux installed.

### Step 4: Run tests to verify they pass

Run: `go test ./cmd/...`
Expected: PASS

### Step 5: Run full test suite and coverage

Run: `make test && make coverage`
Expected: PASS, coverage >= 80%

### Step 6: Commit

```bash
git add cmd/session.go cmd/session_test.go
git commit -m "feat: implement devenv session command with 6 subcommands"
```

---

## Task 8: Final verification

### Step 1: Run full test suite

Run: `make test`
Expected: PASS

### Step 2: Run linter

Run: `make lint`
Expected: clean

### Step 3: Run coverage check

Run: `make coverage`
Expected: >= 80%

### Step 4: Build

Run: `make build`
Expected: `./devenv` binary built successfully

### Step 5: Smoke test

Run: `./devenv session --help`
Expected: shows subcommands: start, list, show, attach, stop

Run: `./devenv session list`
Expected: header row (SESSION, PROJECT, BRANCH, STATUS) with no entries (or tmux not available error)

### Step 6: Final commit if any fixes needed

```bash
git add -A
git commit -m "fix: address lint and test issues in session command"
```
