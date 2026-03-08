# TUI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build an interactive terminal dashboard (`devenv tui`) using Bubble Tea v2 that provides a single-list view for managing worktrees, agent sessions, and shell sessions.

**Architecture:** Single Bubble Tea v2 program with a `bubbles/v2/list` component, custom `ItemDelegate` for two-line rendering with status tags, and a form screen for creating new worktrees. Data is fetched from existing services (`worktree.Service`, `session.Service`, `project.Service`) via async `tea.Cmd` commands and refreshed every 3 seconds. Attach exits the TUI and `syscall.Exec`s into `tmux attach-session`.

**Tech Stack:** Go, Bubble Tea v2 (`charm.land/bubbletea/v2`), Bubbles v2 (`charm.land/bubbles/v2`), Lipgloss v2 (`charm.land/lipgloss/v2`), existing internal services.

**Design doc:** `docs/plans/2026-03-06-tui-design.md`

---

### Task 1: Add Bubble Tea v2 dependencies

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`

**Step 1: Add v2 modules**

```bash
go get charm.land/bubbletea/v2@latest
go get charm.land/bubbles/v2@latest
go get charm.land/lipgloss/v2@latest
go mod tidy
```

If `charm.land` vanity imports don't resolve, fall back to:
```bash
go get github.com/charmbracelet/bubbletea/v2@latest
go get github.com/charmbracelet/bubbles/v2@latest
go get github.com/charmbracelet/lipgloss/v2@latest
```

**Step 2: Verify coexistence with v1**

The existing `huh` v0.8.0 depends on bubbletea v1 / lipgloss v1. The v2 packages use different module paths, so both coexist. Verify:

```bash
go build ./...
go test ./...
```

Expected: all pass, no conflicts.

**Step 3: Verify import paths**

Create a throwaway file to confirm the actual import paths:

```bash
cat go.mod | grep -E 'bubbletea|bubbles|lipgloss'
```

Note the exact module paths for use in all subsequent tasks. The plan uses `charm.land/bubbletea/v2` etc. — adjust if different.

**Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add Bubble Tea v2, Bubbles v2, Lipgloss v2 dependencies"
```

---

### Task 2: Add `ShellSessionName` to semconv

**Files:**
- Modify: `internal/semconv/semconv.go`
- Modify: `internal/semconv/semconv_test.go`

**Step 1: Write the failing test**

Add to `internal/semconv/semconv_test.go`:

```go
func TestShellSessionName(t *testing.T) {
	tests := []struct {
		project, branch, want string
	}{
		{"myapp", "feature", "myapp-feature~sh"},
		{"myapp", "feature/login", "myapp-feature-login~sh"},
		{"api", "fix/auth/token", "api-fix-auth-token~sh"},
	}
	for _, tt := range tests {
		if got := semconv.ShellSessionName(tt.project, tt.branch); got != tt.want {
			t.Errorf("ShellSessionName(%q, %q) = %q, want %q", tt.project, tt.branch, got, tt.want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/semconv/...
```

Expected: FAIL — `semconv.ShellSessionName` undefined.

**Step 3: Implement**

Add to `internal/semconv/semconv.go`:

```go
func ShellSessionName(project, branch string) string {
	return SessionName(project, branch) + "~sh"
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/semconv/...
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/semconv/semconv.go internal/semconv/semconv_test.go
git commit -m "feat: add ShellSessionName to semconv"
```

---

### Task 3: TUI item types and sorting

This task creates the core data types and the pure `buildItems` function that transforms service data into sorted list items. This is the most testable piece of the TUI.

**Files:**
- Create: `internal/tui/items.go`
- Create: `internal/tui/items_test.go`

**Step 1: Write the failing tests**

Create `internal/tui/items_test.go`:

