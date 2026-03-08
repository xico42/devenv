# TUI Delete Redesign Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace session state files with tmux `@` options and redesign the delete confirmation as a navigable choice list.

**Architecture:** Tmux user-defined options (`@devenv_*`) become the single source of truth for agent session status. The confirm dialog changes from a text input to a navigable list of contextual delete choices. Main worktree deletion is blocked.

**Tech Stack:** Go, Bubble Tea (charm.land/bubbletea/v2), tmux `@` options

---

### Task 1: Add tmux option constants to semconv

**Files:**
- Modify: `internal/semconv/semconv.go:8-11`
- Test: `internal/semconv/semconv_test.go`

**Step 1: Write the test**

Add to `internal/semconv/semconv_test.go`:

```go
func TestTmuxOptionConstants(t *testing.T) {
	// Verify constants have @ prefix (required for tmux user options).
	for _, opt := range []string{
		semconv.TmuxOptionStatus,
		semconv.TmuxOptionQuestion,
		semconv.TmuxOptionStartedAt,
	} {
		if !strings.HasPrefix(opt, "@") {
			t.Errorf("tmux option %q must start with @", opt)
		}
	}
}
```

You'll need to add `"strings"` to the test imports.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/semconv/... -run TestTmuxOptionConstants -v`
Expected: FAIL — undefined constants

**Step 3: Add the constants**

Add to the `const` block in `internal/semconv/semconv.go`:

```go
const (
	SessionEnvVar   = "DEVENV_SESSION"
	DefaultAgentCmd = "claude"

	TmuxOptionStatus    = "@devenv_status"
	TmuxOptionQuestion  = "@devenv_question"
	TmuxOptionStartedAt = "@devenv_started_at"
)
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/semconv/... -run TestTmuxOptionConstants -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/semconv/semconv.go internal/semconv/semconv_test.go
git commit -m "feat: add tmux option constants to semconv"
```

---

### Task 2: Add `GetOption` and `SetOption` to tmux Client

**Files:**
- Modify: `internal/tmux/client.go`
- Test: `internal/tmux/client_test.go`

**Step 1: Write the failing tests**

Add to `internal/tmux/client_test.go`:

```go
func TestGetOption(t *testing.T) {
	mr := &mockRunner{stdout: "running\n"}
	c := NewClient(mr)

	val, err := c.GetOption("myapp-feat", "@devenv_status")
	if err != nil {
		t.Fatalf("GetOption() error = %v", err)
	}
	if val != "running" {
		t.Errorf("GetOption() = %q, want %q", val, "running")
	}
	wantArgs := []string{"show-option", "-t", "myapp-feat", "-v", "@devenv_status"}
	if !slices.Equal(mr.lastArgs, wantArgs) {
		t.Errorf("args = %v, want %v", mr.lastArgs, wantArgs)
	}
}

func TestGetOption_notSet(t *testing.T) {
	mr := &mockRunner{stderr: "unknown option", exitCode: 1}
	c := NewClient(mr)

	val, err := c.GetOption("myapp-feat", "@devenv_status")
	if err != nil {
		t.Fatalf("GetOption() error = %v", err)
	}
	if val != "" {
		t.Errorf("GetOption() = %q, want empty string for unset option", val)
	}
}

func TestGetOption_runnerError(t *testing.T) {
	mr := &mockRunner{err: errors.New("boom")}
	c := NewClient(mr)

	_, err := c.GetOption("myapp-feat", "@devenv_status")
	if err == nil {
		t.Error("GetOption() should return error when runner fails")
	}
}

func TestSetOption(t *testing.T) {
	mr := &mockRunner{}
	c := NewClient(mr)

	err := c.SetOption("myapp-feat", "@devenv_status", "running")
	if err != nil {
		t.Fatalf("SetOption() error = %v", err)
	}
	wantArgs := []string{"set-option", "-t", "myapp-feat", "@devenv_status", "running"}
	if !slices.Equal(mr.lastArgs, wantArgs) {
		t.Errorf("args = %v, want %v", mr.lastArgs, wantArgs)
	}
}

func TestSetOption_error(t *testing.T) {
	mr := &mockRunner{stderr: "no such session", exitCode: 1}
	c := NewClient(mr)

	err := c.SetOption("myapp-feat", "@devenv_status", "running")
	if err == nil {
		t.Error("SetOption() should return error on non-zero exit")
	}
}
```

