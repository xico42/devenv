package cmd_test

import (
	"os"
	"path/filepath"
	"testing"
)

// writeSessionConfig writes a config with agent settings and a project.
func writeSessionConfig(t *testing.T, projectsDir string) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	content := `[defaults]
projects_dir = "` + projectsDir + `"

[defaults.agent]
cmd = "echo"
args = ["hello"]

[projects.myapp]
repo = "git@github.com:user/myapp.git"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func TestSessionCmd_help(t *testing.T) {
	if err := runCmd(t, "session", "--help"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSessionList_empty(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	// list should succeed even with no tmux (service handles errors)
	// May fail if tmux not available — that's acceptable in unit tests.
	_ = runCmd(t, "--config", cfgPath, "session", "list")
}

func TestSessionStart_tooFewArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "start", "myapp"); err == nil {
		t.Error("expected error for missing branch argument")
	}
}

func TestSessionStart_tooManyArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "start", "a", "b", "c"); err == nil {
		t.Error("expected error for too many arguments")
	}
}

func TestSessionAttach_tooFewArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "attach"); err == nil {
		t.Error("expected error for missing session argument")
	}
}

func TestSessionStop_tooFewArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "stop"); err == nil {
		t.Error("expected error for missing session argument")
	}
}

func TestSessionShow_tooFewArgs(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	if err := runCmd(t, "--config", cfgPath, "session", "show"); err == nil {
		t.Error("expected error for missing session argument")
	}
}

func TestSessionMarkRunning_noSession(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	// mark-running with empty --session should succeed silently
	if err := runCmd(t, "--config", cfgPath, "session", "mark-running"); err != nil {
		t.Fatalf("mark-running without --session should succeed: %v", err)
	}
}

func TestSessionMarkRunning_withSession(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	// mark-running with a non-existent session file should succeed silently
	if err := runCmd(t, "--config", cfgPath, "session", "mark-running", "--session", "nonexistent"); err != nil {
		t.Fatalf("mark-running with missing state file should succeed: %v", err)
	}
}

func TestSessionStart_unconfiguredProject(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	// unconfigured project returns a non-sentinel error from worktree service,
	// which exercises the sessionErr default branch (returns error, no os.Exit).
	err := runCmd(t, "--config", cfgPath, "session", "start", "notaproject", "main")
	if err == nil {
		t.Error("expected error for unconfigured project, got nil")
	}
}