```go
package tui

import (
	"testing"

	"github.com/xico42/devenv/internal/state"
)

func TestItem_FilterValue_worktree(t *testing.T) {
	i := Item{Project: "myapp", Branch: "feature", Group: groupAgent}
	want := "myapp / feature"
	if got := i.FilterValue(); got != want {
		t.Errorf("FilterValue() = %q, want %q", got, want)
	}
}

func TestItem_FilterValue_project(t *testing.T) {
	i := Item{Project: "myapp", Group: groupProject}
	want := "myapp"
	if got := i.FilterValue(); got != want {
		t.Errorf("FilterValue() = %q, want %q", got, want)
	}
}

func TestBuildItems_groupOrdering(t *testing.T) {
	data := refreshResult{
		worktrees: []wtEntry{
			{project: "api", branch: "develop", path: "/p/api/wt/develop"},
			{project: "myapp", branch: "feature", path: "/p/myapp/wt/feature"},
		},
		agentSessions: map[string]agentInfo{
			"myapp-feature": {status: state.SessionRunning},
		},
		shellSessions: map[string]bool{},
		projects: []projEntry{
			{name: "api", cloned: true},
			{name: "frontend", cloned: true},
			{name: "infra", cloned: false},
			{name: "myapp", cloned: true},
		},
	}

	items := buildItems(data)

	if len(items) != 4 {
		t.Fatalf("got %d items, want 4", len(items))
	}

	// Group 1: worktrees with agents
	first := items[0].(Item)
	if first.Project != "myapp" || first.Group != groupAgent {
		t.Errorf("item 0: got %s group %d, want myapp group %d", first.Project, first.Group, groupAgent)
	}

	// Group 2: worktrees without agents
	second := items[1].(Item)
	if second.Project != "api" || second.Group != groupWorktree {
		t.Errorf("item 1: got %s group %d, want api group %d", second.Project, second.Group, groupWorktree)
	}

	// Group 3: projects without worktrees (alphabetical)
	third := items[2].(Item)
	if third.Project != "frontend" || third.Group != groupProject {
		t.Errorf("item 2: got %s group %d, want frontend group %d", third.Project, third.Group, groupProject)
	}
	fourth := items[3].(Item)
	if fourth.Project != "infra" || fourth.Group != groupProject {
		t.Errorf("item 3: got %s group %d, want infra group %d", fourth.Project, fourth.Group, groupProject)
	}
}

func TestBuildItems_agentStatus(t *testing.T) {
	data := refreshResult{
		worktrees: []wtEntry{
			{project: "myapp", branch: "feat", path: "/p/wt/feat"},
		},
		agentSessions: map[string]agentInfo{
			"myapp-feat": {status: state.SessionWaiting, question: "Allow?"},
		},
		shellSessions: map[string]bool{},
		projects:      []projEntry{{name: "myapp", cloned: true}},
	}

	items := buildItems(data)
	item := items[0].(Item)

	if item.AgentStatus != state.SessionWaiting {
		t.Errorf("AgentStatus = %q, want %q", item.AgentStatus, state.SessionWaiting)
	}
	if item.Question != "Allow?" {
		t.Errorf("Question = %q, want %q", item.Question, "Allow?")
	}
}

func TestBuildItems_shellSession(t *testing.T) {
	data := refreshResult{
		worktrees: []wtEntry{
			{project: "api", branch: "dev", path: "/p/wt/dev"},
		},
		agentSessions: map[string]agentInfo{},
		shellSessions: map[string]bool{"api-dev~sh": true},
		projects:      []projEntry{{name: "api", cloned: true}},
	}

	items := buildItems(data)
	item := items[0].(Item)

	if !item.HasShell {
		t.Error("expected HasShell = true")
	}
}

func TestBuildItems_cloneStatus(t *testing.T) {
	data := refreshResult{
		worktrees:     []wtEntry{},
		agentSessions: map[string]agentInfo{},
		shellSessions: map[string]bool{},
		projects: []projEntry{
			{name: "cloned-proj", cloned: true},
			{name: "uncloned-proj", cloned: false},
		},
	}

	items := buildItems(data)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	first := items[0].(Item)
	if !first.Cloned {
		t.Error("expected cloned-proj to have Cloned = true")
	}

	second := items[1].(Item)
	if second.Cloned {
		t.Error("expected uncloned-proj to have Cloned = false")
	}
}

func TestBuildItems_alphabeticalWithinGroup(t *testing.T) {
	data := refreshResult{
		worktrees: []wtEntry{
			{project: "zoo", branch: "main", path: "/p/wt/1"},
			{project: "alpha", branch: "main", path: "/p/wt/2"},
			{project: "alpha", branch: "beta", path: "/p/wt/3"},
		},
		agentSessions: map[string]agentInfo{},
		shellSessions: map[string]bool{},
		projects: []projEntry{
			{name: "alpha", cloned: true},
			{name: "zoo", cloned: true},
		},
	}

	items := buildItems(data)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}

	// All group 2 (no agents), alphabetical by project then branch
	i0 := items[0].(Item)
	i1 := items[1].(Item)
	i2 := items[2].(Item)

	if i0.Project != "alpha" || i0.Branch != "beta" {
		t.Errorf("item 0: got %s/%s, want alpha/beta", i0.Project, i0.Branch)
	}
	if i1.Project != "alpha" || i1.Branch != "main" {
		t.Errorf("item 1: got %s/%s, want alpha/main", i1.Project, i1.Branch)
	}
	if i2.Project != "zoo" {
		t.Errorf("item 2: got %s, want zoo", i2.Project)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/...
```

Expected: FAIL — package doesn't exist.

**Step 3: Implement item types and buildItems**

Create `internal/tui/items.go`:

```go
package tui

import (
	"sort"

	"charm.land/bubbles/v2/list"

	"github.com/xico42/devenv/internal/semconv"
)

// Item groups determine sort priority.
const (
	groupAgent   = 1 // worktrees with active agent sessions
	groupWorktree = 2 // worktrees without agent sessions
	groupProject = 3 // projects without worktrees
)

// Item represents a single entry in the TUI list.
type Item struct {
	Project     string
	Branch      string
	Path        string
	Group       int
	HasAgent    bool
	AgentStatus string // "running", "waiting", ""
	Question    string
	HasShell    bool
	Cloned      bool
}

func (i Item) FilterValue() string {
	if i.Branch != "" {
		return i.Project + " / " + i.Branch
	}
	return i.Project
}

// refreshResult holds raw data collected during a refresh cycle.
type refreshResult struct {
	worktrees     []wtEntry
	agentSessions map[string]agentInfo // keyed by session name (project-branch)
	shellSessions map[string]bool      // keyed by shell session name (project-branch~sh)
	projects      []projEntry
}

type wtEntry struct {
	project string
	branch  string
	path    string
}

type agentInfo struct {
	status   string
	question string
}

type projEntry struct {
	name   string
	cloned bool
}

// buildItems transforms refresh data into a sorted slice of list items.
func buildItems(data refreshResult) []list.Item {
	// Track which projects have worktrees.
	projectHasWorktree := make(map[string]bool)

	var items []Item
	for _, wt := range data.worktrees {
		projectHasWorktree[wt.project] = true

		sessionName := semconv.SessionName(wt.project, wt.branch)
		shellName := semconv.ShellSessionName(wt.project, wt.branch)

		item := Item{
			Project:  wt.project,
			Branch:   wt.branch,
			Path:     wt.path,
			HasShell: data.shellSessions[shellName],
		}

		if agent, ok := data.agentSessions[sessionName]; ok {
			item.Group = groupAgent
			item.HasAgent = true
			item.AgentStatus = agent.status
			item.Question = agent.question
		} else {
			item.Group = groupWorktree
		}

		items = append(items, item)
	}

	for _, p := range data.projects {
		if projectHasWorktree[p.name] {
			continue
		}
		items = append(items, Item{
			Project: p.name,
			Group:   groupProject,
			Cloned:  p.cloned,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Group != items[j].Group {
			return items[i].Group < items[j].Group
		}
		if items[i].Project != items[j].Project {
			return items[i].Project < items[j].Project
		}
		return items[i].Branch < items[j].Branch
	})

	result := make([]list.Item, len(items))
	for i, item := range items {
		result[i] = item
	}
	return result
}
```

**Step 4: Run tests to verify they pass**

```bash
go test ./internal/tui/...
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui/items.go internal/tui/items_test.go
git commit -m "feat(tui): add item types and buildItems sorting logic"
```

