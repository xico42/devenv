package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:     "ssh",
	Short:   "Open an interactive SSH session to the active droplet",
	GroupID: "remote",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("not implemented")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sshCmd)
}
