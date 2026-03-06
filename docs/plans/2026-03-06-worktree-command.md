# `devenv worktree` Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the `devenv worktree` command with five subcommands (`list`, `new`, `delete`, `shell`, `env`) managing git worktrees for configured projects.

**Architecture:** Business logic lives in `internal/worktree` (Service + WorktreeRunner interface). Tmux operations go through a new shared `internal/tmux` package (Runner interface + typed Client). The cmd layer is thin wiring only. `envtemplate.Process` is called directly — it's a pure function.

**Tech Stack:** Go stdlib (`os/exec`, `syscall`, `bufio`, `path/filepath`), Cobra, existing `internal/config`, `internal/envtemplate`.

---

## Task 1: `internal/tmux` — Runner, RealRunner, Client

**Files:**
- Create: `internal/tmux/runner.go`
- Create: `internal/tmux/client.go`
- Create: `internal/tmux/client_test.go`

### Step 1: Write the failing tests

```go
// internal/tmux/client_test.go
package tmux_test

import (
	"testing"

	"github.com/xico42/devenv/internal/tmux"
)

type mockRunner struct {
	stdout   string
	stderr   string
	exitCode int
	err      error
	lastArgs []string
}

func (m *mockRunner) Run(args ...string) (string, string, int, error) {
	m.lastArgs = args
	return m.stdout, m.stderr, m.exitCode, m.err
}

func TestClient_HasSession_found(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	got, err := c.HasSession("myapp-feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected true")
	}
	if r.lastArgs[0] != "has-session" || r.lastArgs[2] != "myapp-feature" {
		t.Errorf("unexpected args: %v", r.lastArgs)
	}
}

func TestClient_HasSession_notFound(t *testing.T) {
	r := &mockRunner{exitCode: 1}
	c := tmux.NewClient(r)
	got, err := c.HasSession("myapp-feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected false for exit code 1")
	}
}

func TestClient_HasSession_execError(t *testing.T) {
	r := &mockRunner{exitCode: -1, err: fmt.Errorf("tmux not found")}
	c := tmux.NewClient(r)
	_, err := c.HasSession("myapp-feature")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_KillSession_ok(t *testing.T) {
	r := &mockRunner{exitCode: 0}
	c := tmux.NewClient(r)
	if err := c.KillSession("myapp-feature"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.lastArgs[0] != "kill-session" || r.lastArgs[2] != "myapp-feature" {
		t.Errorf("unexpected args: %v", r.lastArgs)
	}
}

func TestClient_KillSession_error(t *testing.T) {
	r := &mockRunner{exitCode: 1, stderr: "no such session"}
	c := tmux.NewClient(r)
	if err := c.KillSession("myapp-feature"); err == nil {
		t.Fatal("expected error")
	}
}

func TestClient_ListSessions_ok(t *testing.T) {
	r := &mockRunner{exitCode: 0, stdout: "foo\nbar\n"}
	c := tmux.NewClient(r)
	sessions, err := c.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 2 || sessions[0] != "foo" || sessions[1] != "bar" {
		t.Errorf("unexpected sessions: %v", sessions)
	}
}

func TestClient_ListSessions_none(t *testing.T) {
	// tmux exits 1 when no sessions — not an error
	r := &mockRunner{exitCode: 1}
	c := tmux.NewClient(r)
	sessions, err := c.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty, got %v", sessions)
	}
}
```

Note: the test file uses `fmt` — add `"fmt"` to the import block.

### Step 2: Run tests to verify they fail

```bash
cd ~/.config/superpowers/worktrees/remote-dev/feat/cmd-worktree
go test ./internal/tmux/...
```

Expected: compile error — package does not exist yet.

### Step 3: Implement `runner.go`

```go
// internal/tmux/runner.go
package tmux

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
)

// Runner executes raw tmux commands. Implement this interface in tests to avoid
// spawning a real tmux process.
type Runner interface {
	Run(args ...string) (stdout, stderr string, exitCode int, err error)
}

// RealRunner executes tmux via os/exec.
type RealRunner struct{}

// NewRealRunner returns a Runner backed by the system tmux binary.
func NewRealRunner() *RealRunner { return &RealRunner{} }

// Run executes tmux with the given arguments. Returns stdout, stderr, exit code,
// and a non-nil err only when the process could not be started at all.
func (r *RealRunner) Run(args ...string) (stdout, stderr string, exitCode int, err error) {
	cmd := exec.Command("tmux", args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return stdout, stderr, exitErr.ExitCode(), nil
		}
		return stdout, stderr, -1, fmt.Errorf("running tmux %v: %w", args, runErr)
	}
	return stdout, stderr, 0, nil
}
```

### Step 4: Implement `client.go`

