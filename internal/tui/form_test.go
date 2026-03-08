package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/xico42/devenv/internal/config"
)

var ansiEscapeRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiEscapeRe.ReplaceAllString(s, "")
}

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

func TestFormModel_emptyProjects(t *testing.T) {
	f := newFormModel(nil, nil, nil)

	if f.selectedProject() != "" {
		t.Errorf("selectedProject() with no projects = %q, want empty", f.selectedProject())
	}
	// Should not panic.
	f.nextProject()
	f.prevProject()
}

func TestFormModel_Update_enterKey_emptyProjects(t *testing.T) {
	f := newFormModel(nil, nil, nil)

	// With no projects, submit should return nil.
	f2, cmd := f.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	_ = f2
	if cmd != nil {
		t.Error("enter with no projects should return nil cmd")
	}
}

func TestFormModel_Update_tabKey(t *testing.T) {
	projects := []string{"api", "frontend"}
	f := newFormModel(projects, nil, nil)

	f2, _ := f.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if f2.selectedProject() != "frontend" {
		t.Errorf("after tab: project = %q, want %q", f2.selectedProject(), "frontend")
	}
}

func TestFormModel_Update_shiftTabKey(t *testing.T) {
	projects := []string{"api", "frontend"}
	f := newFormModel(projects, nil, nil)
	f.nextProject() // go to frontend

	f2, _ := f.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
	if f2.selectedProject() != "api" {
		t.Errorf("after shift+tab: project = %q, want %q", f2.selectedProject(), "api")
	}
}

func TestFormModel_Update_escKey(t *testing.T) {
	f := newFormModel([]string{"api"}, nil, nil)

	f2, cmd := f.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	_ = f2
	if cmd != nil {
		t.Error("esc key should return nil cmd")
	}
}

func TestFormModel_Update_enterKey_emptyBranch(t *testing.T) {
	f := newFormModel([]string{"api"}, nil, nil)

	f2, cmd := f.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	_ = f2
	// Empty branch means submit returns nil.
	if cmd != nil {
		t.Error("enter with empty branch should return nil cmd")
	}
}

func TestFormModel_View(t *testing.T) {
	projects := []string{"api", "frontend"}
	f := newFormModel(projects, nil, nil)

	out := f.View()
	if out == "" {
		t.Error("View() returned empty string")
	}
	// View renders with ANSI escape codes so use stripped comparison.
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "api") {
		t.Errorf("View() should contain project name 'api' (stripped): %q", stripped)
	}
	if !strings.Contains(stripped, "frontend") {
		t.Errorf("View() should contain project name 'frontend' (stripped): %q", stripped)
	}
}

func TestFormModel_View_multipleProjects(t *testing.T) {
	projects := []string{"api", "frontend", "worker"}
	f := newFormModel(projects, nil, nil)
	f.nextProject() // move to frontend

	out := f.View()
	stripped := stripANSI(out)
	if !strings.Contains(stripped, "frontend") {
		t.Errorf("View() should contain selected project 'frontend': %q", stripped)
	}
	if !strings.Contains(stripped, "api") {
		t.Errorf("View() should contain non-selected project 'api': %q", stripped)
	}
	if !strings.Contains(stripped, "worker") {
		t.Errorf("View() should contain non-selected project 'worker': %q", stripped)
	}
}

func TestShowForm_withProjects(t *testing.T) {
	m := Model{screen: screenList}
	m.list = newList(nil)
	m.cfg = &config.Config{
		Projects: map[string]config.ProjectConfig{
			"api":      {},
			"frontend": {},
		},
	}

	updated, _ := m.showForm()
	um := updated.(Model)
	if um.screen != screenForm {
		t.Errorf("screen = %d, want %d", um.screen, screenForm)
	}
	if um.form == nil {
		t.Error("form should be initialized after showForm()")
	}
}

func TestUpdateForm_escKey(t *testing.T) {
	m := Model{screen: screenForm}
	m.list = newList(nil)
	m.form = newFormModel([]string{"api"}, nil, nil)

	msg := tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	updated, _ := m.updateForm(msg)
	um := updated.(Model)

	if um.screen != screenList {
		t.Errorf("screen = %d, want %d after esc", um.screen, screenList)
	}
	if um.form != nil {
		t.Error("form should be nil after esc")
	}
}

func TestUpdateForm_nonEscKey(t *testing.T) {
	m := Model{screen: screenForm}
	m.list = newList(nil)
	m.form = newFormModel([]string{"api"}, nil, nil)

	msg := tea.KeyPressMsg(tea.Key{Code: 'a', Text: "a"})
	updated, _ := m.updateForm(msg)
	um := updated.(Model)

	if um.screen != screenForm {
		t.Errorf("screen = %d, want %d after non-esc", um.screen, screenForm)
	}
}
