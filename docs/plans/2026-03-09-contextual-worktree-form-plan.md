# Contextual Worktree Creation Form — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make TUI `n` key contextual — pre-fill form from selected item, replace hand-rolled form with `huh.Form`, add `--from` flag to CLI, and support optional session attach after worktree creation.

**Architecture:** Upgrade huh to v2 first (isolated commit). Extend `WorktreeRunner` interface with `AddNewBranchFrom` for `--from` support. Replace TUI `formModel` with embedded `huh.Form`. Add `--from`, `--attach`, `--agent` flags to CLI.

**Tech Stack:** Go, charmbracelet/huh v2, charmbracelet/bubbletea v2, cobra

---

### Task 1: Upgrade huh to v2

**Files:**
- Modify: `go.mod`
- Modify: `cmd/config.go`

**Step 1: Upgrade the dependency**

Run:
```bash
cd /home/chico/dev/github.com/xico42/devenv__worktrees/worktree-form
go get github.com/charmbracelet/huh@v2.0.1
go mod tidy
```

**Step 2: Update huh import paths in `cmd/config.go`**

The v2 import path changes from `github.com/charmbracelet/huh` to `github.com/charmbracelet/huh/v2`. Update the import at line 11:

```go
// Before:
"github.com/charmbracelet/huh"

// After:
"github.com/charmbracelet/huh/v2"
```

**Step 3: Fix any v2 API changes**

Check for API changes in huh v2. The main change is that `huh.NewOption` constructor may differ. Review each usage site in `cmd/config.go`:

- Line 218-224: `huh.NewConfirm()` — likely unchanged
- Line 242-258: `huh.NewSelect[string]()`, `huh.NewOption()`, `huh.NewInput()` — check generics syntax
- Line 298-318: `huh.NewConfirm()`, `huh.NewInput()` — check EchoMode
- Line 361-387: `huh.NewSelect[string]()`, `huh.NewInput()`, `huh.NewForm()` — check group/field API

Use Context7 to verify exact v2 API if compilation fails.

**Step 4: Run tests**

Run: `make test`
Expected: All existing tests pass

**Step 5: Run lint**

Run: `make lint`
Expected: No lint errors

**Step 6: Commit**

```bash
git add go.mod go.sum cmd/config.go
git commit -m "chore: upgrade charmbracelet/huh to v2.0.1"
```

---

### Task 2: Add `AddNewBranchFrom` to `WorktreeRunner` interface

**Files:**
- Modify: `internal/worktree/worktree.go:54-60` (interface)
- Modify: `internal/worktree/worktree.go:78-86` (RealWorktreeRunner)
- Modify: `internal/worktree/worktree_test.go:80-101` (mockGit)
- Modify: `internal/worktree/worktree_test.go:427-442` (mockGitCreatesDir)

**Step 1: Write the failing test**

Add to `internal/worktree/worktree_test.go` after `TestService_New_branchNotFound_fallsBackToAddNew`:

```go
func TestService_New_withFromBranch(t *testing.T) {
	git := &mockGit{}
	svc, tmpDir := makeService(t, git, &mockTmuxRunner{})
	if err := os.MkdirAll(cloneDirPath(tmpDir), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := svc.NewFrom("myapp", "my-feature", "feature-auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !git.addNewFromCalled {
		t.Error("expected AddNewBranchFrom to be called")
	}
	if git.addNewFromStartPoint != "feature-auth" {
		t.Errorf("start point = %q, want %q", git.addNewFromStartPoint, "feature-auth")
	}
	expectedPath := cloneDirPath(tmpDir) + "__worktrees/my-feature"
	if result.Path != expectedPath {
		t.Errorf("path = %q, want %q", result.Path, expectedPath)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/worktree/... -run TestService_New_withFromBranch -v`
Expected: FAIL — `NewFrom` method does not exist

**Step 3: Add `AddNewBranchFrom` to the interface**

In `internal/worktree/worktree.go`, add to `WorktreeRunner` interface (line ~59):

```go
AddNewBranchFrom(cloneDir, worktreePath, branch, startPoint string) error
```

**Step 4: Implement on `RealWorktreeRunner`**

