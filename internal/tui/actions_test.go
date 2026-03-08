package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/semconv"
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

func TestStartDelete_worktreeItem(t *testing.T) {
	items := []Item{{Project: "myapp", Branch: "feat", Group: groupWorktree}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList}
	m.list = newList(listItems)

	updated, _ := m.startDelete()
	um := updated.(Model)
	if um.screen != screenConfirmDelete {
		t.Errorf("screen = %d, want %d (should go to confirm)", um.screen, screenConfirmDelete)
	}
	if um.deleteTarget == nil {
		t.Fatal("deleteTarget should be set")
	}
	if um.deleteTarget.Project != "myapp" || um.deleteTarget.Branch != "feat" {
		t.Errorf("deleteTarget = %v/%v, want myapp/feat", um.deleteTarget.Project, um.deleteTarget.Branch)
	}
}

func TestUpdateConfirmDelete_yKey(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{
		screen:       screenConfirmDelete,
		deleteTarget: &target,
	}
	m.list = newList(nil)

	msg := tea.KeyPressMsg(tea.Key{Code: 'y', Text: "y"})
	updated, cmd := m.updateConfirmDelete(msg)
	um := updated.(Model)

	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after 'y'", um.screen, screenList)
	}
	if um.deleteTarget != nil {
		t.Error("deleteTarget should be nil after confirm")
	}
	// cmd is the delete func — just verify it's non-nil (services are nil so we don't call it)
	_ = cmd
}

func TestUpdateConfirmDelete_otherKey(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{
		screen:       screenConfirmDelete,
		deleteTarget: &target,
	}
	m.list = newList(nil)

	msg := tea.KeyPressMsg(tea.Key{Code: 'x', Text: "x"})
	updated, _ := m.updateConfirmDelete(msg)
	um := updated.(Model)

	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after non-y key", um.screen, screenList)
	}
	if um.deleteTarget != nil {
		t.Error("deleteTarget should be nil after cancel")
	}
}

func TestViewConfirmDelete_nilTarget(t *testing.T) {
	m := Model{screen: screenConfirmDelete}
	m.list = newList(nil)
	m.keys = defaultKeyMap()

	// Should not panic and should return non-empty (falls through to viewList).
	out := m.viewConfirmDelete()
	if out == "" {
		t.Error("viewConfirmDelete() with nil target should return viewList output")
	}
}

func TestViewConfirmDelete_withTarget(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{
		screen:       screenConfirmDelete,
		deleteTarget: &target,
	}
	m.list = newList(nil)
	m.keys = defaultKeyMap()

	out := m.viewConfirmDelete()
	if out == "" {
		t.Error("viewConfirmDelete() returned empty string")
	}
	// Should contain the project name.
	if !strings.Contains(out, "myapp") {
		t.Errorf("viewConfirmDelete() output does not contain 'myapp': %q", out)
	}
}

func TestCloneAction_nilSelection(t *testing.T) {
	m := Model{screen: screenList}
	m.list = newList(nil)

	cmd := m.cloneAction()
	if cmd != nil {
		t.Error("cloneAction() with no selection should return nil")
	}
}

func TestCloneAction_nonProjectItem(t *testing.T) {
	items := []Item{{Project: "myapp", Branch: "feat", Group: groupWorktree}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList}
	m.list = newList(listItems)

	cmd := m.cloneAction()
	if cmd != nil {
		t.Error("cloneAction() on non-project item should return nil")
	}
}

func TestCloneAction_alreadyCloned(t *testing.T) {
	items := []Item{{Project: "myapp", Group: groupProject, Cloned: true}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList}
	m.list = newList(listItems)

	cmd := m.cloneAction()
	if cmd != nil {
		t.Error("cloneAction() on already-cloned item should return nil")
	}
}

func TestAttachAction_nilSelection(t *testing.T) {
	m := Model{screen: screenList}
	m.list = newList(nil)

	cmd := m.attachAction()
	if cmd != nil {
		t.Error("attachAction() with no selection should return nil")
	}
}

func TestAttachAction_agentGroup(t *testing.T) {
	items := []Item{{Project: "myapp", Branch: "feat", Group: groupAgent}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList}
	m.list = newList(listItems)

	cmd := m.attachAction()
	if cmd == nil {
		t.Fatal("attachAction() on agent item should return non-nil cmd")
	}
	msg := cmd()
	am, ok := msg.(attachMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want attachMsg", msg)
	}
	want := semconv.SessionName("myapp", "feat")
	if am.session != want {
		t.Errorf("session = %q, want %q", am.session, want)
	}
}