```go
// internal/tmux/client.go
package tmux

import (
	"fmt"
	"strings"
)

// Client provides typed tmux operations built on a Runner.
type Client struct {
	runner Runner
}

// NewClient creates a Client using the given Runner.
func NewClient(r Runner) *Client {
	return &Client{runner: r}
}

// HasSession reports whether a tmux session with the given name exists.
// tmux exits with code 1 to signal "no session" — this is not an error.
func (c *Client) HasSession(name string) (bool, error) {
	_, _, code, err := c.runner.Run("has-session", "-t", name)
	if err != nil {
		return false, fmt.Errorf("tmux has-session: %w", err)
	}
	return code == 0, nil
}

// KillSession terminates the named tmux session.
func (c *Client) KillSession(name string) error {
	_, stderr, code, err := c.runner.Run("kill-session", "-t", name)
	if err != nil {
		return fmt.Errorf("tmux kill-session: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("tmux kill-session: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// NewSession creates a detached tmux session with the given name and start directory.
func (c *Client) NewSession(name, dir string) error {
	_, stderr, code, err := c.runner.Run("new-session", "-d", "-s", name, "-c", dir)
	if err != nil {
		return fmt.Errorf("tmux new-session: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("tmux new-session: %s", strings.TrimSpace(stderr))
	}
	return nil
}

// ListSessions returns the names of all active tmux sessions.
// Returns nil (no error) when no sessions exist (tmux exits 1 in that case).
func (c *Client) ListSessions() ([]string, error) {
	stdout, stderr, code, err := c.runner.Run("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}
	if code == 1 {
		return nil, nil // no sessions — not an error
	}
	if code != 0 {
		return nil, fmt.Errorf("tmux list-sessions: %s", strings.TrimSpace(stderr))
	}
	var sessions []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line != "" {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
}
```

### Step 5: Run tests to verify they pass

```bash
go test ./internal/tmux/... -v
```

Expected: all tests PASS.

### Step 6: Commit

```bash
git add internal/tmux/
git commit -m "feat: add internal/tmux package with Runner interface and Client"
```

---

## Task 2: `internal/worktree` — foundation

**Files:**
- Create: `internal/worktree/worktree.go`
- Create: `internal/worktree/worktree_test.go` (parseWorktreePorcelain + flattenBranch tests only)

### Step 1: Write the failing tests

```go
// internal/worktree/worktree_test.go
package worktree

import (
	"testing"
)

func TestFlattenBranch(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"feature", "feature"},
		{"feature/login", "feature-login"},
		{"fix/123/auth", "fix-123-auth"},
		{"main", "main"},
	}
	for _, tc := range cases {
		got := flattenBranch(tc.in)
		if got != tc.want {
			t.Errorf("flattenBranch(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseWorktreePorcelain(t *testing.T) {
	input := `worktree /home/user/projects/myapp
HEAD abc123
branch refs/heads/main

worktree /home/user/projects/myapp__worktrees/feature
HEAD def456
branch refs/heads/feature

worktree /home/user/projects/myapp__worktrees/detached
HEAD ghi789
detached

`
	got := parseWorktreePorcelain(input)

	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	if got[0].Path != "/home/user/projects/myapp" || got[0].Branch != "main" {
		t.Errorf("entry 0: %+v", got[0])
	}
	if got[1].Path != "/home/user/projects/myapp__worktrees/feature" || got[1].Branch != "feature" {
		t.Errorf("entry 1: %+v", got[1])
	}
	if got[2].Branch != "" {
		t.Errorf("entry 2 should have empty branch for detached HEAD, got %q", got[2].Branch)
	}
}

func TestParseWorktreePorcelain_empty(t *testing.T) {
	got := parseWorktreePorcelain("")
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}
```

### Step 2: Run tests to verify they fail

```bash
go test ./internal/worktree/...
```

Expected: compile error — package does not exist yet.

### Step 3: Implement `worktree.go` (foundation — no Service methods yet)

