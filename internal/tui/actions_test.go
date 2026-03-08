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
	// Can't delete a project-only item — stays on list with a status message.
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d (should stay on list for project items)", um.screen, screenList)
	}
	if um.statusMsg == "" {
		t.Error("statusMsg should be set when trying to delete a project item")
	}
}

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
	if um.confirm == nil {
		t.Fatal("confirm should be set after startDelete on worktree item")
	}
	if um.confirm.target.Project != "myapp" || um.confirm.target.Branch != "feat" {
		t.Errorf("confirm.target = %v/%v, want myapp/feat", um.confirm.target.Project, um.confirm.target.Branch)
	}
}

// ── confirmModel unit tests ──────────────────────────────────────────────────

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
	if c.choices[0].action != deleteAll {
		t.Errorf("choices[0].action = %v, want deleteAll", c.choices[0].action)
	}
	if c.choices[1].action != deleteAgent {
		t.Errorf("choices[1].action = %v, want deleteAgent", c.choices[1].action)
	}
	if c.choices[2].action != deleteShell {
		t.Errorf("choices[2].action = %v, want deleteShell", c.choices[2].action)
	}
	if c.choices[3].action != deleteCancel {
		t.Errorf("choices[3].action = %v, want deleteCancel", c.choices[3].action)
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

	// j at bottom stays at last choice index
	c2 := newConfirmModel(target)
	lastIdx := len(c2.choices) - 1
	for i := 0; i < lastIdx+2; i++ {
		c2, _ = c2.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	}
	if c2.cursor != lastIdx {
		t.Errorf("cursor after j past bottom = %d, want %d", c2.cursor, lastIdx)
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

// ── updateConfirmDelete integration tests ────────────────────────────────────

func TestUpdateConfirmDelete_esc(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{screen: screenConfirmDelete}
	m.confirm = newConfirmModel(target)
	m.list = newList(nil)

	updated, _ := m.updateConfirmDelete(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after esc", um.screen, screenList)
	}
	if um.confirm != nil {
		t.Error("confirm should be nil after esc")
	}
}

func TestUpdateConfirmDelete_enterDeleteAll(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{screen: screenConfirmDelete}
	m.confirm = newConfirmModel(target)
	m.list = newList(nil)

	// cursor starts at 0 (deleteAll) — press enter immediately
	updated, cmd := m.updateConfirmDelete(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after enter with deleteAll", um.screen, screenList)
	}
	if um.confirm != nil {
		t.Error("confirm should be nil after successful delete")
	}
	_ = cmd // delete cmd — not called (services are nil)
}

func TestUpdateConfirmDelete_enterCancel(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{screen: screenConfirmDelete}
	m.confirm = newConfirmModel(target)
	m.list = newList(nil)

	// Move to cancel choice (last item).
	m.confirm, _ = m.confirm.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))

	updated, _ := m.updateConfirmDelete(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	um := updated.(Model)
	// Cancel selection should go back to list.
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after cancel selection", um.screen, screenList)
	}
	if um.confirm != nil {
		t.Error("confirm should be nil after cancel")
	}
}

func TestUpdateConfirmDelete_nonKeyMsg(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{screen: screenConfirmDelete}
	m.confirm = newConfirmModel(target)
	m.list = newList(nil)

	updated, cmd := m.updateConfirmDelete(cloneDoneMsg{project: "x"})
	um := updated.(Model)
	if um.screen != screenConfirmDelete {
		t.Errorf("screen = %d, want %d for non-key msg", um.screen, screenConfirmDelete)
	}
	_ = cmd
}

func TestConfirmDeleteNo(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree}
	m := Model{screen: screenConfirmDelete}
	m.confirm = newConfirmModel(target)
	m.list = newList(nil)

	updated, _ := m.confirmDeleteNo()
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after cancel", um.screen, screenList)
	}
	if um.confirm != nil {
		t.Error("confirm should be nil after cancel")
	}
}

// ── Clone / Attach / Shell action tests ─────────────────────────────────────

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
}

func TestResolveAgentEnv_empty(t *testing.T) {
	cfg := &config.Config{}
	env := resolveAgentEnv(cfg, "myapp")
	if env == nil {
		t.Error("resolveAgentEnv should return non-nil map")
	}
	if len(env) != 0 {
		t.Errorf("resolveAgentEnv returned %d entries, want 0 for empty config", len(env))
	}
}

func TestResolveAgentEnv_withEnv(t *testing.T) {
	cfg := &config.Config{
		Projects: map[string]config.ProjectConfig{
			"myapp": {
				Agent: config.AgentConfig{
					Env: map[string]string{
						"MY_VAR": "hello",
						"OTHER":  "world",
					},
				},
			},
		},
	}
	env := resolveAgentEnv(cfg, "myapp")
	if env["MY_VAR"] != "hello" {
		t.Errorf("MY_VAR = %q, want %q", env["MY_VAR"], "hello")
	}
	if env["OTHER"] != "world" {
		t.Errorf("OTHER = %q, want %q", env["OTHER"], "world")
	}
}

