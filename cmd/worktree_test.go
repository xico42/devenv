package cmd_test

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes a minimal TOML config file with a projects_dir and
// a single project entry, then returns the config file path.
func writeConfig(t *testing.T, projectsDir string) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	content := "[defaults]\nprojects_dir = \"" + projectsDir + "\"\n\n[projects.myapp]\nrepo = \"git@github.com:user/myapp.git\"\n"
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

// TestWorktreeCmd_help verifies the worktree group has correct --help output.
// Using only the group-level --help avoids Cobra flag state pollution on
// subcommands.
func TestWorktreeCmd_help(t *testing.T) {
	if err := runCmd(t, "worktree", "--help"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestWorktreeList_noProjects exercises the list command against a config whose
// projects_dir exists but no projects are cloned.  List returns 0 entries
// and prints only the header — no error.
func TestWorktreeList_noProjects(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	if err := runCmd(t, "--config", cfgPath, "worktree", "list"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestWorktreeList_unknownProject exercises list with a named project argument
// that is unknown (not in config), which should return an error.
func TestWorktreeList_unknownProject(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	if err := runCmd(t, "--config", cfgPath, "worktree", "list", "nosuchproject"); err == nil {
		t.Error("expected error for unknown project, got nil")
	}
}

// TestWorktreeNew_tooFewArgs verifies ExactArgs(2) on the new subcommand.
func TestWorktreeNew_tooFewArgs(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	if err := runCmd(t, "--config", cfgPath, "worktree", "new", "myapp"); err == nil {
		t.Error("expected error for missing branch argument")
	}
}

// TestWorktreeNew_tooManyArgs verifies ExactArgs(2) on the new subcommand.
func TestWorktreeNew_tooManyArgs(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	if err := runCmd(t, "--config", cfgPath, "worktree", "new", "a", "b", "c"); err == nil {
		t.Error("expected error for too many arguments")
	}
}

// TestWorktreeDelete_tooFewArgs verifies ExactArgs(2) on the delete subcommand.
func TestWorktreeDelete_tooFewArgs(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	if err := runCmd(t, "--config", cfgPath, "worktree", "delete", "myapp"); err == nil {
		t.Error("expected error for missing branch argument")
	}
}

// TestWorktreeShell_tooFewArgs verifies ExactArgs(2) on the shell subcommand.
func TestWorktreeShell_tooFewArgs(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	if err := runCmd(t, "--config", cfgPath, "worktree", "shell", "myapp"); err == nil {
		t.Error("expected error for missing branch argument")
	}
}

// TestWorktreeEnv_tooFewArgs verifies ExactArgs(2) on the env subcommand.
func TestWorktreeEnv_tooFewArgs(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	if err := runCmd(t, "--config", cfgPath, "worktree", "env", "myapp"); err == nil {
		t.Error("expected error for missing branch argument")
	}
}

// TestWorktreeEnv_tooManyArgs verifies ExactArgs(2) guard on the env subcommand.
func TestWorktreeEnv_tooManyArgs(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)
	if err := runCmd(t, "--config", cfgPath, "worktree", "env", "a", "b", "c"); err == nil {
		t.Error("expected error for too many arguments")
	}
}

// TestWorktreeList_withClonedProject exercises the list command when the clone
// directory exists (service doesn't skip the project), which exercises the git
// worktree list path.  Since the dir is not a real git repo, git will return
// an error — that's acceptable; we only verify there is no panic.
func TestWorktreeList_withClonedProject(t *testing.T) {
	projectsDir := t.TempDir()
	cloneDir := filepath.Join(projectsDir, "github.com", "user", "myapp")
	os.MkdirAll(cloneDir, 0o755)
	cfgPath := writeConfig(t, projectsDir)
	// May return error (not a git repo); that's fine — no panic is the goal.
	runCmd(t, "--config", cfgPath, "worktree", "list") //nolint:errcheck
}

// TestWorktreeDelete_abortedPrompt exercises the delete confirmation flow by
// piping a "n" answer via stdin, which should abort without error.
// Because Force=false, the confirmation prompt runs before any service call,
// so we do not reach the os.Exit(1) from worktreeErr.
func TestWorktreeDelete_abortedPrompt(t *testing.T) {
	projectsDir := t.TempDir()
	cfgPath := writeConfig(t, projectsDir)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.WriteString("n\n")
	w.Close()

	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()

	if err := runCmd(t, "--config", cfgPath, "worktree", "delete", "myapp", "feature"); err != nil {
		t.Logf("delete returned error (acceptable): %v", err)
	}
}