```go
// internal/worktree/worktree.go
package worktree

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/envtemplate"
	"github.com/xico42/devenv/internal/tmux"
)

// Sentinel errors returned by Service methods.
var (
	ErrNotCloned        = errors.New("project not cloned")
	ErrWorktreeExists   = errors.New("worktree already exists")
	ErrWorktreeNotFound = errors.New("worktree not found")
	ErrSessionRunning   = errors.New("session is running")
)

// WorktreeInfo holds data from a single git worktree entry.
type WorktreeInfo struct {
	Path   string
	Branch string // empty if detached HEAD
}

// ListEntry is one row in the worktree list output.
type ListEntry struct {
	Project string
	Branch  string
	Path    string
	Session string // "<name>-<branch> (running)" or ""
}

// NewResult is the result of a successful worktree creation.
type NewResult struct {
	Path       string
	EnvWritten bool
}

// EnvResult is the result of env template processing.
type EnvResult struct {
	Output string
	Source string
	DryRun bool
}

// WorktreeRunner abstracts git worktree operations for testability.
type WorktreeRunner interface {
	Add(cloneDir, worktreePath, branch string) error
	AddNewBranch(cloneDir, worktreePath, branch string) error
	Remove(cloneDir, worktreePath string) error
	List(cloneDir string) ([]WorktreeInfo, error)
}

// RealWorktreeRunner runs git worktree commands via os/exec.
type RealWorktreeRunner struct{}

// NewRealWorktreeRunner returns a WorktreeRunner backed by the system git binary.
func NewRealWorktreeRunner() *RealWorktreeRunner { return &RealWorktreeRunner{} }

func (r *RealWorktreeRunner) Add(cloneDir, worktreePath, branch string) error {
	cmd := exec.Command("git", "worktree", "add", worktreePath, branch)
	cmd.Dir = cloneDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add: %w\n%s", err, out)
	}
	return nil
}

func (r *RealWorktreeRunner) AddNewBranch(cloneDir, worktreePath, branch string) error {
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreePath)
	cmd.Dir = cloneDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add -b: %w\n%s", err, out)
	}
	return nil
}

func (r *RealWorktreeRunner) Remove(cloneDir, worktreePath string) error {
	cmd := exec.Command("git", "worktree", "remove", worktreePath)
	cmd.Dir = cloneDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree remove: %w\n%s", err, out)
	}
	return nil
}

func (r *RealWorktreeRunner) List(cloneDir string) ([]WorktreeInfo, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = cloneDir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	return parseWorktreePorcelain(string(out)), nil
}

// parseWorktreePorcelain parses the output of `git worktree list --porcelain`.
// Blocks are separated by blank lines.
func parseWorktreePorcelain(output string) []WorktreeInfo {
	var result []WorktreeInfo
	var current WorktreeInfo
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "worktree "):
			current = WorktreeInfo{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch "):
			ref := strings.TrimPrefix(line, "branch ")
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		case line == "":
			if current.Path != "" {
				result = append(result, current)
				current = WorktreeInfo{}
			}
		}
	}
	if current.Path != "" {
		result = append(result, current)
	}
	return result
}

// Service provides worktree management operations.
type Service struct {
	cfg  *config.Config
	git  WorktreeRunner
	tmux *tmux.Client
}

// NewService creates a Service.
func NewService(cfg *config.Config, git WorktreeRunner, tmux *tmux.Client) *Service {
	return &Service{cfg: cfg, git: git, tmux: tmux}
}

// flattenBranch converts a branch name to a filesystem-safe directory name.
// "feature/login" -> "feature-login"
func flattenBranch(branch string) string {
	return strings.ReplaceAll(branch, "/", "-")
}

// resolvePaths returns cloneDir, worktreesRoot, and worktreePath for a project+branch.
func (s *Service) resolvePaths(project, branch string) (cloneDir, worktreesRoot, worktreePath string, err error) {
	p, ok := s.cfg.Projects[project]
	if !ok {
		return "", "", "", fmt.Errorf("project %q is not configured", project)
	}
	repoPath, err := config.RepoPath(p.Repo)
	if err != nil {
		return "", "", "", fmt.Errorf("parsing repo URL: %w", err)
	}
	cloneDir = filepath.Join(s.cfg.Defaults.ProjectsDir, repoPath)
	worktreesRoot = cloneDir + "__worktrees"
	worktreePath = filepath.Join(worktreesRoot, flattenBranch(branch))
	return cloneDir, worktreesRoot, worktreePath, nil
}

// resolveTemplate finds the .env template for a worktree.
// Returns ("", "", nil) when no template is configured — callers decide whether to error.
// Priority: repo-local .env.template > config EnvTemplate path.
func resolveTemplate(worktreePath string, projCfg config.ProjectConfig) (content, source string, err error) {
	repoLocal := filepath.Join(worktreePath, ".env.template")
	if data, readErr := os.ReadFile(repoLocal); readErr == nil {
		return string(data), "repo-local", nil
	}
	if projCfg.EnvTemplate != "" {
		data, readErr := os.ReadFile(projCfg.EnvTemplate)
		if readErr != nil {
			return "", "", fmt.Errorf("reading env template %q: %w", projCfg.EnvTemplate, readErr)
		}
		return string(data), projCfg.EnvTemplate, nil
	}
	return "", "", nil
}

// Ensure envtemplate import is used (methods added in later tasks reference it).
var _ = envtemplate.Process
```

> **Note:** The `var _ = envtemplate.Process` line prevents "imported and not used" until later tasks add the real calls. Remove it once `New` or `Env` methods are implemented in Tasks 3 and 6.

### Step 4: Run tests to verify they pass

```bash
go test ./internal/worktree/... -v -run "TestFlattenBranch|TestParseWorktreePorcelain"
```

Expected: all PASS.

### Step 5: Commit

```bash
git add internal/worktree/worktree.go internal/worktree/worktree_test.go
git commit -m "feat: add internal/worktree foundation (types, runner, parser)"
```

---

## Task 3: `Service.New`

**Files:**
- Modify: `internal/worktree/worktree.go` — add `New` method
- Modify: `internal/worktree/worktree_test.go` — add New tests

### Step 1: Write the failing tests

Add to `worktree_test.go`:

