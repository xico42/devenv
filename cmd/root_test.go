package cmd_test

import (
	"os"
	"testing"

	"github.com/xico42/devenv/cmd"
)

// runCmd sets os.Args to simulate a CLI invocation and calls Execute.
// It restores os.Args after the call.
func runCmd(t *testing.T, args ...string) error {
	t.Helper()
	orig := os.Args
	os.Args = append([]string{"devenv"}, args...)
	defer func() { os.Args = orig }()
	return cmd.Execute()
}

// TestExecute_Help exercises Execute() and all init() registrations by
// running --help, which Cobra handles internally and returns nil.
func TestExecute_Help(t *testing.T) {
	if err := runCmd(t, "--help"); err != nil {
		t.Errorf("Execute(--help) = %v, want nil", err)
	}
}

// TestExecute_Subcommands exercises the RunE closures of stub subcommands
// and the PersistentPreRunE that loads config.
// Each subcommand prints "not implemented" and returns nil.
func TestExecute_Subcommands(t *testing.T) {
	// Use a non-existent config path so Load() returns empty defaults (nil error).
	dir := t.TempDir()
	cfgPath := dir + "/config.toml"

	subcommands := []string{"up", "down", "status", "ssh", "config", "notify", "project", "worktree", "session"}
	for _, sub := range subcommands {
		t.Run(sub, func(t *testing.T) {
			err := runCmd(t, "--config", cfgPath, sub)
			if err != nil {
				t.Errorf("Execute(%q) = %v, want nil", sub, err)
			}
		})
	}
}