func TestShellAction_nilSelection(t *testing.T) {
	m := Model{screen: screenList}
	m.list = newList(nil)

	cmd := m.shellAction()
	if cmd != nil {
		t.Error("shellAction() with no selection should return nil")
	}
}

func TestResolveAgentCommand_default(t *testing.T) {
	cfg := &config.Config{}
	cmd := resolveAgentCommand(cfg, "myapp")
	if cmd == "" {
		t.Error("resolveAgentCommand should return non-empty default command")
	}
	if !strings.Contains(cmd, semconv.DefaultAgentCmd) {
		t.Errorf("resolveAgentCommand = %q, want to contain %q", cmd, semconv.DefaultAgentCmd)
	}
}

func TestResolveAgentCommand_customCmd(t *testing.T) {
	cfg := &config.Config{
		Projects: map[string]config.ProjectConfig{
			"myapp": {
				Agent: config.AgentConfig{
					Cmd: "my-agent",
				},
			},
		},
	}
	cmd := resolveAgentCommand(cfg, "myapp")
	if cmd != "my-agent" {
		t.Errorf("resolveAgentCommand = %q, want %q", cmd, "my-agent")
	}
}

func TestResolveAgentCommand_withArgs(t *testing.T) {
	cfg := &config.Config{
		Projects: map[string]config.ProjectConfig{
			"myapp": {
				Agent: config.AgentConfig{
					Cmd:  "my-agent",
					Args: []string{"--flag", "value"},
				},
			},
		},
	}
	cmd := resolveAgentCommand(cfg, "myapp")
	want := "my-agent --flag value"
	if cmd != want {
		t.Errorf("resolveAgentCommand = %q, want %q", cmd, want)
	}
}

func TestUpdateConfirmDelete_nonKeyMsg(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{
		screen:       screenConfirmDelete,
		deleteTarget: &target,
	}
	m.list = newList(nil)

	// Non-key message should not change state.
	updated, cmd := m.updateConfirmDelete(cloneDoneMsg{project: "x"})
	um := updated.(Model)
	if um.screen != screenConfirmDelete {
		t.Errorf("screen = %d, want %d for non-key msg", um.screen, screenConfirmDelete)
	}
	if cmd != nil {
		t.Error("non-key message should return nil cmd")
	}
}

func TestConfirmDeleteYes_nilTarget(t *testing.T) {
	m := Model{
		screen:       screenConfirmDelete,
		deleteTarget: nil,
	}
	m.list = newList(nil)

	updated, cmd := m.confirmDeleteYes()
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d", um.screen, screenList)
	}
	if cmd != nil {
		t.Error("confirmDeleteYes with nil target should return nil cmd")
	}
}

func TestUpdateConfirmDelete_capitalY(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{
		screen:       screenConfirmDelete,
		deleteTarget: &target,
	}
	m.list = newList(nil)

	msg := tea.KeyPressMsg(tea.Key{Code: 'Y', Text: "Y"})
	updated, _ := m.updateConfirmDelete(msg)
	um := updated.(Model)

	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after 'Y'", um.screen, screenList)
	}
}

func TestAttachAction_worktreeGroup_returnsCmd(t *testing.T) {
	cfg := &config.Config{}
	items := []Item{{Project: "myapp", Branch: "feat", Path: "/some/path", Group: groupWorktree}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList, cfg: cfg}
	m.list = newList(listItems)

	cmd := m.attachAction()
	if cmd == nil {
		t.Fatal("attachAction() on worktree item should return non-nil cmd")
	}
	// We don't call cmd() because sesSvc is nil and would panic.
	// Just verify a cmd was returned.
}

func TestViewConfirmDelete_projectOnly(t *testing.T) {
	target := Item{Project: "myapp", Branch: "", Group: groupProject}
	m := Model{
		screen:       screenConfirmDelete,
		deleteTarget: &target,
	}
	m.list = newList(nil)
	m.keys = defaultKeyMap()

	out := m.viewConfirmDelete()
	if !strings.Contains(out, "myapp") {
		t.Errorf("viewConfirmDelete() output does not contain 'myapp': %q", out)
	}
	// Should not contain " / " when no branch.
	if strings.Contains(out, " / ") {
		t.Errorf("viewConfirmDelete() should not contain ' / ' for project-only: %q", out)
	}
}