```go
import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/tmux"
)

// mockGit records calls and controls return values.
type mockGit struct {
	addErr        error
	addNewErr     error
	removeErr     error
	listResult    []WorktreeInfo
	listErr       error
	addCalled     bool
	addNewCalled  bool
}

func (m *mockGit) Add(cloneDir, worktreePath, branch string) error {
	m.addCalled = true
	return m.addErr
}
func (m *mockGit) AddNewBranch(cloneDir, worktreePath, branch string) error {
	m.addNewCalled = true
	return m.addNewErr
}
func (m *mockGit) Remove(cloneDir, worktreePath string) error { return m.removeErr }
func (m *mockGit) List(cloneDir string) ([]WorktreeInfo, error) {
	return m.listResult, m.listErr
}

// mockTmuxRunner controls tmux subprocess results.
type mockTmuxRunner struct {
	exitCode int
	stdout   string
}

func (m *mockTmuxRunner) Run(args ...string) (string, string, int, error) {
	return m.stdout, "", m.exitCode, nil
}

// makeService creates a Service backed by mocks with a temp projects dir.
// Returns the Service and the temp dir.
func makeService(t *testing.T, git WorktreeRunner, tmuxRunner tmux.Runner) (*Service, string) {
	t.Helper()
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Defaults: config.DefaultsConfig{ProjectsDir: tmpDir},
		Projects: map[string]config.ProjectConfig{
			"myapp": {Repo: "git@github.com:user/myapp.git", DefaultBranch: "main"},
		},
	}
	tc := tmux.NewClient(tmuxRunner)
	return NewService(cfg, git, tc), tmpDir
}

// cloneDir returns the expected clone path for "myapp" in tmpDir.
func cloneDir(tmpDir string) string {
	return filepath.Join(tmpDir, "github.com", "user", "myapp")
}

func TestService_New_notCloned(t *testing.T) {
	svc, _ := makeService(t, &mockGit{}, &mockTmuxRunner{})
	_, err := svc.New("myapp", "feature")
	if !errors.Is(err, ErrNotCloned) {
		t.Errorf("expected ErrNotCloned, got %v", err)
	}
}

func TestService_New_worktreeExists(t *testing.T) {
	svc, tmpDir := makeService(t, &mockGit{}, &mockTmuxRunner{})
	clone := cloneDir(tmpDir)
	os.MkdirAll(clone, 0o755)
	// Pre-create the worktree path
	worktreePath := clone + "__worktrees/feature"
	os.MkdirAll(worktreePath, 0o755)

	_, err := svc.New("myapp", "feature")
	if !errors.Is(err, ErrWorktreeExists) {
		t.Errorf("expected ErrWorktreeExists, got %v", err)
	}
}

func TestService_New_success(t *testing.T) {
	git := &mockGit{}
	svc, tmpDir := makeService(t, git, &mockTmuxRunner{})
	os.MkdirAll(cloneDir(tmpDir), 0o755)

	result, err := svc.New("myapp", "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !git.addCalled {
		t.Error("expected git.Add to be called")
	}
	expectedPath := cloneDir(tmpDir) + "__worktrees/feature"
	if result.Path != expectedPath {
		t.Errorf("path = %q, want %q", result.Path, expectedPath)
	}
}

func TestService_New_branchNotFound_fallsBackToAddNew(t *testing.T) {
	git := &mockGit{addErr: fmt.Errorf("invalid reference")}
	svc, tmpDir := makeService(t, git, &mockTmuxRunner{})
	os.MkdirAll(cloneDir(tmpDir), 0o755)

	_, err := svc.New("myapp", "new-feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !git.addNewCalled {
		t.Error("expected AddNewBranch to be called on Add failure")
	}
}

func TestService_New_branchFlattened(t *testing.T) {
	git := &mockGit{}
	svc, tmpDir := makeService(t, git, &mockTmuxRunner{})
	os.MkdirAll(cloneDir(tmpDir), 0o755)

	result, err := svc.New("myapp", "feature/login")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(result.Path, "feature-login") {
		t.Errorf("expected flattened path, got %q", result.Path)
	}
}
```

Add `"errors"`, `"fmt"`, `"strings"` to the import block.

### Step 2: Run tests to verify they fail

```bash
go test ./internal/worktree/... -run "TestService_New"
```

Expected: FAIL — `New` method does not exist yet.

### Step 3: Implement `Service.New`

Add to `worktree.go` (after `resolvePaths`):

```go
// New creates a new git worktree for the given project and branch.
func (s *Service) New(project, branch string) (NewResult, error) {
	p, ok := s.cfg.Projects[project]
	if !ok {
		return NewResult{}, fmt.Errorf("project %q is not configured", project)
	}

	cloneDir, worktreesRoot, worktreePath, err := s.resolvePaths(project, branch)
	if err != nil {
		return NewResult{}, err
	}

	if _, err := os.Stat(cloneDir); os.IsNotExist(err) {
		return NewResult{}, fmt.Errorf("%w: %s", ErrNotCloned, project)
	}

	if _, err := os.Stat(worktreePath); err == nil {
		return NewResult{}, fmt.Errorf("%w: %s/%s", ErrWorktreeExists, project, branch)
	}

	if err := os.MkdirAll(worktreesRoot, 0o755); err != nil {
		return NewResult{}, fmt.Errorf("creating worktrees dir: %w", err)
	}

	addErr := s.git.Add(cloneDir, worktreePath, branch)
	if addErr != nil {
		if err := s.git.AddNewBranch(cloneDir, worktreePath, branch); err != nil {
			return NewResult{}, fmt.Errorf("failed to create worktree: %w", addErr)
		}
	}

	result := NewResult{Path: worktreePath}

	content, source, _ := resolveTemplate(worktreePath, p)
	if content != "" {
		ctx := envtemplate.EnvTemplateContext{
			Project:      project,
			Branch:       branch,
			WorktreePath: worktreePath,
			SessionName:  project + "-" + branch,
		}
		if rendered, renderErr := envtemplate.Process(content, source, ctx); renderErr == nil {
			envPath := filepath.Join(worktreePath, ".env")
			if writeErr := os.WriteFile(envPath, []byte(rendered), 0o644); writeErr == nil {
				result.EnvWritten = true
			}
		}
	}

	return result, nil
}
```

Remove the placeholder `var _ = envtemplate.Process` line now that `New` uses it directly.

### Step 4: Run tests to verify they pass

```bash
go test ./internal/worktree/... -run "TestService_New" -v
```

Expected: all PASS.

### Step 5: Commit

```bash
git add internal/worktree/worktree.go internal/worktree/worktree_test.go
git commit -m "feat: implement Service.New for worktree creation"
```

---

## Task 4: `Service.List`

**Files:**
- Modify: `internal/worktree/worktree.go` — add `List` method
- Modify: `internal/worktree/worktree_test.go` — add List tests

### Step 1: Write the failing tests

