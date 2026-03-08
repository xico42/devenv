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