You'll need to add `"errors"` and `"slices"` to the test imports.

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tmux/... -run "TestGetOption|TestSetOption" -v`
Expected: FAIL — methods don't exist

**Step 3: Implement the methods**

Add to `internal/tmux/client.go`:

```go
// GetOption reads a tmux user-defined option from a session.
// Returns empty string (no error) when the option is not set (tmux exits 1).
func (c *Client) GetOption(session, option string) (string, error) {
	stdout, _, code, err := c.runner.Run("show-option", "-t", session, "-v", option)
	if err != nil {
		return "", fmt.Errorf("tmux show-option: %w", err)
	}
	if code != 0 {
		return "", nil // option not set
	}
	return strings.TrimSpace(stdout), nil
}

// SetOption sets a tmux user-defined option on a session.
func (c *Client) SetOption(session, option, value string) error {
	_, stderr, code, err := c.runner.Run("set-option", "-t", session, option, value)
	if err != nil {
		return fmt.Errorf("tmux set-option: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("tmux set-option: %s", strings.TrimSpace(stderr))
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tmux/... -run "TestGetOption|TestSetOption" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tmux/client.go internal/tmux/client_test.go
git commit -m "feat: add GetOption and SetOption to tmux Client"
```

---

### Task 3: Update session Service to use tmux options instead of state files

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/session_test.go`

This is the core migration. `Service` drops `sessionsDir`, uses `tmux.Client.SetOption`/`GetOption` instead of `state.SaveSession`/`LoadSession`/`ClearSession`.

**Step 1: Update `Service` struct and constructor**

In `internal/session/session.go`, remove `sessionsDir` from the struct and constructor:

```go
type Service struct {
	tmux *tmux.Client
}

func NewService(tmux *tmux.Client) *Service {
	return &Service{tmux: tmux}
}
```

**Step 2: Update `Start` method**

Replace `state.SaveSession` with `tmux.SetOption` calls. After `NewSessionWithEnv`, add:

```go
	now := time.Now().UTC()
	_ = s.tmux.SetOption(name, semconv.TmuxOptionStatus, state.SessionRunning)
	_ = s.tmux.SetOption(name, semconv.TmuxOptionStartedAt, now.Format(time.RFC3339))
```

Remove the `state.SaveSession` block entirely.

**Step 3: Update `List` method**

Replace `state.LoadSession` with `tmux.GetOption`:

```go
func (s *Service) List() ([]SessionInfo, error) {
	names, err := s.tmux.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("listing tmux sessions: %w", err)
	}

	var result []SessionInfo
	for _, name := range names {
		info := SessionInfo{Name: name}
		info.Status, _ = s.tmux.GetOption(name, semconv.TmuxOptionStatus)
		info.Question, _ = s.tmux.GetOption(name, semconv.TmuxOptionQuestion)
		if ts, _ := s.tmux.GetOption(name, semconv.TmuxOptionStartedAt); ts != "" {
			info.StartedAt, _ = time.Parse(time.RFC3339, ts)
		}
		result = append(result, info)
	}
	return result, nil
}
```

Note: `List` no longer populates `Project`/`Branch`/`UpdatedAt` because those were derived from state files. Callers that need project/branch can derive them from the session name or the TUI's existing worktree data.

**Step 4: Update `Show` method**

Same pattern as `List` but for a single session:

```go
func (s *Service) Show(name string) (*SessionInfo, error) {
	exists, err := s.tmux.HasSession(name)
	if err != nil {
		return nil, fmt.Errorf("checking session: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrSessionNotFound, name)
	}

	info := &SessionInfo{Name: name}
	info.Status, _ = s.tmux.GetOption(name, semconv.TmuxOptionStatus)
	info.Question, _ = s.tmux.GetOption(name, semconv.TmuxOptionQuestion)
	if ts, _ := s.tmux.GetOption(name, semconv.TmuxOptionStartedAt); ts != "" {
		info.StartedAt, _ = time.Parse(time.RFC3339, ts)
	}
	return info, nil
}
```

**Step 5: Update `Stop` method**

Remove `state.ClearSession`:

```go
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
	return nil
}
```

**Step 6: Update `MarkRunning` method**

Replace `state.LoadSession`/`state.SaveSession` with `tmux.SetOption`:

```go
func (s *Service) MarkRunning(name string) error {
	if name == "" {
		return nil
	}
	_ = s.tmux.SetOption(name, semconv.TmuxOptionStatus, state.SessionRunning)
	_ = s.tmux.SetOption(name, semconv.TmuxOptionQuestion, "")
	return nil
}
```

**Step 7: Remove `state` import**

Remove `"github.com/xico42/devenv/internal/state"` from the import block. Replace `state.SessionRunning` with a local constant or keep using the state package for just the constant values. Better: move the constants to semconv to avoid importing state.

Actually — add status constants to semconv:

In `internal/semconv/semconv.go`:

```go
const (
	StatusRunning = "running"
	StatusWaiting = "waiting"
)
```

Then replace `state.SessionRunning` with `semconv.StatusRunning` throughout.

**Step 8: Update tests**

The test helper `newService` currently returns `(*session.Service, string)` where the string is `sessionsDir`. Update to just return `*session.Service`:

```go
func newService(t *testing.T, r *mockRunner) *session.Service {
	t.Helper()
	tc := tmux.NewClient(r)
	return session.NewService(tc)
}
```

Update all test cases:
- Remove state file assertions (file existence checks)
- For `Start` tests: mock runner needs to handle the `SetOption` calls that happen after `NewSessionWithEnv` — use `mockRunnerSequence` with additional responses for the set-option calls
- For `List`/`Show` tests: mock runner returns `GetOption` responses
- For `Stop` tests: remove state file cleanup assertions
- For `MarkRunning` tests: mock runner should accept `set-option` calls

**Step 9: Run all session tests**

Run: `go test ./internal/session/... -v`
Expected: PASS

**Step 10: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go internal/semconv/semconv.go
git commit -m "refactor: replace session state files with tmux @ options"
```

---

### Task 4: Update cmd/session.go for new Service constructor

**Files:**
- Modify: `cmd/session.go`
- Modify: `cmd/session_internal_test.go`

**Step 1: Update `sessionsDir` and `newSessionService`**

In `cmd/session.go`:
- Remove `sessionsDir()` function
- Update `newSessionService()`:

```go
func newSessionService() *session.Service {
	return session.NewService(tmux.NewClient(tmux.RealRunner{}))
}
```

**Step 2: Update `sessionListCmd`**

The `List()` method no longer returns `Project`/`Branch` from state files. The session name encodes project-branch, so display can use the name directly. Update the table output accordingly:

```go
fmt.Fprintln(w, "SESSION\tSTATUS")
for _, s := range sessions {
    fmt.Fprintf(w, "%s\t%s\n", s.Name, s.Status)
}
```

Or keep the current columns but accept that Project/Branch will be empty (they weren't reliable before either when state files were missing).

**Step 3: Update `sessionShowCmd`**

Same adjustment — display whatever `SessionInfo` has.

**Step 4: Update `sessionMarkRunningCmd`**

No changes needed — it already just calls `svc.MarkRunning(name)`.

**Step 5: Remove `state.ClearSession` calls from worktree delete**

In `cmd/worktree.go`, the delete handler may reference `state.ClearSession`. Remove any such calls.

**Step 6: Run tests**

Run: `go test ./cmd/... -v`
Expected: PASS

**Step 7: Commit**

```bash
git add cmd/session.go cmd/session_internal_test.go cmd/worktree.go
git commit -m "refactor: update session commands for tmux-based state"
```

---

### Task 5: Delete `internal/state/session.go`

**Files:**
- Delete: `internal/state/session.go`
- Delete: `internal/state/session_test.go`

**Step 1: Check for remaining references**

Run: `grep -r "state\.LoadSession\|state\.SaveSession\|state\.ClearSession\|state\.ListSessions\|state\.SessionState\|state\.SessionRunning\|state\.SessionWaiting" --include="*.go" .`

There should be no remaining references after Tasks 3 and 4. If any remain, fix them first.

**Step 2: Delete the files**

```bash
rm internal/state/session.go internal/state/session_test.go
```

**Step 3: Run full test suite**

Run: `go test ./... -count=1`
Expected: PASS — no references to deleted code remain

**Step 4: Commit**

```bash
git add -A internal/state/
git commit -m "refactor: remove session state files (replaced by tmux options)"
```

---

### Task 6: Add `IsMainWorktree` Item field and populate it

**Files:**
- Modify: `internal/tui/items.go`
- Modify: `internal/tui/model.go`

The main worktree is identified by its path equaling the clone dir (`semconv.CloneDir(projectsDir, repoPath)`).

**Step 1: Add `IsMain` field to Item**

In `internal/tui/items.go`, add to the `Item` struct:

```go
type Item struct {
	Project     string
	Branch      string
	Path        string
	Group       int
	HasAgent    bool
	AgentStatus string
	Question    string
	HasShell    bool
	Cloned      bool
	IsMain      bool // true for the main worktree (clone dir)
}
```

**Step 2: Add `cloneDirs` to `refreshResult`**

```go
type refreshResult struct {
	worktrees     []wtEntry
	agentSessions map[string]agentInfo
	shellSessions map[string]bool
	projects      []projEntry
	cloneDirs     map[string]string // project name -> clone dir path
}
```

**Step 3: Populate `cloneDirs` in `refreshCmd`**

In `model.go:refreshCmd`, after building the projects list, build `cloneDirs`:

```go
data.cloneDirs = make(map[string]string)
if cfg != nil {
	for name := range cfg.Projects {
		p := cfg.Projects[name]
		if rp, err := config.RepoPath(p.Repo); err == nil {
			data.cloneDirs[name] = semconv.CloneDir(cfg.Defaults.ProjectsDir, rp)
		}
	}
}
```

**Step 4: Set `IsMain` in `buildItems`**

In the worktree loop of `buildItems`:

```go
item.IsMain = data.cloneDirs[wt.project] == wt.path
```

**Step 5: Run tests**

Run: `go test ./internal/tui/... -v`
Expected: PASS

**Step 6: Commit**

```bash
git add internal/tui/items.go internal/tui/model.go
git commit -m "feat: add IsMain field to TUI items for main worktree detection"
```

---

### Task 7: Migrate TUI refresh from state files to tmux queries

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

**Step 1: Remove `sessionsDir` from `Model`**

In `model.go`, remove `sessionsDir string` from the `Model` struct. Update `NewModel` to drop the `sessionsDir` parameter.

**Step 2: Update `refreshCmd` to query tmux for agent status**

Replace the "Session states" block (lines 307-318) with tmux queries:

```go
// 2. Agent sessions (query tmux for status)
if tmuxClient != nil {
	names, err := tmuxClient.ListSessions()
	if err == nil {
		for _, name := range names {
			// Skip shell sessions.
			if strings.HasSuffix(name, "~sh") {
				data.shellSessions[name] = true
				continue
			}
			status, _ := tmuxClient.GetOption(name, semconv.TmuxOptionStatus)
			question, _ := tmuxClient.GetOption(name, semconv.TmuxOptionQuestion)
			data.agentSessions[name] = agentInfo{
				status:   status,
				question: question,
			}
		}
	}
}
```

Remove the separate "Tmux sessions (for shell session detection)" block — it's now merged above.

Remove the `state` import from `model.go`.

**Step 3: Update tests**

- Remove `TestModel_refreshCmd_withSessionsDir` — no longer relevant
- Update `TestNewModel` — remove `sessionsDir` argument

**Step 4: Run tests**

Run: `go test ./internal/tui/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "refactor: TUI refresh queries tmux directly instead of state files"
```

---

### Task 8: Redesign confirm dialog as navigable choice list

**Files:**
- Modify: `internal/tui/confirm.go`
- Modify: `internal/tui/actions_test.go`

**Step 1: Write failing tests for the new `confirmModel`**

Replace the existing confirm tests in `actions_test.go` with:

```go
func TestNewConfirmModel_noSessions(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	c := newConfirmModel(target)
	if len(c.choices) != 2 {
		t.Errorf("choices = %d, want 2 (delete worktree + cancel)", len(c.choices))
	}
	if c.choices[0].action != deleteAll {
		t.Errorf("first choice = %v, want deleteAll", c.choices[0].action)
	}
	if c.choices[1].action != deleteCancel {
		t.Errorf("last choice = %v, want deleteCancel", c.choices[1].action)
	}
}

func TestNewConfirmModel_agentOnly(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupAgent, HasAgent: true, AgentStatus: "running"}
	c := newConfirmModel(target)
	if len(c.choices) != 3 {
		t.Errorf("choices = %d, want 3", len(c.choices))
	}
	if c.choices[0].action != deleteAll {
		t.Errorf("first = %v, want deleteAll", c.choices[0].action)
	}
	if c.choices[1].action != deleteAgent {
		t.Errorf("second = %v, want deleteAgent", c.choices[1].action)
	}
}

func TestNewConfirmModel_bothSessions(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupAgent, HasAgent: true, HasShell: true, AgentStatus: "running"}
	c := newConfirmModel(target)
	if len(c.choices) != 4 {
		t.Errorf("choices = %d, want 4", len(c.choices))
	}
}

func TestConfirmModel_navigation(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	c := newConfirmModel(target)
	if c.cursor != 0 {
		t.Errorf("initial cursor = %d, want 0", c.cursor)
	}

	// j moves down
	c, _ = c.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if c.cursor != 1 {
		t.Errorf("cursor after j = %d, want 1", c.cursor)
	}

	// k moves back up
	c, _ = c.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
	if c.cursor != 0 {
		t.Errorf("cursor after k = %d, want 0", c.cursor)
	}

	// k at top stays at 0
	c, _ = c.Update(tea.KeyPressMsg(tea.Key{Code: 'k', Text: "k"}))
	if c.cursor != 0 {
		t.Errorf("cursor after k at top = %d, want 0", c.cursor)
	}
}

func TestConfirmModel_selection(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	c := newConfirmModel(target)
	if c.selected() != deleteAll {
		t.Errorf("selected() = %v, want deleteAll", c.selected())
	}

	// Move to cancel
	c, _ = c.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if c.selected() != deleteCancel {
		t.Errorf("selected() = %v, want deleteCancel", c.selected())
	}
}

func TestConfirmModel_View(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupAgent, HasAgent: true, AgentStatus: "running"}
	c := newConfirmModel(target)
	out := stripANSI(c.View())
	if !strings.Contains(out, "myapp") {
		t.Errorf("View() should contain project name: %q", out)
	}
	if !strings.Contains(out, "Agent session") {
		t.Errorf("View() should mention active agent: %q", out)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/... -run "TestNewConfirmModel|TestConfirmModel_navigation|TestConfirmModel_selection|TestConfirmModel_View" -v`
Expected: FAIL

**Step 3: Rewrite `confirm.go`**

Replace `internal/tui/confirm.go` entirely:

```go
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type deleteAction int

const (
	deleteAll    deleteAction = iota // worktree + all sessions
	deleteAgent                      // agent session only
	deleteShell                      // shell session only
	deleteCancel                     // cancel
)

type choice struct {
	label  string
	action deleteAction
}

type confirmModel struct {
	target  Item
	choices []choice
	cursor  int
}

func newConfirmModel(target Item) *confirmModel {
	var choices []choice

	hasAgent := target.HasAgent
	hasShell := target.HasShell

	// "Delete everything" label varies by what's active.
	switch {
	case hasAgent && hasShell:
		choices = append(choices, choice{"Delete everything (worktree + all sessions)", deleteAll})
		choices = append(choices, choice{"Delete agent session only", deleteAgent})
		choices = append(choices, choice{"Delete shell session only", deleteShell})
	case hasAgent:
		choices = append(choices, choice{"Delete everything (worktree + agent session)", deleteAll})
		choices = append(choices, choice{"Delete agent session only", deleteAgent})
	case hasShell:
		choices = append(choices, choice{"Delete everything (worktree + shell session)", deleteAll})
		choices = append(choices, choice{"Delete shell session only", deleteShell})
	default:
		choices = append(choices, choice{"Delete worktree", deleteAll})
	}
	choices = append(choices, choice{"Cancel", deleteCancel})

	return &confirmModel{target: target, choices: choices}
}

func (c *confirmModel) selected() deleteAction {
	return c.choices[c.cursor].action
}

func (c *confirmModel) Update(msg tea.Msg) (*confirmModel, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch kp.String() {
	case "j", "down":
		if c.cursor < len(c.choices)-1 {
			c.cursor++
		}
	case "k", "up":
		if c.cursor > 0 {
			c.cursor--
		}
	}
	return c, nil
}

func (c *confirmModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true)
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	selectedStyle := lipgloss.NewStyle().Bold(true)

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n────\n", titleStyle.Render(fmt.Sprintf("Delete — %s/%s", c.target.Project, c.target.Branch)))

	// Active sessions warning
	if c.target.HasAgent || c.target.HasShell {
		b.WriteString(warnStyle.Render("  ⚠ Active sessions detected:"))
		b.WriteString("\n")
		if c.target.HasAgent {
			status := c.target.AgentStatus
			if status == "" {
				status = "active"
			}
			fmt.Fprintf(&b, "    • Agent session (%s)\n", status)
		}
		if c.target.HasShell {
			b.WriteString("    • Shell session (running)\n")
		}
		b.WriteString("\n")
	}

	// Choices
	for i, ch := range c.choices {
		if i == c.cursor {
			fmt.Fprintf(&b, "  > %s\n", selectedStyle.Render(ch.label))
		} else {
			fmt.Fprintf(&b, "    %s\n", ch.label)
		}
	}

	b.WriteString("────\nEnter: select  |  Esc: cancel")
	return b.String()
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... -run "TestNewConfirmModel|TestConfirmModel_navigation|TestConfirmModel_selection|TestConfirmModel_View" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/tui/confirm.go internal/tui/actions_test.go
git commit -m "feat: redesign confirm dialog as navigable choice list"
```

---

### Task 9: Update delete actions to handle granular choices

**Files:**
- Modify: `internal/tui/actions.go`
- Modify: `internal/tui/actions_test.go`

**Step 1: Write failing tests for new behavior**

Add to `actions_test.go`:

```go
func TestStartDelete_mainWorktree(t *testing.T) {
	items := []Item{{Project: "myapp", Branch: "main", Group: groupWorktree, IsMain: true}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList}
	m.list = newList(listItems)

	updated, _ := m.startDelete()
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d (should stay on list for main worktree)", um.screen, screenList)
	}
	if um.statusMsg == "" {
		t.Error("statusMsg should be set when trying to delete main worktree")
	}
	if !strings.Contains(um.statusMsg, "main worktree") {
		t.Errorf("statusMsg = %q, should mention main worktree", um.statusMsg)
	}
}
```

**Step 2: Update `startDelete` to block main worktree**

In `actions.go`, add after the project check:

```go
if sel.IsMain {
	m.statusMsg = "Cannot delete the main worktree"
	return m, nil
}
```

**Step 3: Update `updateConfirmDelete`**

Replace the current implementation. The key handling now routes Enter to the appropriate action based on `c.selected()`, and handles j/k/up/down/q navigation:

```go
func (m Model) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch kp.String() {
	case "esc", "q":
		return m.confirmDeleteNo()
	case "enter":
		switch m.confirm.selected() {
		case deleteCancel:
			return m.confirmDeleteNo()
		case deleteAll:
			return m.confirmDeleteAll()
		case deleteAgent:
			return m.confirmDeleteAgent()
		case deleteShell:
			return m.confirmDeleteShell()
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		return m, cmd
	}
}
```

**Step 4: Add granular delete methods**

```go
func (m Model) confirmDeleteAll() (tea.Model, tea.Cmd) {
	target := m.confirm.target
	m.confirm = nil
	m.screen = screenList

	sesSvc := m.sesSvc
	wtSvc := m.wtSvc
	tmuxClient := m.tmuxClient
	project := target.Project
	branch := target.Branch

	return m, func() tea.Msg {
		agentName := semconv.SessionName(project, branch)
		if running, _ := tmuxClient.HasSession(agentName); running {
			_ = sesSvc.Stop(agentName)
		}

		shellName := semconv.ShellSessionName(project, branch)
		if running, _ := tmuxClient.HasSession(shellName); running {
			_ = tmuxClient.KillSession(shellName)
		}

		err := wtSvc.Delete(worktree.DeleteRequest{
			Project: project,
			Branch:  branch,
			Force:   true,
		})
		if err != nil {
			return errMsg{err: err}
		}
		return m.refreshCmd()()
	}
}

func (m Model) confirmDeleteAgent() (tea.Model, tea.Cmd) {
	target := m.confirm.target
	m.confirm = nil
	m.screen = screenList

	sesSvc := m.sesSvc
	project := target.Project
	branch := target.Branch

	return m, func() tea.Msg {
		agentName := semconv.SessionName(project, branch)
		if err := sesSvc.Stop(agentName); err != nil {
			return errMsg{err: err}
		}
		return m.refreshCmd()()
	}
}

func (m Model) confirmDeleteShell() (tea.Model, tea.Cmd) {
	target := m.confirm.target
	m.confirm = nil
	m.screen = screenList

	tmuxClient := m.tmuxClient
	project := target.Project
	branch := target.Branch

	return m, func() tea.Msg {
		shellName := semconv.ShellSessionName(project, branch)
		if err := tmuxClient.KillSession(shellName); err != nil {
			return errMsg{err: err}
		}
		return m.refreshCmd()()
	}
}
```

**Step 5: Remove old `confirmDeleteYes`**

Delete the `confirmDeleteYes` method — it's replaced by `confirmDeleteAll`, `confirmDeleteAgent`, and `confirmDeleteShell`.

**Step 6: Remove `state` import from actions.go**

The `state` import is no longer needed.

**Step 7: Update existing tests**

Update the existing confirm tests in `actions_test.go`:
- `TestUpdateConfirmDelete_esc`: unchanged (still works)
- `TestUpdateConfirmDelete_enterMatch`: replace with test that Enter on first choice (deleteAll) returns to list
- `TestUpdateConfirmDelete_enterNoMatch`: remove (no longer applicable — no text input)
- `TestConfirmModel_ready_*`: remove (no longer applicable)
- `TestConfirmModel_update_enter_*`: remove (no longer applicable)

**Step 8: Run tests**

Run: `go test ./internal/tui/... -v`
Expected: PASS

**Step 9: Commit**

```bash
git add internal/tui/actions.go internal/tui/actions_test.go
git commit -m "feat: granular delete actions (all, agent only, shell only)"
```

---

### Task 10: Update callers of NewModel and NewService

**Files:**
- Modify: `cmd/session.go` (if not already done in Task 4)
- Modify: `cmd/worktree.go` (TUI launch point)
- Search for all `NewModel` and `NewService` calls

**Step 1: Find all call sites**

Run: `grep -rn "NewModel\|NewService\|sessionsDir" --include="*.go" .`

**Step 2: Update each call site**

- `NewModel(cfg, wtSvc, sesSvc, projSvc, tmuxClient, sessionsDir)` becomes `NewModel(cfg, wtSvc, sesSvc, projSvc, tmuxClient)`
- `session.NewService(tmuxClient, sessionsDir)` becomes `session.NewService(tmuxClient)`

**Step 3: Run full test suite**

Run: `go test ./... -count=1`
Expected: PASS

**Step 4: Commit**

```bash
git add -A
git commit -m "refactor: update all callers for removed sessionsDir parameter"
```

---

### Task 11: Run coverage check and fix gaps

**Step 1: Run coverage**

Run: `make coverage`
Expected: Coverage >= 80%

**Step 2: If coverage drops, add missing tests**

Likely areas needing coverage:
- `confirmDeleteAgent`, `confirmDeleteShell` methods
- `GetOption`/`SetOption` edge cases
- `IsMain` field in items

**Step 3: Run lint**

Run: `make lint`
Expected: PASS

**Step 4: Final commit if tests were added**

```bash
git add -A
git commit -m "test: add coverage for delete redesign"
```

---

### Task 12: Run full verification

**Step 1: Full test suite**

Run: `make test`
Expected: PASS

**Step 2: Coverage check**

Run: `make coverage`
Expected: >= 80%

**Step 3: Lint**

Run: `make lint`
Expected: PASS

**Step 4: Build**

Run: `make build`
Expected: SUCCESS