```go
func TestService_List_allProjects(t *testing.T) {
	git := &mockGit{
		listResult: []WorktreeInfo{
			{Path: "/tmp/myapp", Branch: "main"},
			{Path: "/tmp/myapp__worktrees/feature", Branch: "feature"},
		},
	}
	// tmux exit 1 = no session
	svc, tmpDir := makeService(t, git, &mockTmuxRunner{exitCode: 1})
	os.MkdirAll(cloneDir(tmpDir), 0o755)

	entries, err := svc.List("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Project != "myapp" {
		t.Errorf("expected project myapp, got %q", entries[0].Project)
	}
}

func TestService_List_withRunningSession(t *testing.T) {
	git := &mockGit{
		listResult: []WorktreeInfo{
			{Path: "/tmp/myapp__worktrees/feature", Branch: "feature"},
		},
	}
	// tmux exit 0 = session exists
	svc, tmpDir := makeService(t, git, &mockTmuxRunner{exitCode: 0})
	os.MkdirAll(cloneDir(tmpDir), 0o755)

	entries, err := svc.List("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected entries")
	}
	if entries[0].Session == "" {
		t.Errorf("expected session name to be populated, got empty")
	}
}

func TestService_List_skipUncloned(t *testing.T) {
	git := &mockGit{}
	svc, _ := makeService(t, git, &mockTmuxRunner{exitCode: 1})
	// cloneDir does NOT exist — project should be skipped

	entries, err := svc.List("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries for uncloned project, got %d", len(entries))
	}
}

func TestService_List_singleProject_notConfigured(t *testing.T) {
	svc, _ := makeService(t, &mockGit{}, &mockTmuxRunner{})
	_, err := svc.List("nonexistent")
	if err == nil {
		t.Fatal("expected error for unconfigured project")
	}
}
```

### Step 2: Run to verify they fail

```bash
go test ./internal/worktree/... -run "TestService_List"
```

Expected: FAIL — `List` not defined.

### Step 3: Implement `Service.List`

```go
// List returns worktree entries for all configured projects, or just the named one.
// Skips projects that are not cloned. Never returns an error for individual project
// failures — those are silently skipped.
func (s *Service) List(project string) ([]ListEntry, error) {
	names, err := s.projectNames(project)
	if err != nil {
		return nil, err
	}

	var entries []ListEntry
	for _, name := range names {
		p := s.cfg.Projects[name]
		repoPath, err := config.RepoPath(p.Repo)
		if err != nil {
			continue
		}
		cd := filepath.Join(s.cfg.Defaults.ProjectsDir, repoPath)
		if _, err := os.Stat(cd); os.IsNotExist(err) {
			continue
		}

		worktrees, err := s.git.List(cd)
		if err != nil {
			continue
		}

		for _, wt := range worktrees {
			session := ""
			if wt.Branch != "" {
				candidate := name + "-" + wt.Branch
				if running, _ := s.tmux.HasSession(candidate); running {
					session = candidate + " (running)"
				}
			}
			entries = append(entries, ListEntry{
				Project: name,
				Branch:  wt.Branch,
				Path:    wt.Path,
				Session: session,
			})
		}
	}
	return entries, nil
}

// projectNames returns sorted project names. If project is non-empty, validates and
// returns just that one.
func (s *Service) projectNames(project string) ([]string, error) {
	if project != "" {
		if _, ok := s.cfg.Projects[project]; !ok {
			return nil, fmt.Errorf("project %q is not configured", project)
		}
		return []string{project}, nil
	}
	names := make([]string, 0, len(s.cfg.Projects))
	for name := range s.cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
```

Add `"sort"` to the import block.

### Step 4: Run tests to verify they pass

```bash
go test ./internal/worktree/... -run "TestService_List" -v
```

Expected: all PASS.

### Step 5: Commit

```bash
git add internal/worktree/worktree.go internal/worktree/worktree_test.go
git commit -m "feat: implement Service.List for worktree listing"
```

---

## Task 5: `Service.Delete`

**Files:**
- Modify: `internal/worktree/worktree.go` — add `Delete` method and `DeleteRequest` type
- Modify: `internal/worktree/worktree_test.go` — add Delete tests

### Step 1: Write the failing tests

```go
func TestService_Delete_notFound(t *testing.T) {
	svc, _ := makeService(t, &mockGit{}, &mockTmuxRunner{exitCode: 1})
	// worktree dir does not exist
	err := svc.Delete(DeleteRequest{Project: "myapp", Branch: "feature"})
	if !errors.Is(err, ErrWorktreeNotFound) {
		t.Errorf("expected ErrWorktreeNotFound, got %v", err)
	}
}

func TestService_Delete_sessionRunning_noForce(t *testing.T) {
	svc, tmpDir := makeService(t, &mockGit{}, &mockTmuxRunner{exitCode: 0}) // session exists
	// Create worktree dir so stat check passes
	worktreePath := cloneDir(tmpDir) + "__worktrees/feature"
	os.MkdirAll(worktreePath, 0o755)

	err := svc.Delete(DeleteRequest{Project: "myapp", Branch: "feature", Force: false})
	if !errors.Is(err, ErrSessionRunning) {
		t.Errorf("expected ErrSessionRunning, got %v", err)
	}
}

func TestService_Delete_sessionRunning_force(t *testing.T) {
	git := &mockGit{}
	svc, tmpDir := makeService(t, git, &mockTmuxRunner{exitCode: 0}) // session exists
	worktreePath := cloneDir(tmpDir) + "__worktrees/feature"
	os.MkdirAll(worktreePath, 0o755)

	err := svc.Delete(DeleteRequest{Project: "myapp", Branch: "feature", Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// git.Remove should have been called
	if git.removeErr != nil {
		t.Error("expected remove to succeed")
	}
}

func TestService_Delete_success(t *testing.T) {
	git := &mockGit{}
	svc, tmpDir := makeService(t, git, &mockTmuxRunner{exitCode: 1}) // no session
	worktreePath := cloneDir(tmpDir) + "__worktrees/feature"
	os.MkdirAll(worktreePath, 0o755)

	err := svc.Delete(DeleteRequest{Project: "myapp", Branch: "feature"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
```