---

### Task 4: TUI key bindings

**Files:**
- Create: `internal/tui/keys.go`
- Create: `internal/tui/keys_test.go`

**Step 1: Write the failing test**

Create `internal/tui/keys_test.go`:

```go
package tui

import "testing"

func TestKeyMap_ShortHelp(t *testing.T) {
	km := defaultKeyMap()
	bindings := km.ShortHelp()
	if len(bindings) == 0 {
		t.Fatal("ShortHelp returned no bindings")
	}
	// Verify key actions are present.
	keys := make(map[string]bool)
	for _, b := range bindings {
		keys[b.Help().Key] = true
	}
	for _, want := range []string{"a/enter", "s", "n", "d", "q"} {
		if !keys[want] {
			t.Errorf("ShortHelp missing key %q", want)
		}
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/tui/...
```

Expected: FAIL — `defaultKeyMap` undefined.

**Step 3: Implement key bindings**

Create `internal/tui/keys.go`:

```go
package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Attach  key.Binding
	Shell   key.Binding
	Clone   key.Binding
	New     key.Binding
	Delete  key.Binding
	Refresh key.Binding
	Help    key.Binding
	Quit    key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Attach: key.NewBinding(
			key.WithKeys("a", "enter"),
			key.WithHelp("a/enter", "agent"),
		),
		Shell: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "shell"),
		),
		Clone: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "clone"),
		),
		New: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "new"),
		),
		Delete: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Attach, k.Shell, k.New, k.Delete, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Attach, k.Shell, k.Clone},
		{k.New, k.Delete, k.Refresh},
		{k.Help, k.Quit},
	}
}
```

**Step 4: Run tests**

```bash
go test ./internal/tui/...
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui/keys.go internal/tui/keys_test.go
git commit -m "feat(tui): add key bindings and help integration"
```

---

### Task 5: TUI custom delegate

The delegate renders each list item as two lines: `project / branch` on line 1, styled status tags on line 2.

**Files:**
- Create: `internal/tui/delegate.go`
- Create: `internal/tui/delegate_test.go`

**Step 1: Write the failing tests**

Create `internal/tui/delegate_test.go`:

```go
package tui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"

	"github.com/xico42/devenv/internal/state"
)

func TestDelegate_Height(t *testing.T) {
	d := newDelegate()
	if d.Height() != 2 {
		t.Errorf("Height() = %d, want 2", d.Height())
	}
}

func TestDelegate_Spacing(t *testing.T) {
	d := newDelegate()
	if d.Spacing() != 1 {
		t.Errorf("Spacing() = %d, want 1", d.Spacing())
	}
}

func TestDelegate_Render_agentWaiting(t *testing.T) {
	d := newDelegate()
	m := list.New([]list.Item{
		Item{Project: "myapp", Branch: "feature", Group: groupAgent, HasAgent: true, AgentStatus: state.SessionWaiting, HasShell: true},
	}, d, 80, 10)

	var buf bytes.Buffer
	d.Render(&buf, m, 0, m.Items()[0])
	out := buf.String()

	if !strings.Contains(out, "myapp") || !strings.Contains(out, "feature") {
		t.Errorf("render missing project/branch, got: %q", out)
	}
	if !strings.Contains(out, "WAITING FOR INPUT") {
		t.Errorf("render missing WAITING FOR INPUT tag, got: %q", out)
	}
	if !strings.Contains(out, "shell") {
		t.Errorf("render missing shell tag, got: %q", out)
	}
}

func TestDelegate_Render_projectNotCloned(t *testing.T) {
	d := newDelegate()
	m := list.New([]list.Item{
		Item{Project: "infra", Group: groupProject, Cloned: false},
	}, d, 80, 10)

	var buf bytes.Buffer
	d.Render(&buf, m, 0, m.Items()[0])
	out := buf.String()

	if !strings.Contains(out, "infra") {
		t.Errorf("render missing project name, got: %q", out)
	}
	if !strings.Contains(out, "not cloned") {
		t.Errorf("render missing 'not cloned' tag, got: %q", out)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/...
```

Expected: FAIL — `newDelegate` undefined.

**Step 3: Implement delegate**

Create `internal/tui/delegate.go`:

```go
package tui

import (
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"

	"github.com/xico42/devenv/internal/state"
)

// ANSI 256 colors (mosh-safe, no true color).
var (
	titleStyle       = lipgloss.NewStyle().Bold(true)
	selectedStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("170"))
	cursorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("170"))
	waitingTag       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("226"))
	runningTag       = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	shellTag         = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
	clonedTag        = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("34"))
	notClonedTag     = lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("240"))
	dimStyle         = lipgloss.NewStyle().Faint(true)
)

type itemDelegate struct{}

func newDelegate() itemDelegate { return itemDelegate{} }

func (d itemDelegate) Height() int                                 { return 2 }
func (d itemDelegate) Spacing() int                                { return 1 }
func (d itemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd     { return nil }

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	item, ok := listItem.(Item)
	if !ok {
		return
	}

	isSelected := index == m.Index()

	// Line 1: project / branch
	var line1 string
	if item.Branch != "" {
		line1 = item.Project + " / " + item.Branch
	} else {
		line1 = item.Project
	}

	cursor := "  "
	if isSelected {
		cursor = cursorStyle.Render("▸ ")
		line1 = selectedStyle.Render(line1)
	} else {
		line1 = "  " + line1
	}

	// Line 2: status tags
	var tags []string
	switch {
	case item.HasAgent && item.AgentStatus == state.SessionWaiting:
		tags = append(tags, waitingTag.Render("WAITING FOR INPUT"))
	case item.HasAgent && item.AgentStatus == state.SessionRunning:
		tags = append(tags, runningTag.Render("running"))
	case item.HasAgent:
		tags = append(tags, runningTag.Render("agent"))
	}

	if item.HasShell {
		tags = append(tags, shellTag.Render("shell"))
	}

	if item.Group == groupProject {
		if item.Cloned {
			tags = append(tags, clonedTag.Render("cloned"))
		} else {
			tags = append(tags, notClonedTag.Render("not cloned"))
		}
	}

	line2 := "    " + strings.Join(tags, "  ")
	if len(tags) == 0 {
		line2 = ""
	}

	if isSelected {
		fmt.Fprintf(w, "%s%s\n%s", cursor, line1, line2)
	} else {
		fmt.Fprintf(w, "%s\n%s", line1, line2)
	}
}
```

