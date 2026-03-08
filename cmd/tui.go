package cmd

import (
	"fmt"

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
		p := tea.NewProgram(m)

		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("tui: %w", err)
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