### Step 2: Run to verify they fail

```bash
go test ./internal/worktree/... -run "TestService_Delete"
```

Expected: FAIL — `Delete` / `DeleteRequest` not defined.

### Step 3: Implement `DeleteRequest` and `Service.Delete`

```go
// DeleteRequest holds parameters for a worktree deletion.
type DeleteRequest struct {
	Project string
	Branch  string
	Force   bool
}

// Delete removes a git worktree. Returns ErrWorktreeNotFound if the worktree
// directory does not exist, ErrSessionRunning if a tmux session is active and
// Force is false.
func (s *Service) Delete(req DeleteRequest) error {
	cloneDir, _, worktreePath, err := s.resolvePaths(req.Project, req.Branch)
	if err != nil {
		return err
	}

	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s/%s", ErrWorktreeNotFound, req.Project, req.Branch)
	}

	sessionName := req.Project + "-" + req.Branch
	running, err := s.tmux.HasSession(sessionName)
	if err != nil {
		return fmt.Errorf("checking tmux session: %w", err)
	}
	if running && !req.Force {
		return fmt.Errorf("%w: %s", ErrSessionRunning, sessionName)
	}
	if running && req.Force {
		if err := s.tmux.KillSession(sessionName); err != nil {
			return fmt.Errorf("killing session: %w", err)
		}
	}

	return s.git.Remove(cloneDir, worktreePath)
}
```

### Step 4: Run tests to verify they pass

```bash
go test ./internal/worktree/... -run "TestService_Delete" -v
```

Expected: all PASS.

### Step 5: Commit

```bash
git add internal/worktree/worktree.go internal/worktree/worktree_test.go
git commit -m "feat: implement Service.Delete for worktree removal"
```

---

## Task 6: `Service.WorktreePath` + `Service.Env`

**Files:**
- Modify: `internal/worktree/worktree.go` — add `WorktreePath` and `Env` methods
- Modify: `internal/worktree/worktree_test.go` — add Env + WorktreePath tests

### Step 1: Write the failing tests

```go
func TestService_WorktreePath_notCloned(t *testing.T) {
	svc, _ := makeService(t, &mockGit{}, &mockTmuxRunner{})
	_, err := svc.WorktreePath("myapp", "feature")
	if !errors.Is(err, ErrNotCloned) {
		t.Errorf("expected ErrNotCloned, got %v", err)
	}
}

func TestService_WorktreePath_worktreeNotFound(t *testing.T) {
	svc, tmpDir := makeService(t, &mockGit{}, &mockTmuxRunner{})
	os.MkdirAll(cloneDir(tmpDir), 0o755)
	// worktree dir does not exist

	_, err := svc.WorktreePath("myapp", "feature")
	if !errors.Is(err, ErrWorktreeNotFound) {
		t.Errorf("expected ErrWorktreeNotFound, got %v", err)
	}
}

func TestService_WorktreePath_ok(t *testing.T) {
	svc, tmpDir := makeService(t, &mockGit{}, &mockTmuxRunner{})
	worktreePath := cloneDir(tmpDir) + "__worktrees/feature"
	os.MkdirAll(cloneDir(tmpDir), 0o755)
	os.MkdirAll(worktreePath, 0o755)

	path, err := svc.WorktreePath("myapp", "feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != worktreePath {
		t.Errorf("path = %q, want %q", path, worktreePath)
	}
}

func TestService_Env_noTemplate(t *testing.T) {
	svc, tmpDir := makeService(t, &mockGit{}, &mockTmuxRunner{})
	worktreePath := cloneDir(tmpDir) + "__worktrees/feature"
	os.MkdirAll(worktreePath, 0o755)

	_, err := svc.Env("myapp", "feature", false)
	if err == nil {
		t.Fatal("expected error when no template found")
	}
}

func TestService_Env_repoLocalTemplate(t *testing.T) {
	svc, tmpDir := makeService(t, &mockGit{}, &mockTmuxRunner{})
	worktreePath := cloneDir(tmpDir) + "__worktrees/feature"
	os.MkdirAll(worktreePath, 0o755)
	// Write a repo-local .env.template
	os.WriteFile(filepath.Join(worktreePath, ".env.template"), []byte("PORT={{ port \"web\" }}\n"), 0o644)

	result, err := svc.Env("myapp", "feature", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Source != "repo-local" {
		t.Errorf("expected source repo-local, got %q", result.Source)
	}
	// .env file should be written
	if _, statErr := os.Stat(filepath.Join(worktreePath, ".env")); statErr != nil {
		t.Error("expected .env to be written")
	}
}

func TestService_Env_dryRun(t *testing.T) {
	svc, tmpDir := makeService(t, &mockGit{}, &mockTmuxRunner{})
	worktreePath := cloneDir(tmpDir) + "__worktrees/feature"
	os.MkdirAll(worktreePath, 0o755)
	os.WriteFile(filepath.Join(worktreePath, ".env.template"), []byte("X=1\n"), 0o644)

	result, err := svc.Env("myapp", "feature", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DryRun != true {
		t.Error("expected DryRun=true")
	}
	// .env file should NOT be written
	if _, statErr := os.Stat(filepath.Join(worktreePath, ".env")); statErr == nil {
		t.Error("expected .env NOT to be written in dry-run mode")
	}
}

func TestService_Env_configTemplate(t *testing.T) {
	tmpDir := t.TempDir()
	// Write template to a separate file
	templatePath := filepath.Join(tmpDir, "my.env.template")
	os.WriteFile(templatePath, []byte("DB_PORT={{ port \"db\" }}\n"), 0o644)

	cfg := &config.Config{
		Defaults: config.DefaultsConfig{ProjectsDir: tmpDir},
		Projects: map[string]config.ProjectConfig{
			"myapp": {
				Repo:        "git@github.com:user/myapp.git",
				EnvTemplate: templatePath,
			},
		},
	}
	tc := tmux.NewClient(&mockTmuxRunner{})
	svc := NewService(cfg, &mockGit{}, tc)

	worktreePath := filepath.Join(tmpDir, "github.com", "user", "myapp__worktrees", "feature")
	os.MkdirAll(worktreePath, 0o755)

	result, err := svc.Env("myapp", "feature", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Source != templatePath {
		t.Errorf("expected source %q, got %q", templatePath, result.Source)
	}
}
```