**Step 4: Run tests**

```bash
go test ./internal/tui/...
```

Expected: PASS. If `list.New` signature or `list.Model` API differs in v2, adjust constructor args accordingly — check with `go doc charm.land/bubbles/v2/list.New`.

**Step 5: Commit**

```bash
git add internal/tui/delegate.go internal/tui/delegate_test.go
git commit -m "feat(tui): add custom ItemDelegate with two-line tag rendering"
```

---

### Task 6: TUI model — list view with auto-refresh

This creates the top-level Bubble Tea model: screen routing, list setup, data refresh via async commands, and the View rendering.

**Files:**
- Create: `internal/tui/model.go`
- Create: `internal/tui/model_test.go`

**Step 1: Write the failing tests**

Create `internal/tui/model_test.go`:

```go
package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModel_Update_windowResize(t *testing.T) {
	m := Model{screen: screenList}
	m.list = newList(nil)

	msg := tea.WindowSizeMsg{Width: 100, Height: 40}
	updated, _ := m.Update(msg)
	um := updated.(Model)

	if um.width != 100 || um.height != 40 {
		t.Errorf("size = %dx%d, want 100x40", um.width, um.height)
	}
}

func TestModel_Update_quit(t *testing.T) {
	m := Model{screen: screenList}
	m.list = newList(nil)
	m.keys = defaultKeyMap()

	msg := tea.KeyPressMsg(tea.KeyPressEvent{})
	// Simulate 'q' press — we test the quit path via Cmd check.
	// Direct key simulation is tricky; test via itemsMsg instead.
	_, _ = m.Update(msg)
}

func TestModel_Update_itemsMsg(t *testing.T) {
	m := Model{screen: screenList}
	m.list = newList(nil)

	items := []Item{
		{Project: "myapp", Branch: "feat", Group: groupAgent},
	}

	updated, _ := m.Update(itemsMsg(items))
	um := updated.(Model)

	if len(um.list.Items()) != 1 {
		t.Errorf("list has %d items, want 1", len(um.list.Items()))
	}
}

func TestModel_Update_errMsg(t *testing.T) {
	m := Model{screen: screenList}
	m.list = newList(nil)

	updated, _ := m.Update(errMsg{err: nil})
	um := updated.(Model)

	if um.screen != screenList {
		t.Errorf("screen = %d, want %d", um.screen, screenList)
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/...
```

Expected: FAIL — `Model`, `screenList`, `newList`, `itemsMsg`, `errMsg` undefined.

**Step 3: Implement model**

Create `internal/tui/model.go`:

```go
package tui

import (
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/project"
	"github.com/xico42/devenv/internal/semconv"
	"github.com/xico42/devenv/internal/session"
	"github.com/xico42/devenv/internal/state"
	"github.com/xico42/devenv/internal/tmux"
	"github.com/xico42/devenv/internal/worktree"
)

const (
	screenList = iota
	screenForm
	screenConfirmDelete
)

const maxWidth = 80
const refreshInterval = 3 * time.Second

// Messages
type tickMsg time.Time
type itemsMsg []Item
type errMsg struct{ err error }
type attachMsg struct{ session string }
type cloneDoneMsg struct{ project string }
type worktreeCreatedMsg struct {
	project string
	branch  string
	path    string
}

// Model is the top-level Bubble Tea model.
type Model struct {
	screen int
	list   list.Model
	keys   keyMap
	help   help.Model

	cfg         *config.Config
	wtSvc       *worktree.Service
	sesSvc      *session.Service
	projSvc     *project.Service
	tmuxClient  *tmux.Client
	sessionsDir string

	width  int
	height int

	// Set before quitting to trigger tmux attach.
	PendingAttach string

	// Delete confirmation state.
	deleteTarget *Item

	// Status message for async operations.
	statusMsg string

	// Form sub-model.
	form *formModel
}

// NewModel creates the TUI model with all required services.
func NewModel(
	cfg *config.Config,
	wtSvc *worktree.Service,
	sesSvc *session.Service,
	projSvc *project.Service,
	tmuxClient *tmux.Client,
	sessionsDir string,
) Model {
	keys := defaultKeyMap()
	delegate := newDelegate()
	l := newList(nil)
	l.Title = "devenv"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false) // we render our own help
	_ = delegate // delegate is set via newList or SetDelegate

	h := help.New()

	return Model{
		screen:      screenList,
		list:        l,
		keys:        keys,
		help:        h,
		cfg:         cfg,
		wtSvc:       wtSvc,
		sesSvc:      sesSvc,
		projSvc:     projSvc,
		tmuxClient:  tmuxClient,
		sessionsDir: sessionsDir,
	}
}

func newList(items []list.Item) list.Model {
	if items == nil {
		items = []list.Item{}
	}
	l := list.New(items, newDelegate(), maxWidth, 20)
	l.Title = "devenv"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	return l
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refreshCmd(), tickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w := msg.Width
		if w > maxWidth {
			w = maxWidth
		}
		m.list.SetSize(w, msg.Height-4) // room for title + help
		m.help.SetWidth(w)
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.refreshCmd(), tickCmd())

	case itemsMsg:
		items := make([]list.Item, len(msg))
		for i, item := range msg {
			items[i] = item
		}
		// Preserve selection.
		var selProject, selBranch string
		if sel, ok := m.list.SelectedItem().(Item); ok {
			selProject = sel.Project
			selBranch = sel.Branch
		}
		m.list.SetItems(items)
		if selProject != "" {
			for i, li := range items {
				if it, ok := li.(Item); ok && it.Project == selProject && it.Branch == selBranch {
					m.list.Select(i)
					break
				}
			}
		}
		return m, nil

	case errMsg:
		if msg.err != nil {
			m.statusMsg = msg.err.Error()
		}
		return m, nil

	case attachMsg:
		m.PendingAttach = msg.session
		return m, tea.Quit

	case cloneDoneMsg:
		m.statusMsg = fmt.Sprintf("Cloned %s", msg.project)
		return m, m.refreshCmd()

	case worktreeCreatedMsg:
		m.statusMsg = fmt.Sprintf("Created %s/%s", msg.project, msg.branch)
		return m, m.refreshCmd()
	}

	// Route to sub-screens.
	switch m.screen {
	case screenConfirmDelete:
		return m.updateConfirmDelete(msg)
	case screenForm:
		return m.updateForm(msg)
	default:
		return m.updateList(msg)
	}
}

func (m Model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		// Don't handle custom keys while filtering.
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Attach):
			return m, m.attachAction()

		case key.Matches(msg, m.keys.Shell):
			return m, m.shellAction()

		case key.Matches(msg, m.keys.Clone):
			return m, m.cloneAction()

		case key.Matches(msg, m.keys.New):
			return m.showForm()

		case key.Matches(msg, m.keys.Delete):
			return m.startDelete()

		case key.Matches(msg, m.keys.Refresh):
			return m, m.refreshCmd()

		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() tea.View {
	switch m.screen {
	case screenForm:
		return tea.NewView(m.form.View())
	case screenConfirmDelete:
		return tea.NewView(m.viewConfirmDelete())
	default:
		return tea.NewView(m.viewList())
	}
}

func (m Model) viewList() string {
	// Count agents for title bar.
	agentCount := 0
	for _, item := range m.list.Items() {
		if it, ok := item.(Item); ok && it.HasAgent {
			agentCount++
		}
	}

	titleBar := lipgloss.NewStyle().Bold(true).Render("devenv")
	if agentCount > 0 {
		counter := lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf("%d agents", agentCount))
		pad := maxWidth - lipgloss.Width(titleBar) - lipgloss.Width(counter)
		if pad < 1 {
			pad = 1
		}
		titleBar = titleBar + fmt.Sprintf("%*s", pad, counter)
	}

	w := m.width
	if w > maxWidth {
		w = maxWidth
	}

	helpView := m.help.View(m.keys)

	var status string
	if m.statusMsg != "" {
		status = "\n" + lipgloss.NewStyle().Faint(true).Render(m.statusMsg)
	}

	return fmt.Sprintf("%s\n\n%s%s\n\n%s", titleBar, m.list.View(), status, helpView)
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// refreshCmd fetches all data from services asynchronously.
func (m Model) refreshCmd() tea.Cmd {
	wtSvc := m.wtSvc
	tmuxClient := m.tmuxClient
	sessionsDir := m.sessionsDir
	cfg := m.cfg

	return func() tea.Msg {
		data := refreshResult{
			agentSessions: make(map[string]agentInfo),
			shellSessions: make(map[string]bool),
		}

		// 1. Worktrees
		if wtSvc != nil {
			entries, err := wtSvc.List("")
			if err == nil {
				for _, e := range entries {
					data.worktrees = append(data.worktrees, wtEntry{
						project: e.Project,
						branch:  e.Branch,
						path:    e.Path,
					})
				}
			}
		}

		// 2. Session states (for agent status/question)
		if sessionsDir != "" {
			states, err := state.ListSessions(sessionsDir)
			if err == nil {
				for _, s := range states {
					data.agentSessions[s.Session] = agentInfo{
						status:   s.Status,
						question: s.Question,
					}
				}
			}
		}

		// 3. Tmux sessions (for shell session detection)
		if tmuxClient != nil {
			names, err := tmuxClient.ListSessions()
			if err == nil {
				for _, name := range names {
					data.shellSessions[name] = true
				}
			}
		}

		// 4. Project list with clone status
		if cfg != nil {
			for name := range cfg.Projects {
				p := cfg.Projects[name]
				cloned := false
				if rp, err := config.RepoPath(p.Repo); err == nil {
					path := semconv.CloneDir(cfg.Defaults.ProjectsDir, rp)
					if _, err := os.Stat(path); err == nil {
						cloned = true
					}
				}
				data.projects = append(data.projects, projEntry{name: name, cloned: cloned})
			}
		}

		items := buildItems(data)
		result := make([]Item, len(items))
		for i, li := range items {
			result[i] = li.(Item)
		}
		return itemsMsg(result)
	}
}

// selectedItem returns the currently selected Item, or nil.
func (m Model) selectedItem() *Item {
	sel, ok := m.list.SelectedItem().(Item)
	if !ok {
		return nil
	}
	return &sel
}
```

**Step 4: Run tests**

```bash
go test ./internal/tui/...
```

Expected: PASS. Some tests may need adjustment based on exact bubbles v2 API — e.g., `list.Model.Select(i)` might be `list.Model.SetCursor(i)`. Check with `go doc charm.land/bubbles/v2/list Model.Select` and adapt.

**Step 5: Commit**

```bash
git add internal/tui/model.go internal/tui/model_test.go
git commit -m "feat(tui): add main model with list view and auto-refresh"
```

---

### Task 7: TUI actions — attach, shell, clone, delete

Implements the key-press handlers that orchestrate service calls.

**Files:**
- Create: `internal/tui/actions.go`
- Create: `internal/tui/actions_test.go`

**Step 1: Write the failing tests**

Create `internal/tui/actions_test.go`:

```go
package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
)

func TestStartDelete_noSelection(t *testing.T) {
	m := Model{screen: screenList}
	m.list = newList(nil)

	updated, _ := m.startDelete()
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d (should stay on list)", um.screen, screenList)
	}
}

func TestStartDelete_projectItem(t *testing.T) {
	items := []Item{{Project: "infra", Group: groupProject}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList}
	m.list = newList(listItems)

	updated, _ := m.startDelete()
	um := updated.(Model)
	// Can't delete a project-only item.
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d (should stay on list for project items)", um.screen, screenList)
	}
}

func TestConfirmDelete_cancel(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{
		screen:       screenConfirmDelete,
		deleteTarget: &target,
	}
	m.list = newList(nil)

	// Press 'n' to cancel.
	// We test the state transition, not the exact key.
	updated, _ := m.confirmDeleteNo()
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after cancel", um.screen, screenList)
	}
	if um.deleteTarget != nil {
		t.Error("deleteTarget should be nil after cancel")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/...
```