// ── confirmDeleteAgent / confirmDeleteShell ──────────────────────────────────

func TestConfirmDeleteAgent_returnsCmd(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupAgent, HasAgent: true}
	m := Model{screen: screenConfirmDelete}
	m.confirm = newConfirmModel(target)
	m.list = newList(nil)
	// tmuxClient nil — confirmDeleteAgent builds a cmd; calling it would panic on HasSession.
	// We just verify the model state transitions correctly.
	updated, cmd := m.confirmDeleteAgent()
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after confirmDeleteAgent", um.screen, screenList)
	}
	if um.confirm != nil {
		t.Error("confirm should be nil after confirmDeleteAgent")
	}
	if cmd == nil {
		t.Error("confirmDeleteAgent should return a non-nil cmd")
	}
}

func TestConfirmDeleteShell_returnsCmd(t *testing.T) {
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree, HasShell: true}
	m := Model{screen: screenConfirmDelete}
	m.confirm = newConfirmModel(target)
	m.list = newList(nil)
	// tmuxClient nil — confirmDeleteShell builds a cmd; calling it would panic on HasSession.
	updated, cmd := m.confirmDeleteShell()
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after confirmDeleteShell", um.screen, screenList)
	}
	if um.confirm != nil {
		t.Error("confirm should be nil after confirmDeleteShell")
	}
	if cmd == nil {
		t.Error("confirmDeleteShell should return a non-nil cmd")
	}
}

func TestUpdateConfirmDelete_enterDeleteAgent(t *testing.T) {
	// Build a target with an agent session so deleteAgent appears as a choice.
	target := Item{Project: "myapp", Branch: "feat", Group: groupAgent, HasAgent: true}
	m := Model{screen: screenConfirmDelete}
	m.confirm = newConfirmModel(target)
	m.list = newList(nil)

	// choices[0] = deleteAll, choices[1] = deleteAgent — move down once.
	m.confirm, _ = m.confirm.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if m.confirm.selected() != deleteAgent {
		t.Fatalf("precondition: selected = %v, want deleteAgent", m.confirm.selected())
	}

	updated, cmd := m.updateConfirmDelete(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want screenList after deleteAgent", um.screen)
	}
	if um.confirm != nil {
		t.Error("confirm should be nil after deleteAgent")
	}
	if cmd == nil {
		t.Error("deleteAgent should return non-nil cmd")
	}
}

func TestUpdateConfirmDelete_enterDeleteShell(t *testing.T) {
	// Build a target with both sessions so deleteShell appears as a choice.
	target := Item{Project: "myapp", Branch: "feat", Group: groupAgent, HasAgent: true, HasShell: true}
	m := Model{screen: screenConfirmDelete}
	m.confirm = newConfirmModel(target)
	m.list = newList(nil)

	// choices[0]=deleteAll, [1]=deleteAgent, [2]=deleteShell — move down twice.
	m.confirm, _ = m.confirm.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	m.confirm, _ = m.confirm.Update(tea.KeyPressMsg(tea.Key{Code: 'j', Text: "j"}))
	if m.confirm.selected() != deleteShell {
		t.Fatalf("precondition: selected = %v, want deleteShell", m.confirm.selected())
	}

	updated, cmd := m.updateConfirmDelete(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	um := updated.(Model)
	if um.screen != screenList {
		t.Errorf("screen = %d, want screenList after deleteShell", um.screen)
	}
	if um.confirm != nil {
		t.Error("confirm should be nil after deleteShell")
	}
	if cmd == nil {
		t.Error("deleteShell should return non-nil cmd")
	}
}

func TestConfirmView_shellOnly(t *testing.T) {
	// Cover the hasShell-only branch in newConfirmModel View.
	target := Item{Project: "myapp", Branch: "feat", Group: groupWorktree, HasShell: true}
	c := newConfirmModel(target)
	if len(c.choices) != 3 {
		t.Errorf("choices = %d, want 3 (deleteAll + deleteShell + cancel)", len(c.choices))
	}
	if c.choices[0].action != deleteAll {
		t.Errorf("choices[0] = %v, want deleteAll", c.choices[0].action)
	}
	if c.choices[1].action != deleteShell {
		t.Errorf("choices[1] = %v, want deleteShell", c.choices[1].action)
	}
	out := stripANSI(c.View())
	if !strings.Contains(out, "Shell session") {
		t.Errorf("View() should mention Shell session: %q", out)
	}
}