### Step 2: Run to verify they fail

```bash
go test ./internal/worktree/... -run "TestService_WorktreePath|TestService_Env"
```

Expected: FAIL — methods not defined.

### Step 3: Implement `WorktreePath` and `Env`

```go
// WorktreePath resolves the filesystem path for the given project+branch worktree,
// checking that both the clone and the worktree exist.
func (s *Service) WorktreePath(project, branch string) (string, error) {
	cloneDir, _, worktreePath, err := s.resolvePaths(project, branch)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(cloneDir); os.IsNotExist(err) {
		return "", fmt.Errorf("%w: %s", ErrNotCloned, project)
	}
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return "", fmt.Errorf("%w: %s/%s", ErrWorktreeNotFound, project, branch)
	}
	return worktreePath, nil
}

// Env processes the .env template for the given worktree and writes .env.
// If dryRun is true, the rendered content is returned without writing.
func (s *Service) Env(project, branch string, dryRun bool) (EnvResult, error) {
	p, ok := s.cfg.Projects[project]
	if !ok {
		return EnvResult{}, fmt.Errorf("project %q is not configured", project)
	}

	_, _, worktreePath, err := s.resolvePaths(project, branch)
	if err != nil {
		return EnvResult{}, err
	}
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		return EnvResult{}, fmt.Errorf("%w: %s/%s", ErrWorktreeNotFound, project, branch)
	}

	content, source, err := resolveTemplate(worktreePath, p)
	if err != nil {
		return EnvResult{}, fmt.Errorf("template error: %w", err)
	}
	if content == "" {
		return EnvResult{}, fmt.Errorf("no .env.template found for %s (checked repo and config)", project)
	}

	ctx := envtemplate.EnvTemplateContext{
		Project:      project,
		Branch:       branch,
		WorktreePath: worktreePath,
		SessionName:  project + "-" + branch,
	}
	rendered, err := envtemplate.Process(content, source, ctx)
	if err != nil {
		return EnvResult{}, fmt.Errorf("template error: %w", err)
	}

	if !dryRun {
		envPath := filepath.Join(worktreePath, ".env")
		if err := os.WriteFile(envPath, []byte(rendered), 0o644); err != nil {
			return EnvResult{}, fmt.Errorf("writing .env: %w", err)
		}
	}

	return EnvResult{Output: rendered, Source: source, DryRun: dryRun}, nil
}
```

### Step 4: Run tests to verify they pass

```bash
go test ./internal/worktree/... -v
```

Expected: all PASS.

### Step 5: Check coverage

```bash
go test ./internal/worktree/... ./internal/tmux/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep -E "worktree|tmux"
```

Expected: both packages well above 80%.

### Step 6: Commit

```bash
git add internal/worktree/worktree.go internal/worktree/worktree_test.go
git commit -m "feat: implement Service.WorktreePath and Service.Env"
```

---

## Task 7: `cmd/worktree.go` — all 5 subcommands

**Files:**
- Modify: `cmd/worktree.go` — replace stub with full implementation

No unit tests at the cmd layer — it's covered by service tests. Run full build + `make coverage` to verify.

### Step 1: Replace `cmd/worktree.go`

