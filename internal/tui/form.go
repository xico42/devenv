package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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