Expected: FAIL — `startDelete`, `confirmDeleteNo`, etc. undefined.

**Step 3: Implement actions**

Create `internal/tui/actions.go`:

```go
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/semconv"
	"github.com/xico42/devenv/internal/session"
	"github.com/xico42/devenv/internal/state"
	"github.com/xico42/devenv/internal/worktree"
)

// resolveAgentCommand builds the full agent command string from config,
// matching the logic in cmd/session.go:resolveAgentCmd.
func resolveAgentCommand(cfg *config.Config, project string) string {
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

// ── Attach (agent) ──────────────────────────────────────────────────────────

func (m Model) attachAction() tea.Cmd {
	sel := m.selectedItem()
	if sel == nil {
		return nil
	}

	sesSvc := m.sesSvc
	wtSvc := m.wtSvc
	projSvc := m.projSvc
	cfg := m.cfg
	project := sel.Project
	branch := sel.Branch

	switch sel.Group {
	case groupAgent:
		// Already has agent — just attach.
		sessionName := semconv.SessionName(project, branch)
		return func() tea.Msg { return attachMsg{session: sessionName} }

	case groupWorktree:
		// Has worktree but no agent — start agent then attach.
		path := sel.Path
		return func() tea.Msg {
			sessionName := semconv.SessionName(project, branch)
			agentCmd := resolveAgentCommand(cfg, project)
			agentCfg := cfg.ResolveAgent(project)
			env := make(map[string]string)
			for k, v := range agentCfg.Env {
				env[k] = v
			}
			err := sesSvc.Start(session.StartRequest{
				Project: project,
				Branch:  branch,
				Path:    path,
				Cmd:     agentCmd,
				Env:     env,
			})
			if err != nil {
				return errMsg{err: err}
			}
			return attachMsg{session: sessionName}
		}

	case groupProject:
		// No worktree — need branch. For now, use default branch.
		return func() tea.Msg {
			defaultBranch := "main"
			if p, ok := cfg.Projects[project]; ok && p.DefaultBranch != "" {
				defaultBranch = p.DefaultBranch
			}

			// Clone if needed.
			if projSvc != nil {
				_ = projSvc.Clone(project) // ignore AlreadyClonedError
			}

			// Create worktree.
			result, err := wtSvc.New(project, defaultBranch)
			if err != nil {
				return errMsg{err: err}
			}

			// Start agent.
			sessionName := semconv.SessionName(project, defaultBranch)
			agentCmd := resolveAgentCommand(cfg, project)
			agentCfg := cfg.ResolveAgent(project)
			env := make(map[string]string)
			for k, v := range agentCfg.Env {
				env[k] = v
			}
			err = sesSvc.Start(session.StartRequest{
				Project: project,
				Branch:  defaultBranch,
				Path:    result.Path,
				Cmd:     agentCmd,
				Env:     env,
			})
			if err != nil {
				return errMsg{err: err}
			}
			return attachMsg{session: sessionName}
		}
	}
	return nil
}

// ── Shell ───────────────────────────────────────────────────────────────────

func (m Model) shellAction() tea.Cmd {
	sel := m.selectedItem()
	if sel == nil {
		return nil
	}

	tmuxClient := m.tmuxClient
	wtSvc := m.wtSvc
	projSvc := m.projSvc
	cfg := m.cfg
	project := sel.Project
	branch := sel.Branch
	path := sel.Path

	return func() tea.Msg {
		// For group 3, clone + create worktree first.
		if branch == "" {
			defaultBranch := "main"
			if p, ok := cfg.Projects[project]; ok && p.DefaultBranch != "" {
				defaultBranch = p.DefaultBranch
			}
			branch = defaultBranch

			if projSvc != nil {
				_ = projSvc.Clone(project)
			}
			result, err := wtSvc.New(project, branch)
			if err != nil {
				return errMsg{err: err}
			}
			path = result.Path
		}

		shellName := semconv.ShellSessionName(project, branch)

		// Create shell session if it doesn't exist.
		exists, err := tmuxClient.HasSession(shellName)
		if err != nil {
			return errMsg{err: err}
		}
		if !exists {
			if err := tmuxClient.NewSession(shellName, path); err != nil {
				return errMsg{err: err}
			}
		}

		return attachMsg{session: shellName}
	}
}

// ── Clone ───────────────────────────────────────────────────────────────────

func (m Model) cloneAction() tea.Cmd {
	sel := m.selectedItem()
	if sel == nil || sel.Group != groupProject || sel.Cloned {
		return nil
	}

	projSvc := m.projSvc
	project := sel.Project

	return func() tea.Msg {
		if err := projSvc.Clone(project); err != nil {
			return errMsg{err: err}
		}
		return cloneDoneMsg{project: project}
	}
}

// ── Delete ──────────────────────────────────────────────────────────────────

func (m Model) startDelete() (tea.Model, tea.Cmd) {
	sel := m.selectedItem()
	if sel == nil || sel.Group == groupProject {
		return m, nil
	}
	m.deleteTarget = sel
	m.screen = screenConfirmDelete
	return m, nil
}

func (m Model) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "y", "Y":
			return m.confirmDeleteYes()
		default:
			return m.confirmDeleteNo()
		}
	}
	return m, nil
}

func (m Model) confirmDeleteYes() (tea.Model, tea.Cmd) {
	target := m.deleteTarget
	m.deleteTarget = nil
	m.screen = screenList

	if target == nil {
		return m, nil
	}

	sesSvc := m.sesSvc
	wtSvc := m.wtSvc
	tmuxClient := m.tmuxClient
	sessionsDir := m.sessionsDir
	project := target.Project
	branch := target.Branch

	return m, func() tea.Msg {
		// Kill agent session if exists.
		agentName := semconv.SessionName(project, branch)
		if running, _ := tmuxClient.HasSession(agentName); running {
			_ = sesSvc.Stop(agentName)
		}

		// Kill shell session if exists.
		shellName := semconv.ShellSessionName(project, branch)
		if running, _ := tmuxClient.HasSession(shellName); running {
			_ = tmuxClient.KillSession(shellName)
		}

		// Remove worktree.
		err := wtSvc.Delete(worktree.DeleteRequest{
			Project: project,
			Branch:  branch,
			Force:   true,
		})
		if err != nil {
			return errMsg{err: err}
		}

		_ = state.ClearSession(sessionsDir, agentName)

		return itemsMsg(nil) // trigger refresh
	}
}

func (m Model) confirmDeleteNo() (tea.Model, tea.Cmd) {
	m.deleteTarget = nil
	m.screen = screenList
	return m, nil
}

func (m Model) viewConfirmDelete() string {
	if m.deleteTarget == nil {
		return m.viewList()
	}
	name := m.deleteTarget.Project
	if m.deleteTarget.Branch != "" {
		name += " / " + m.deleteTarget.Branch
	}
	prompt := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("Delete %s? [y/n]", name),
	)
	return fmt.Sprintf("%s\n\n%s", m.viewList(), prompt)
}
```