Add after `AddNewBranch` method (after line 86):

```go
func (r *RealWorktreeRunner) AddNewBranchFrom(cloneDir, worktreePath, branch, startPoint string) error {
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreePath, startPoint)
	cmd.Dir = cloneDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add -b (from): %w\n%s", err, out)
	}
	return nil
}
```

**Step 5: Update mock implementations**

In `mockGit` struct, add fields:

```go
addNewFromErr        error
addNewFromCalled     bool
addNewFromStartPoint string
```

Add method:

```go
func (m *mockGit) AddNewBranchFrom(cloneDir, worktreePath, branch, startPoint string) error {
	m.addNewFromCalled = true
	m.addNewFromStartPoint = startPoint
	return m.addNewFromErr
}
```

In `mockGitCreatesDir`, add method:

```go
func (m *mockGitCreatesDir) AddNewBranchFrom(cloneDir, worktreePath, branch, startPoint string) error {
	return os.MkdirAll(worktreePath, 0o755)
}
```

**Step 6: Add `NewFrom` method to `Service`**

Add after `New` method in `internal/worktree/worktree.go` (after line 231):

```go
// NewFrom creates a new git worktree branching from the given start point.
func (s *Service) NewFrom(project, branch, fromBranch string) (NewResult, error) {
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

	if err := s.git.AddNewBranchFrom(cloneDir, worktreePath, branch, fromBranch); err != nil {
		return NewResult{}, fmt.Errorf("creating worktree from %s: %w", fromBranch, err)
	}

	result := NewResult{Path: worktreePath}

	content, source, _ := resolveTemplate(worktreePath, p)
	if content != "" {
		ctx := envtemplate.EnvTemplateContext{
			Project:      project,
			Branch:       branch,
			WorktreePath: worktreePath,
			SessionName:  semconv.SessionName(project, branch),
		}
		if rendered, renderErr := envtemplate.Process(content, source, ctx); renderErr == nil {
			envPath := filepath.Join(worktreePath, ".env")
			if writeErr := os.WriteFile(envPath, []byte(rendered), 0o600); writeErr == nil {
				result.EnvWritten = true
			}
		}
	}

	return result, nil
}
```

**Step 7: Run test to verify it passes**

Run: `go test ./internal/worktree/... -run TestService_New_withFromBranch -v`
Expected: PASS

**Step 8: Add more tests for `NewFrom` edge cases**

Add to `internal/worktree/worktree_test.go`:

```go
func TestService_NewFrom_notCloned(t *testing.T) {
	svc, _ := makeService(t, &mockGit{}, &mockTmuxRunner{})
	_, err := svc.NewFrom("myapp", "feature", "main")
	if !errors.Is(err, ErrNotCloned) {
		t.Errorf("expected ErrNotCloned, got %v", err)
	}
}

func TestService_NewFrom_worktreeExists(t *testing.T) {
	svc, tmpDir := makeService(t, &mockGit{}, &mockTmuxRunner{})
	clone := cloneDirPath(tmpDir)
	if err := os.MkdirAll(clone, 0o755); err != nil {
		t.Fatal(err)
	}
	worktreePath := clone + "__worktrees/feature"
	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := svc.NewFrom("myapp", "feature", "main")
	if !errors.Is(err, ErrWorktreeExists) {
		t.Errorf("expected ErrWorktreeExists, got %v", err)
	}
}

func TestService_NewFrom_gitError(t *testing.T) {
	git := &mockGit{addNewFromErr: fmt.Errorf("invalid start point")}
	svc, tmpDir := makeService(t, git, &mockTmuxRunner{})
	if err := os.MkdirAll(cloneDirPath(tmpDir), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := svc.NewFrom("myapp", "feature", "nonexistent")
	if err == nil {
		t.Fatal("expected error when AddNewBranchFrom fails")
	}
}
```

**Step 9: Run all worktree tests**

Run: `go test ./internal/worktree/... -v`
Expected: All tests pass

**Step 10: Commit**

```bash
git add internal/worktree/worktree.go internal/worktree/worktree_test.go
git commit -m "feat: add NewFrom method for creating worktrees from a base branch"
```

---

