//go:build integration

package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeSessionConfigWithProjectsDir writes a config pointing at a real
// projects_dir that has a cloned repo at the expected path.
func writeSessionConfigWithProjectsDir(t *testing.T, projectsDir string) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	content := `[defaults]
projects_dir = "` + projectsDir + `"
agent = "test-agent"

[agents.test-agent]
cmd = "true"

[projects.myapp]
repo = "git@github.com:user/myapp.git"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

// initBareRepo creates a minimal git repo at cloneDir suitable for
// `git worktree add` calls.
func initBareRepo(t *testing.T, cloneDir string) {
	t.Helper()
	os.MkdirAll(cloneDir, 0o755)
	cmds := [][]string{
		{"git", "init", cloneDir},
		{"git", "-C", cloneDir, "config", "user.email", "test@test.com"},
		{"git", "-C", cloneDir, "config", "user.name", "Test"},
		{"git", "-C", cloneDir, "commit", "--allow-empty", "-m", "init"},
	}
	for _, c := range cmds {
		if out, err := exec.Command(c[0], c[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("setup %v: %v\n%s", c, err, out)
		}
	}
}

// TestSessionStart_autoCreate_createsWorktreeAndStartsSession verifies that
// session start creates the worktree when it is missing and the project is
// cloned. Requires tmux to be available.
func TestSessionStart_autoCreate_createsWorktreeAndStartsSession(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	projectsDir := t.TempDir()
	cloneDir := filepath.Join(projectsDir, "github.com", "user", "myapp")
	initBareRepo(t, cloneDir)

	cfgPath := writeSessionConfigWithProjectsDir(t, projectsDir)

	// No worktree exists yet — session start should create it automatically.
	err := runCmd(t, "--config", cfgPath, "session", "start", "myapp", "feat")
	if err != nil {
		t.Fatalf("session start with auto-create = %v, want nil", err)
	}

	// Clean up the tmux session.
	exec.Command("tmux", "kill-session", "-t", "myapp-feat").Run()
}

// TestSessionStart_noCreate_failsWhenWorktreeMissing verifies that --no-create
// causes the command to exit non-zero when the worktree does not exist.
// Because sessionErr calls os.Exit(1), we test this via a subprocess.
func TestSessionStart_noCreate_failsWhenWorktreeMissing(t *testing.T) {
	projectsDir := t.TempDir()
	cloneDir := filepath.Join(projectsDir, "github.com", "user", "myapp")
	initBareRepo(t, cloneDir)

	cfgPath := writeSessionConfigWithProjectsDir(t, projectsDir)

	// Re-exec this test binary as a subprocess so os.Exit(1) doesn't kill the test.
	cmd := exec.Command(os.Args[0],
		"-test.run=TestSessionStart_noCreate_subprocess",
		"-test.v",
	)
	cmd.Env = append(os.Environ(),
		"DEVENV_TEST_SUBPROCESS=1",
		"DEVENV_TEST_CFG="+cfgPath,
	)
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit, got nil")
	}
}

func TestSessionStart_noCreate_subprocess(t *testing.T) {
	if os.Getenv("DEVENV_TEST_SUBPROCESS") != "1" {
		t.Skip("not a subprocess invocation")
	}
	cfgPath := os.Getenv("DEVENV_TEST_CFG")
	runCmd(t, "--config", cfgPath, "session", "start", "--no-create", "myapp", "feat") //nolint:errcheck
}