**Step 4: Run tests**

```bash
go test ./internal/tui/...
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui/actions.go internal/tui/actions_test.go
git commit -m "feat(tui): add attach, shell, clone, delete actions"
```

---

### Task 8: TUI new worktree form

The form screen replaces the list when `n` is pressed. Two fields: project selector (cycles through configured projects) and branch text input.

**Files:**
- Create: `internal/tui/form.go`
- Create: `internal/tui/form_test.go`

**Step 1: Write the failing tests**

Create `internal/tui/form_test.go`:

```go
package tui

import (
	"testing"
)

func TestFormModel_cycleProject(t *testing.T) {
	projects := []string{"api", "frontend", "myapp"}
	f := newFormModel(projects, nil, nil)

	if f.selectedProject() != "api" {
		t.Errorf("initial project = %q, want %q", f.selectedProject(), "api")
	}

	f.nextProject()
	if f.selectedProject() != "frontend" {
		t.Errorf("after next: project = %q, want %q", f.selectedProject(), "frontend")
	}

	f.nextProject()
	f.nextProject() // wraps around
	if f.selectedProject() != "api" {
		t.Errorf("after wrap: project = %q, want %q", f.selectedProject(), "api")
	}
}

func TestFormModel_prevProject(t *testing.T) {
	projects := []string{"api", "frontend", "myapp"}
	f := newFormModel(projects, nil, nil)

	f.prevProject()
	if f.selectedProject() != "myapp" {
		t.Errorf("after prev from start: project = %q, want %q", f.selectedProject(), "myapp")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/tui/...
```

Expected: FAIL — `newFormModel`, `selectedProject`, `nextProject`, `prevProject` undefined.

**Step 3: Implement form**

Create `internal/tui/form.go`:

```go
package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/xico42/devenv/internal/project"
	"github.com/xico42/devenv/internal/worktree"
)

type formModel struct {
	projects    []string
	projectIdx  int
	branchInput textinput.Model

	wtSvc   *worktree.Service
	projSvc *project.Service
}

func newFormModel(projects []string, wtSvc *worktree.Service, projSvc *project.Service) *formModel {
	ti := textinput.New()
	ti.Placeholder = "branch-name"
	ti.Focus()

	return &formModel{
		projects:    projects,
		branchInput: ti,
		wtSvc:       wtSvc,
		projSvc:     projSvc,
	}
}

func (f *formModel) selectedProject() string {
	if len(f.projects) == 0 {
		return ""
	}
	return f.projects[f.projectIdx]
}

func (f *formModel) nextProject() {
	if len(f.projects) == 0 {
		return
	}
	f.projectIdx = (f.projectIdx + 1) % len(f.projects)
}

func (f *formModel) prevProject() {
	if len(f.projects) == 0 {
		return
	}
	f.projectIdx = (f.projectIdx - 1 + len(f.projects)) % len(f.projects)
}

func (f *formModel) Update(msg tea.Msg) (*formModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return f, nil // caller handles screen switch
		case "tab", "right":
			f.nextProject()
			return f, nil
		case "shift+tab", "left":
			f.prevProject()
			return f, nil
		case "enter":
			return f, f.submit()
		}
	}

	var cmd tea.Cmd
	f.branchInput, cmd = f.branchInput.Update(msg)
	return f, cmd
}

func (f *formModel) submit() tea.Cmd {
	proj := f.selectedProject()
	branch := strings.TrimSpace(f.branchInput.Value())
	if proj == "" || branch == "" {
		return nil
	}

	wtSvc := f.wtSvc
	projSvc := f.projSvc

	return func() tea.Msg {
		// Clone if needed.
		if projSvc != nil {
			_ = projSvc.Clone(proj) // ignore AlreadyClonedError
		}

		result, err := wtSvc.New(proj, branch)
		if err != nil {
			return errMsg{err: err}
		}
		return worktreeCreatedMsg{
			project: proj,
			branch:  branch,
			path:    result.Path,
		}
	}
}

func (f *formModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true)

	// Project selector line.
	var projParts []string
	for i, p := range f.projects {
		if i == f.projectIdx {
			projParts = append(projParts, lipgloss.NewStyle().Bold(true).Underline(true).Render(p))
		} else {
			projParts = append(projParts, lipgloss.NewStyle().Faint(true).Render(p))
		}
	}
	projLine := strings.Join(projParts, "  /  ")

	return fmt.Sprintf(
		"%s\n────\n  Project:  %s\n  Branch:   %s\n────\nEnter: create  |  Esc: cancel  |  Tab: next project",
		titleStyle.Render("New Worktree"),
		projLine,
		f.branchInput.View(),
	)
}

// showForm transitions the model to the form screen.
func (m Model) showForm() (tea.Model, tea.Cmd) {
	projects := make([]string, 0, len(m.cfg.Projects))
	for name := range m.cfg.Projects {
		projects = append(projects, name)
	}
	// Sort for deterministic order.
	sort.Strings(projects)

	m.form = newFormModel(projects, m.wtSvc, m.projSvc)
	m.screen = screenForm
	return m, nil
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

	// If form submitted successfully, switch back to list.
	// The worktreeCreatedMsg will be handled by the top-level Update.
	return m, cmd
}

```