### Task 3: Add `--from`, `--attach`, `--agent` flags to CLI

**Files:**
- Modify: `cmd/worktree.go:60-84` (worktreeNewCmd)
- Modify: `cmd/worktree.go:204-214` (init)
- Modify: `cmd/worktree_test.go`

**Step 1: Write the failing test for `--from` flag**

Add to `cmd/worktree_test.go`:

```go
func TestWorktreeNew_fromFlag_parsed(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	// No clone dir exists, so the command will fail with "not cloned".
	// We just verify the flag is accepted without "unknown flag" error.
	err := runCmd(t, "--config", cfgPath, "worktree", "new", "myapp", "feature", "--from", "main")
	if err == nil {
		t.Fatal("expected error (project not cloned), but no error")
	}
	// Should NOT be "unknown flag" error
	if err.Error() == `unknown flag: --from` {
		t.Fatal("--from flag not registered")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/... -run TestWorktreeNew_fromFlag_parsed -v`
Expected: FAIL — unknown flag: --from

**Step 3: Add flags to `worktreeNewCmd`**

In `cmd/worktree.go`, add flag variables near line 88:

```go
var (
	worktreeNewFrom   string
	worktreeNewAttach bool
	worktreeNewAgent  string
)
```

Update `init()` to register flags (before `worktreeCmd.AddCommand(worktreeNewCmd)`):

```go
worktreeNewCmd.Flags().StringVar(&worktreeNewFrom, "from", "", "base branch to create worktree from")
worktreeNewCmd.Flags().BoolVar(&worktreeNewAttach, "attach", false, "start a coding session after creation")
worktreeNewCmd.Flags().StringVar(&worktreeNewAgent, "agent", "", "agent to use for the session (with --attach)")
```

**Step 4: Update `worktreeNewCmd.RunE` to use `--from`**

Replace the `RunE` body of `worktreeNewCmd` (lines 66-83):

```go
RunE: func(cmd *cobra.Command, args []string) error {
	project, branch := args[0], args[1]
	fmt.Fprintf(cmd.OutOrStdout(), "Creating worktree %s/%s...  ", project, branch)

	svc := newWorktreeService()
	var result worktree.NewResult
	var err error
	if worktreeNewFrom != "" {
		result, err = svc.NewFrom(project, branch, worktreeNewFrom)
	} else {
		result, err = svc.New(project, branch)
	}
	if err != nil {
		fmt.Fprintln(cmd.OutOrStdout())
		return worktreeErr(cmd, project, branch, err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "done")
	fmt.Fprintf(cmd.OutOrStdout(), "  Path: %s\n", result.Path)
	if result.EnvWritten {
		fmt.Fprintf(cmd.OutOrStdout(), "  Env:  %s/.env\n", result.Path)
	}

	if !worktreeNewAttach {
		return nil
	}

	agentName := worktreeNewAgent
	agents := cfg.AgentNames()
	if len(agents) == 0 {
		return fmt.Errorf("no agents configured — add [agents.<name>] to config")
	}
	if agentName == "" {
		if len(agents) == 1 {
			agentName = agents[0]
		} else {
			return fmt.Errorf("multiple agents configured — specify one with --agent")
		}
	}

	agent, err := cfg.AgentByName(agentName)
	if err != nil {
		return err
	}

	sesSvc := newSessionService()
	agentCmd := buildAgentCmd(agent)
	if err := sesSvc.Start(session.StartRequest{
		Project: project,
		Branch:  branch,
		Path:    result.Path,
		Cmd:     agentCmd,
		Env:     agent.Env,
	}); err != nil {
		return fmt.Errorf("starting session: %w", err)
	}

	sessionName := semconv.SessionName(project, branch)
	fmt.Fprintf(cmd.OutOrStdout(), "  Session: %s\n", sessionName)

	return attachToSession(sessionName)
},
```

This requires importing `session` and `semconv` packages, and the `buildAgentCmd` helper (already in `internal/tui/agent_picker.go`). Move or duplicate `buildAgentCmd` — check if it's already accessible from `cmd/` or needs to be extracted.

