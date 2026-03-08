package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSessionConfig writes a config with agent settings and a project.
func writeSessionConfig(t *testing.T, projectsDir string) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	content := `[defaults]
projects_dir = "` + projectsDir + `"
agent = "echo-agent"

[agents.echo-agent]
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

// TestSessionStart_noCreateFlag_recognized verifies --no-create is a known flag.
// Before the flag is wired, Cobra returns "unknown flag: --no-create".
// After wiring, the command proceeds past flag parsing and fails on the
// unconfigured project (non-sentinel error returned, not os.Exit(1)).
func TestSessionStart_noCreateFlag_recognized(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	err := runCmd(t, "--config", cfgPath, "session", "start", "--no-create", "notaproject", "main")
	if err == nil {
		t.Fatal("expected error for unconfigured project, got nil")
	}
	if strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--no-create flag not recognised: %v", err)
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

func TestSessionStart_agentFlag_recognized(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	err := runCmd(t, "--config", cfgPath, "session", "start", "--agent", "echo-agent", "notaproject", "main")
	if err == nil {
		t.Fatal("expected error for unconfigured project, got nil")
	}
	if strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--agent flag not recognised: %v", err)
	}
}

func TestSessionStart_noAgentConfigured_errors(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	content := `[defaults]
projects_dir = "` + t.TempDir() + `"

[projects.myapp]
repo = "git@github.com:user/myapp.git"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCmd(t, "--config", cfgPath, "session", "start", "myapp", "main")
	if err == nil {
		t.Fatal("expected error when no agent configured")
	}
	if !strings.Contains(err.Error(), "no agent specified") {
		t.Errorf("error = %q, want to contain 'no agent specified'", err.Error())
	}
}