**Step 4: Run tests**

```bash
go test ./internal/tui/...
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/tui/form.go internal/tui/form_test.go
git commit -m "feat(tui): add new worktree inline form"
```

---

### Task 9: Cobra command and attach flow

Wires the TUI model into a `devenv tui` command. After the program exits, checks for a pending attach and `syscall.Exec`s into tmux.

**Files:**
- Create: `cmd/tui.go`

**Step 1: Implement the command**

Create `cmd/tui.go`:

```go
package cmd

import (
	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/xico42/devenv/internal/project"
	"github.com/xico42/devenv/internal/tmux"
	"github.com/xico42/devenv/internal/tui"
	"github.com/xico42/devenv/internal/worktree"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive terminal dashboard",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		tmuxRunner := tmux.NewRealRunner()
		tmuxClient := tmux.NewClient(tmuxRunner)
		wtSvc := worktree.NewService(cfg, worktree.NewRealWorktreeRunner(), tmuxClient)
		sesSvc := newSessionService()
		projSvc := project.NewService(cfg, project.NewRealGitRunner())

		m := tui.NewModel(cfg, wtSvc, sesSvc, projSvc, tmuxClient, sessionsDir())
		p := tea.NewProgram(m, tea.WithAltScreen())

		finalModel, err := p.Run()
		if err != nil {
			return err
		}

		// If the user requested an attach, exec into tmux.
		if fm, ok := finalModel.(tui.Model); ok && fm.PendingAttach != "" {
			return execTmuxAttach(fm.PendingAttach)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
```

**Step 2: Build and verify**

```bash
go build ./...
```

Expected: compiles without errors.

**Step 3: Run full test suite**

```bash
make test
```

Expected: all tests pass.

**Step 4: Run coverage check**

```bash
make coverage
```

Expected: coverage >= 80%. If below, add tests to `internal/tui/` package — focus on `buildItems` edge cases and delegate rendering.

**Step 5: Lint**

```bash
make lint
```

Fix any issues.

**Step 6: Commit**

```bash
git add cmd/tui.go
git commit -m "feat: add devenv tui command"
```

---

### Post-implementation checklist

After all tasks are complete:

1. **Manual smoke test**: Run `devenv tui` with at least one configured project. Verify:
   - List shows projects with correct grouping
   - `/` activates filtering
   - `?` toggles help
   - `a` on a worktree starts agent and attaches
   - `s` creates shell session and attaches
   - `n` opens new worktree form
   - `d` shows delete confirmation
   - `c` clones an uncloned project
   - `q` quits
   - Auto-refresh updates after 3 seconds

2. **Mosh test**: Connect via mosh and verify:
   - Colors render correctly (256 color)
   - Navigation works (j/k, arrows)
   - No visual artifacts on resize

3. **Coverage**: `make coverage` passes at >= 80%.

4. **Final commit**: If any smoke-test fixes were needed.

---

### API notes for implementer

**Bubble Tea v2 key differences from v1:**
- `View()` returns `tea.View` — use `tea.NewView(string)`.
- Key press messages are `tea.KeyPressMsg`, not `tea.KeyMsg`.
- `Init()` returns `tea.Cmd` (not `(tea.Model, tea.Cmd)`).
- Import: `tea "charm.land/bubbletea/v2"` (or `github.com/charmbracelet/bubbletea/v2`).

**Bubbles v2 key matching (IMPORTANT):**
- Use `key.Matches(msg, binding)` — a **package-level function**, NOT a method on the binding.
- Import `"charm.land/bubbles/v2/key"` wherever you match keys.
- Example: `case key.Matches(msg, m.keys.Quit):` — NOT `m.keys.Quit.Matches(msg)`.

**Bubbles v2 help:**
- `help.New()` creates the model.
- `h.SetWidth(w)` — method, not field assignment.
- `h.ShowAll` — bool field, toggle for short/full help.
- `h.View(keyMap)` returns string.

**Bubbles v2 list API (verify with `go doc`):**
- `list.New(items []list.Item, delegate list.ItemDelegate, width, height int) list.Model`
- `list.Item` interface: `FilterValue() string`
- `ItemDelegate` interface: `Height() int`, `Spacing() int`, `Update(tea.Msg, *list.Model) tea.Cmd`, `Render(w io.Writer, m list.Model, index int, listItem list.Item)`
- `m.list.SelectedItem()` returns `list.Item`
- `m.list.Index()` returns selected index
- `m.list.SetItems(items []list.Item)` — may return `tea.Cmd`; capture it if so
- `m.list.Select(index int)` or `m.list.SetCursor(index int)` — check which exists in v2
- `m.list.FilterState()` returns filter state enum (e.g., `list.Filtering`)
- `m.list.SetSize(w, h)`, `m.list.SetShowStatusBar(bool)`, `m.list.SetFilteringEnabled(bool)`
- `m.list.View()` returns `string` (not `tea.View`), wrap in `tea.NewView()`

**Bubbles v2 textinput:**
- `textinput.New()` creates the model.
- `textinput.DefaultKeyMap()` — function call in v2, not a variable.
- `textinput.DefaultStyles(isDark bool)` — takes a bool parameter in v2.

**Lipgloss v2:**
- `lipgloss.NewStyle()` — same as v1
- `lipgloss.Color("170")` — ANSI 256 colors (mosh-safe)
- `lipgloss.Width(s)` — rendered width of styled string

**Existing patterns to reuse:**
- `execTmuxAttach(name)` in `cmd/session.go:189` — `syscall.Exec` into tmux
- `newWorktreeService()` in `cmd/worktree.go:17` — service construction
- `newSessionService()` in `cmd/session.go:27` — service construction
- `sessionsDir()` in `cmd/session.go:22` — state directory path
- `resolveAgentCmd/resolveAgentEnv` in `cmd/session.go:32-51` — agent config resolution (duplicated in `actions.go:resolveAgentCommand` for TUI use)
