package cmd

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xico42/devenv/internal/project"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured projects",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := project.NewService(cfg, project.NewRealGitRunner())
		entries := svc.List()
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "NAME\tREPO\tBRANCH")
		for _, e := range entries {
			fmt.Fprintf(w, "%s\t%s\t%s\n", e.Name, e.Config.Repo, e.Config.DefaultBranch)
		}
		return w.Flush()
	},
}

var projectShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show config for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := project.NewService(cfg, project.NewRealGitRunner())
		e, err := svc.Show(args[0])
		if err != nil {
			return fmt.Errorf("show project: %w", err)
		}
		cloned := "no"
		if e.Cloned {
			cloned = "yes"
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "Name:\t%s\n", e.Name)
		fmt.Fprintf(w, "Repo:\t%s\n", e.Config.Repo)
		fmt.Fprintf(w, "Branch:\t%s\n", e.Config.DefaultBranch)
		fmt.Fprintf(w, "Path:\t%s\n", e.Path)
		fmt.Fprintf(w, "Cloned:\t%s\n", cloned)
		return w.Flush()
	},
}

var cloneAll bool

var projectCloneCmd = &cobra.Command{
	Use:   "clone [<name>]",
	Short: "Clone a project's repo into projects_dir",
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := project.NewService(cfg, project.NewRealGitRunner())

		if cloneAll {
			results := svc.CloneAll()
			hadFailure := false
			for _, r := range results {
				switch {
				case r.Err == nil:
					fmt.Fprintf(cmd.OutOrStdout(), "Cloning %s... done\n", r.Name)
				default:
					var ace *project.AlreadyClonedError
					if errors.As(r.Err, &ace) {
						fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", ace)
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to clone %s: %v\n", r.Name, r.Err)
						hadFailure = true
					}
				}
			}
			if hadFailure {
				return fmt.Errorf("one or more clones failed")
			}
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("requires a project name, or use --all")
		}
		name := args[0]
		fmt.Fprintf(cmd.OutOrStdout(), "Cloning %s... ", name)
		err := svc.Clone(name)
		switch {
		case err == nil:
			fmt.Fprintln(cmd.OutOrStdout(), "done")
			if e, showErr := svc.Show(name); showErr == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  Path: %s\n", e.Path)
			}
		default:
			fmt.Fprintln(cmd.OutOrStdout()) // newline after "Cloning..."
			var ace *project.AlreadyClonedError
			if errors.As(err, &ace) {
				fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", ace)
			} else {
				return fmt.Errorf("failed to clone %s: %w", name, err)
			}
		}
		return nil
	},
}

func init() {
	projectCloneCmd.Flags().BoolVar(&cloneAll, "all", false, "clone all configured projects")
	projectCmd.AddCommand(projectCloneCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectShowCmd)
	rootCmd.AddCommand(projectCmd)
}