```go
// cmd/worktree.go
package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"syscall"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xico42/devenv/internal/tmux"
	"github.com/xico42/devenv/internal/worktree"
)

func newWorktreeService() *worktree.Service {
	return worktree.NewService(cfg, worktree.NewRealWorktreeRunner(), tmux.NewClient(tmux.NewRealRunner()))
}

var worktreeCmd = &cobra.Command{
	Use:   "worktree",
	Short: "Manage git worktrees for configured projects",
}

// ── list ─────────────────────────────────────────────────────────────────────

var worktreeListCmd = &cobra.Command{
	Use:   "list [project]",
	Short: "List worktrees (all projects, or a single project)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		project := ""
		if len(args) == 1 {
			project = args[0]
		}
		svc := newWorktreeService()
		entries, err := svc.List(project)
		if err != nil {
			return fmt.Errorf("list: %w", err)
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "PROJECT\tBRANCH\tPATH\tSESSION")
		for _, e := range entries {
			session := e.Session
			if session == "" {
				session = "--"
			}
			branch := e.Branch
			if branch == "" {
				branch = "(detached)"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Project, branch, e.Path, session)
		}
		return w.Flush()
	},
}

// ── new ──────────────────────────────────────────────────────────────────────

var worktreeNewCmd = &cobra.Command{
	Use:   "new <project> <branch>",
	Short: "Create a new worktree for a project",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, branch := args[0], args[1]
		fmt.Fprintf(cmd.OutOrStdout(), "Creating worktree %s/%s...  ", project, branch)

		svc := newWorktreeService()
		result, err := svc.New(project, branch)
		if err != nil {
			fmt.Fprintln(cmd.OutOrStdout())
			return worktreeErr(cmd, project, branch, err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "done")
		fmt.Fprintf(cmd.OutOrStdout(), "  Path: %s\n", result.Path)
		if result.EnvWritten {
			fmt.Fprintf(cmd.OutOrStdout(), "  Env:  %s/.env\n", result.Path)
		}
		return nil
	},
}

// ── delete ───────────────────────────────────────────────────────────────────

var worktreeForce bool

var worktreeDeleteCmd = &cobra.Command{
	Use:   "delete <project> <branch>",
	Short: "Delete a worktree",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, branch := args[0], args[1]

		if !worktreeForce {
			fmt.Fprintf(cmd.OutOrStdout(), "Delete worktree %s/%s? [y/N] ", project, branch)
			scanner := bufio.NewScanner(cmd.InOrStdin())
			scanner.Scan()
			if scanner.Text() != "y" && scanner.Text() != "Y" {
				fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
				return nil
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Deleting worktree %s/%s...  ", project, branch)
		svc := newWorktreeService()
		err := svc.Delete(worktree.DeleteRequest{
			Project: project,
			Branch:  branch,
			Force:   worktreeForce,
		})
		if err != nil {
			fmt.Fprintln(cmd.OutOrStdout())
			return worktreeErr(cmd, project, branch, err)
		}

		fmt.Fprintln(cmd.OutOrStdout(), "done")
		return nil
	},
}

// ── shell ─────────────────────────────────────────────────────────────────────

var worktreeShellCmd = &cobra.Command{
	Use:   "shell <project> <branch>",
	Short: "Open an interactive shell in a worktree",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, branch := args[0], args[1]
		svc := newWorktreeService()
		path, err := svc.WorktreePath(project, branch)
		if err != nil {
			return worktreeErr(cmd, project, branch, err)
		}

		shell := os.Getenv("SHELL")
		if shell == "" {
			shell = "/bin/sh"
		}

		if err := os.Chdir(path); err != nil {
			return fmt.Errorf("chdir %s: %w", path, err)
		}

		return syscall.Exec(shell, []string{shell}, os.Environ())
	},
}

// ── env ──────────────────────────────────────────────────────────────────────

var worktreeEnvDryRun bool

var worktreeEnvCmd = &cobra.Command{
	Use:   "env <project> <branch>",
	Short: "(Re)generate .env from template",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		project, branch := args[0], args[1]

		if !worktreeEnvDryRun {
			fmt.Fprintf(cmd.OutOrStdout(), "Processing .env.template...  ")
		}

		svc := newWorktreeService()
		result, err := svc.Env(project, branch, worktreeEnvDryRun)
		if err != nil {
			if !worktreeEnvDryRun {
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return worktreeErr(cmd, project, branch, err)
		}

		if worktreeEnvDryRun {
			fmt.Fprint(cmd.OutOrStdout(), result.Output)
			return nil
		}

		fmt.Fprintln(cmd.OutOrStdout(), "done")
		return nil
	},
}

// ── error helper ─────────────────────────────────────────────────────────────

func worktreeErr(cmd *cobra.Command, project, branch string, err error) error {
	switch {
	case errors.Is(err, worktree.ErrNotCloned):
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s is not cloned. Run 'devenv project clone %s' first.\n", project, project)
	case errors.Is(err, worktree.ErrWorktreeExists):
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: worktree %s/%s already exists.\n", project, branch)
	case errors.Is(err, worktree.ErrWorktreeNotFound):
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: worktree %s/%s not found. Run 'devenv worktree new %s %s' first.\n", project, branch, project, branch)
	case errors.Is(err, worktree.ErrSessionRunning):
		fmt.Fprintf(cmd.ErrOrStderr(), "Error: session %s-%s is running. Stop it first or use --force.\n", project, branch)
	default:
		return err
	}
	os.Exit(1)
	return nil
}

func init() {
	worktreeDeleteCmd.Flags().BoolVar(&worktreeForce, "force", false, "skip confirmation and kill any active session")
	worktreeEnvCmd.Flags().BoolVar(&worktreeEnvDryRun, "dry-run", false, "print generated .env without writing")

	worktreeCmd.AddCommand(worktreeListCmd)
	worktreeCmd.AddCommand(worktreeNewCmd)
	worktreeCmd.AddCommand(worktreeDeleteCmd)
	worktreeCmd.AddCommand(worktreeShellCmd)
	worktreeCmd.AddCommand(worktreeEnvCmd)
	rootCmd.AddCommand(worktreeCmd)
}
```

### Step 2: Build and verify it compiles

```bash
cd ~/.config/superpowers/worktrees/remote-dev/feat/cmd-worktree
make build
```

Expected: `./devenv` binary produced with no errors.

### Step 3: Run full test suite and coverage

```bash
make coverage
```

Expected: all tests PASS, aggregate coverage ≥ 80%.

### Step 4: Smoke test the help output

```bash
./devenv worktree --help
./devenv worktree list --help
./devenv worktree new --help
./devenv worktree delete --help
```

Expected: all subcommands listed with correct descriptions and flags.

### Step 5: Commit

```bash
git add cmd/worktree.go
git commit -m "feat: implement cmd/worktree with list, new, delete, shell, env subcommands"
```

---

## Final verification

```bash
make setup
```

Expected: deps → test → test-integration → lint → build all PASS.
