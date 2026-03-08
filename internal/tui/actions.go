package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/semconv"
	"github.com/xico42/devenv/internal/session"
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

func resolveAgentEnv(cfg *config.Config, project string) map[string]string {
	agent := cfg.ResolveAgent(project)
	env := make(map[string]string)
	for k, v := range agent.Env {
		env[k] = v
	}
	return env
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
			env := resolveAgentEnv(cfg, project)
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
		// No worktree — need branch. Use default branch.
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
			env := resolveAgentEnv(cfg, project)
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
		// For group 3 (project-only), clone + create worktree first.
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
	if sel == nil {
		return m, nil
	}
	if sel.Group == groupProject {
		m.statusMsg = "cannot delete a project entry — select a worktree"
		return m, nil
	}
	if sel.IsMain {
		m.statusMsg = "cannot delete the main worktree"
		return m, nil
	}
	m.confirm = newConfirmModel(*sel)
	m.screen = screenConfirmDelete
	return m, nil
}

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
	tmuxClient := m.tmuxClient
	project := target.Project
	branch := target.Branch

	return m, func() tea.Msg {
		agentName := semconv.SessionName(project, branch)
		if running, _ := tmuxClient.HasSession(agentName); running {
			_ = sesSvc.Stop(agentName)
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
		if running, _ := tmuxClient.HasSession(shellName); running {
			_ = tmuxClient.KillSession(shellName)
		}
		return m.refreshCmd()()
	}
}

func (m Model) confirmDeleteNo() (tea.Model, tea.Cmd) {
	m.confirm = nil
	m.screen = screenList
	return m, nil
}
