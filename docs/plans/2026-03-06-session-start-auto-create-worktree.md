# Session Start Auto-Create Worktree Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** When `session start` finds the worktree missing, auto-create it and proceed rather than failing.

**Architecture:** All changes are confined to `cmd/session.go`. The existing `worktree.Service.New()` method is reused as-is for creation. A new `--no-create` flag preserves the old fail-fast behavior. No changes to any other package.

**Tech Stack:** Go, Cobra, existing `worktree.Service` and `session.Service`.

---

## Background

`sessionStartCmd.RunE` currently:

```
WorktreePath(project, branch) → error? → sessionErr → os.Exit(1)
                               → ok    → svc.Start(...)
```

After this change:

```
WorktreePath → ErrWorktreeNotFound + !--no-create → print + wtSvc.New() → svc.Start(...)
             → ErrWorktreeNotFound + --no-create  → sessionErr → os.Exit(1)
             → any other error                    → sessionErr → os.Exit(1) / return error
             → ok                                 → svc.Start(...)
```

---

## Task 1: Write the failing test for `--no-create` flag recognition

**Files:**
- Modify: `cmd/session_test.go`

**Step 1: Add the test**

Add to `cmd/session_test.go`:

```go
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
```

Also add `"strings"` to the import block in `cmd/session_test.go`.

**Step 2: Run to verify it fails**

```bash
go test ./cmd/... -run TestSessionStart_noCreateFlag_recognized -v
```

Expected: FAIL — `unknown flag: --no-create`

---

## Task 2: Wire the `--no-create` flag

**Files:**
- Modify: `cmd/session.go`

**Step 1: Add the flag variable**

After the existing `var sessionStartAttach bool` line (line 60), add:

```go
var sessionStartNoCreate bool
```

**Step 2: Register the flag in `init()`**

In the `init()` function, after the existing `sessionStartCmd.Flags().BoolVar(&sessionStartAttach, ...)` line, add:

```go
sessionStartCmd.Flags().BoolVar(&sessionStartNoCreate, "no-create", false, "fail if worktree does not exist instead of creating it")
```

**Step 3: Run the test — it should now pass**

```bash
go test ./cmd/... -run TestSessionStart_noCreateFlag_recognized -v
```

Expected: PASS

**Step 4: Run the full test suite to check nothing is broken**

```bash
make test
```

Expected: all tests pass

**Step 5: Commit**

```bash
git add cmd/session.go cmd/session_test.go
git commit -m "feat: add --no-create flag to session start"
```

---

## Task 3: Implement the auto-create logic

**Files:**
- Modify: `cmd/session.go`

The current `sessionStartCmd.RunE` resolves the path like this (lines 69–73):

```go
wtSvc := newWorktreeService()
path, err := wtSvc.WorktreePath(project, branch)
if err != nil {
    return sessionErr(cmd, err)
}
```

Replace those lines with:

```go
wtSvc := newWorktreeService()
path, err := wtSvc.WorktreePath(project, branch)
if err != nil {
    if errors.Is(err, worktree.ErrWorktreeNotFound) && !sessionStartNoCreate {
        fmt.Fprintf(cmd.OutOrStdout(), "Worktree %s/%s not found, creating...  ", project, branch)
        result, createErr := wtSvc.New(project, branch)
        if createErr != nil {
            fmt.Fprintln(cmd.OutOrStdout())
            return worktreeErr(cmd, project, branch, createErr)
        }
        fmt.Fprintln(cmd.OutOrStdout(), "done")
        path = result.Path
    } else {
        return sessionErr(cmd, err)
    }
}
```

**Step 1: Apply the change to `cmd/session.go`**

Make the replacement above. Verify `errors` is already imported (it is, at line 8).

**Step 2: Run the full test suite**

```bash
make test
```

Expected: all tests pass

**Step 3: Commit**

```bash
git add cmd/session.go
git commit -m "feat: auto-create worktree on session start if missing"
```

---

## Task 4: Verify coverage

**Step 1: Run coverage check**

```bash
make coverage
```

Expected: passes (≥80% aggregate). The new code paths through `ErrWorktreeNotFound`
in `sessionStartCmd` are exercised indirectly by existing tests (the
`TestSessionStart_unconfiguredProject` test still hits the `sessionErr` default branch;
the auto-create branch requires real git + tmux and is covered by integration tests in
Task 5).

If coverage drops below 80%, add a unit test that covers the `!sessionStartNoCreate`
branch by setting up a configured project where `wtSvc.New()` returns a non-sentinel
error (e.g. an unconfigured project name passed to `New` hits the default `worktreeErr`
branch and returns an error rather than calling `os.Exit`).

---

## Task 5: Integration test (build-tagged)

These tests require a real git repo and are tagged `integration`. They are optional for
the 80% threshold but document the intended end-to-end behavior.

**Files:**
- Create: `cmd/session_integration_test.go`

```go
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

[defaults.agent]
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
```

**Step 1: Create the file**

Write the file as above.

**Step 2: Run the integration tests**

```bash
make test-integration
```

Expected: integration tests pass (or skip if tmux unavailable in CI).

**Step 3: Commit**

```bash
git add cmd/session_integration_test.go
git commit -m "test: add integration tests for session start auto-create"
```

---

## Task 6: Final verification

```bash
make setup
```

Expected: deps → test → test-integration → lint → build all pass.