Note: `buildAgentCmd` is in `internal/tui/agent_picker.go` (unexported package). Either:
- Duplicate the 3-line function in `cmd/worktree.go`
- Or move it to `internal/config/agent.go` as `AgentConfig.Command() string`

Preferred: Add `Command()` method to `AgentConfig` in `internal/config/agent.go`:

```go
// Command returns the full command string (cmd + args joined).
func (a AgentConfig) Command() string {
	if len(a.Args) == 0 {
		return a.Cmd
	}
	return a.Cmd + " " + strings.Join(a.Args, " ")
}
```

Then update `buildAgentCmd` in `internal/tui/agent_picker.go` to call `agent.Command()` instead, and use `agent.Command()` in the CLI.

Also need `newSessionService()` — check if it exists in `cmd/`. Look at `cmd/session.go` for the pattern:

```go
func newSessionService() *session.Service {
	return session.NewService(tmux.NewClient(tmux.NewRealRunner()))
}
```

And `attachToSession` — look at the existing `session attach` command for the `syscall.Exec` pattern. If it doesn't exist as a shared function, extract or create it:

```go
func attachToSession(sessionName string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", sessionName}, os.Environ())
}
```

**Step 5: Run test to verify it passes**

Run: `go test ./cmd/... -run TestWorktreeNew_fromFlag_parsed -v`
Expected: PASS

**Step 6: Add tests for `--attach` and `--agent` flags**

Add to `cmd/worktree_test.go`:

```go
func TestWorktreeNew_attachFlag_parsed(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	err := runCmd(t, "--config", cfgPath, "worktree", "new", "myapp", "feature", "--attach")
	if err == nil {
		t.Fatal("expected error (project not cloned), but no error")
	}
	if err.Error() == `unknown flag: --attach` {
		t.Fatal("--attach flag not registered")
	}
}

func TestWorktreeNew_agentFlag_parsed(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	err := runCmd(t, "--config", cfgPath, "worktree", "new", "myapp", "feature", "--attach", "--agent", "claude")
	if err == nil {
		t.Fatal("expected error (project not cloned), but no error")
	}
	if err.Error() == `unknown flag: --agent` {
		t.Fatal("--agent flag not registered")
	}
}
```

**Step 7: Run all cmd tests**

Run: `go test ./cmd/... -v`
Expected: All tests pass

**Step 8: Commit**

```bash
git add internal/config/agent.go internal/tui/agent_picker.go cmd/worktree.go cmd/worktree_test.go
git commit -m "feat: add --from, --attach, --agent flags to worktree new command"
```

---

### Task 4: Replace TUI form with huh-based form

**Files:**
- Rewrite: `internal/tui/form.go`
- Modify: `internal/tui/model.go:71` (form field type)
- Modify: `internal/tui/model.go:170-174` (worktreeCreatedMsg handler)

**Step 1: Define the new form model**

Rewrite `internal/tui/form.go`. The new `formModel` wraps a `huh.Form` with bound values:

```go
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/huh/v2"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/project"
	"github.com/xico42/devenv/internal/worktree"
)

type formModel struct {
	form *huh.Form

	// Bound values
	branch string
	attach bool
	agent  string

	// Context (read-only)
	project    string
	baseBranch string

	// Services
	wtSvc   *worktree.Service
	projSvc *project.Service
}

type formContext struct {
	project    string
	baseBranch string
}

func newFormModel(ctx formContext, cfg *config.Config, wtSvc *worktree.Service, projSvc *project.Service) *formModel {
	m := &formModel{
		project:    ctx.project,
		baseBranch: ctx.baseBranch,
		attach:     true, // default yes
		wtSvc:      wtSvc,
		projSvc:    projSvc,
	}

	agents := cfg.AgentNames()

	// Group 1: worktree details (always visible)
	group1 := huh.NewGroup(
		huh.NewNote().
			Title("New Worktree").
			Description(fmt.Sprintf("Project: %s\nBase: %s", ctx.project, ctx.baseBranch)),
		huh.NewInput().
			Title("Branch name").
			Placeholder("feature-name").
			Value(&m.branch).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("branch name required")
				}
				return nil
			}),
		huh.NewConfirm().
			Title("Attach coding session?").
			Value(&m.attach),
	)

	groups := []*huh.Group{group1}

	// Group 2: agent selection (conditional)
	if len(agents) > 1 {
		var agentOpts []huh.Option[string]
		for _, name := range agents {
			agentOpts = append(agentOpts, huh.NewOption(name, name))
		}
		if len(agents) > 0 {
			m.agent = agents[0]
		}

		group2 := huh.NewGroup(
			huh.NewSelect[string]().
				Title("Agent").
				Options(agentOpts...).
				Value(&m.agent),
		).WithHideFunc(func() bool {
			return !m.attach
		})
		groups = append(groups, group2)
	} else if len(agents) == 1 {
		m.agent = agents[0]
	}

	m.form = huh.NewForm(groups...)
	return m
}

func (f *formModel) Init() tea.Cmd {
	return f.form.Init()
}

func (f *formModel) Update(msg tea.Msg) (*formModel, tea.Cmd) {
	form, cmd := f.form.Update(msg)
	if ff, ok := form.(*huh.Form); ok {
		f.form = ff
	}
	return f, cmd
}

func (f *formModel) View() string {
	return f.form.View()
}

func (f *formModel) completed() bool {
	return f.form.State == huh.StateCompleted
}

func (f *formModel) submit() tea.Cmd {
	branch := strings.TrimSpace(f.branch)
	project := f.project
	baseBranch := f.baseBranch
	attach := f.attach
	agent := f.agent
	wtSvc := f.wtSvc
	projSvc := f.projSvc

	return func() tea.Msg {
		if projSvc != nil {
			_ = projSvc.Clone(project)
		}

		var result worktree.NewResult
		var err error
		if baseBranch != "" {
			result, err = wtSvc.NewFrom(project, branch, baseBranch)
		} else {
			result, err = wtSvc.New(project, branch)
		}
		if err != nil {
			return errMsg{err: err}
		}

		return worktreeCreatedMsg{
			project: project,
			branch:  branch,
			path:    result.Path,
			attach:  attach,
			agent:   agent,
		}
	}
}

// showForm transitions the model to the form screen.
func (m Model) showForm() (tea.Model, tea.Cmd) {
	sel := m.selectedItem()
	if sel == nil {
		return m, nil
	}

	var ctx formContext
	switch sel.Group {
	case groupProject:
		ctx.project = sel.Project
		if p, ok := m.cfg.Projects[sel.Project]; ok && p.DefaultBranch != "" {
			ctx.baseBranch = p.DefaultBranch
		} else {
			ctx.baseBranch = "main"
		}
	case groupWorktree, groupAgent:
		ctx.project = sel.Project
		ctx.baseBranch = sel.Branch
	}

	m.form = newFormModel(ctx, m.cfg, m.wtSvc, m.projSvc)
	m.screen = screenForm
	return m, m.form.Init()
}

// updateForm handles messages while on the form screen.
func (m Model) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Check for escape to return to list.
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if keyMsg.String() == "esc" {
			m.screen = screenList
			m.form = nil
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.form, cmd = m.form.Update(msg)

	// Check if form completed
	if m.form.completed() {
		return m, m.form.submit()
	}

	return m, cmd
}
```

**Step 2: Update `worktreeCreatedMsg` to carry attach info**

In `internal/tui/model.go`, update the message struct (lines 39-43):

```go
type worktreeCreatedMsg struct {
	project string
	branch  string
	path    string
	attach  bool
	agent   string
}
```

**Step 3: Update `worktreeCreatedMsg` handler in `Model.Update`**

Replace the handler at lines 170-174 in `internal/tui/model.go`:

```go
case worktreeCreatedMsg:
	m.statusMsg = fmt.Sprintf("Created %s/%s", msg.project, msg.branch)
	m.screen = screenList
	m.form = nil
	if msg.attach && msg.agent != "" {
		return m, m.startSessionAfterCreate(msg)
	}
	return m, m.refreshCmd()
```

**Step 4: Add `startSessionAfterCreate` method**

Add to `internal/tui/actions.go`:

```go
func (m Model) startSessionAfterCreate(msg worktreeCreatedMsg) tea.Cmd {
	cfg := m.cfg
	sesSvc := m.sesSvc

	return func() tea.Msg {
		agent, err := cfg.AgentByName(msg.agent)
		if err != nil {
			return errMsg{err: err}
		}
		agentCmd := agent.Command()
		err = sesSvc.Start(session.StartRequest{
			Project: msg.project,
			Branch:  msg.branch,
			Path:    msg.path,
			Cmd:     agentCmd,
			Env:     agent.Env,
		})
		if err != nil {
			return errMsg{err: err}
		}
		return attachMsg{session: semconv.SessionName(msg.project, msg.branch)}
	}
}
```

**Step 5: Update `Model` struct field type**

In `internal/tui/model.go` line 71, change:

```go
// Before:
form *formModel

// After (same type name, but the underlying struct has changed):
form *formModel
```

No change needed — the field name and type name are the same. The `formModel` struct is being replaced in `form.go`.

**Step 6: Run tests**

Run: `make test`
Expected: All tests pass

**Step 7: Run lint**

Run: `make lint`
Expected: No lint errors

**Step 8: Commit**

```bash
git add internal/tui/form.go internal/tui/model.go internal/tui/actions.go
git commit -m "feat: replace TUI form with huh-based contextual worktree creation"
```

---

### Task 5: Update `buildAgentCmd` references to use `AgentConfig.Command()`

**Files:**
- Modify: `internal/tui/agent_picker.go:36-41`
- Modify: `internal/tui/actions.go` (all `buildAgentCmd` calls)

**Step 1: Update `agent_picker.go` to use `agent.Command()`**

Remove the `buildAgentCmd` function from `internal/tui/agent_picker.go` (lines 36-41).

Replace all `buildAgentCmd(agent)` calls in `internal/tui/agent_picker.go` and `internal/tui/actions.go` with `agent.Command()`.

In `agent_picker.go` line 95:
```go
// Before:
agentCmd := buildAgentCmd(agent)
// After:
agentCmd := agent.Command()
```

In `actions.go`, replace all occurrences:
- Line 50: `agentCmd := buildAgentCmd(agent)` → `agentCmd := agent.Command()`
- Line 86: `agentCmd := buildAgentCmd(agent)` → `agentCmd := agent.Command()`

**Step 2: Add test for `AgentConfig.Command()`**

Add to `internal/config/agent_test.go` (create if needed):

```go
package config

import "testing"

func TestAgentConfig_Command(t *testing.T) {
	tests := []struct {
		name string
		cfg  AgentConfig
		want string
	}{
		{"cmd only", AgentConfig{Cmd: "claude"}, "claude"},
		{"cmd with args", AgentConfig{Cmd: "claude", Args: []string{"--model", "opus"}}, "claude --model opus"},
		{"empty", AgentConfig{}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.cfg.Command()
			if got != tc.want {
				t.Errorf("Command() = %q, want %q", got, tc.want)
			}
		})
	}
}
```

**Step 3: Run tests**

Run: `make test`
Expected: All tests pass

**Step 4: Commit**

```bash
git add internal/config/agent.go internal/config/agent_test.go internal/tui/agent_picker.go internal/tui/actions.go
git commit -m "refactor: extract AgentConfig.Command() method"
```

---

### Task 6: Final verification

**Step 1: Run full test suite**

Run: `make test`
Expected: All tests pass

**Step 2: Run integration tests**

Run: `make test-integration`
Expected: All integration tests pass

**Step 3: Run lint**

Run: `make lint`
Expected: Clean

**Step 4: Run coverage check**

Run: `make coverage`
Expected: Coverage >= 80%

**Step 5: Build**

Run: `make build`
Expected: Binary builds successfully

**Step 6: Manual smoke test (if possible)**

Run the TUI and verify:
1. Select a project item, press `n` — form shows project name and default branch
2. Select a worktree item, press `n` — form shows project name and worktree's branch as base
3. Fill branch name, toggle attach, select agent, submit
4. Worktree is created and session attaches (if attach=yes)

Run the CLI:
```bash
./devenv worktree new myproject my-feature --from some-branch
./devenv worktree new myproject my-feature --attach --agent claude
```
