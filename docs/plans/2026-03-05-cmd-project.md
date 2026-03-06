# `devenv project` Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement `devenv project list|show|clone [--all]` — project management subcommands backed by a tested `internal/project` package.

**Architecture:** All business logic lives in `internal/project` (Service struct + GitRunner interface for mocking). `cmd/project.go` is thin orchestration: create service, call methods, format output with `tabwriter`.

**Tech Stack:** Go stdlib (`os/exec`, `text/tabwriter`, `os`, `path/filepath`), Cobra, existing `internal/config` package.

**Worktree:** `~/.config/superpowers/worktrees/remote-dev/feat-cmd-project`

**Design doc:** `docs/plans/2026-03-05-cmd-project-design.md`

---

## Task 1: Create `internal/project/project.go` — types, interface, service skeleton

**Files:**
- Create: `internal/project/project.go`

**Step 1: Create the file with all types and stubs**

```go
package project

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/xico42/devenv/internal/config"
)

// ErrAlreadyCloned is returned by Clone when the target path already exists.
var ErrAlreadyCloned = errors.New("already cloned")

// AlreadyClonedError carries the path that already exists.
type AlreadyClonedError struct{ Path string }

func (e *AlreadyClonedError) Error() string { return e.Path + " already exists, skipping" }
func (e *AlreadyClonedError) Unwrap() error { return ErrAlreadyCloned }

// GitRunner abstracts git clone execution to enable testing.
type GitRunner interface {
	Clone(repo, path, branch string) error
}

// RealGitRunner runs git commands via os/exec.
type RealGitRunner struct{}

// NewRealGitRunner returns a GitRunner backed by the system git binary.
func NewRealGitRunner() *RealGitRunner { return &RealGitRunner{} }

// Clone runs git clone. If branch is non-empty, passes --branch <branch>.
func (r *RealGitRunner) Clone(repo, path, branch string) error {
	args := []string{"clone"}
	if branch != "" {
		args = append(args, "--branch", branch)
	}
	args = append(args, repo, path)
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s", out)
	}
	return nil
}

// ProjectEntry is a project with its derived filesystem path and clone status.
type ProjectEntry struct {
	Name   string
	Config config.ProjectConfig
	Path   string // absolute path derived from repo URL + projects_dir
	Cloned bool   // true if Path exists on the filesystem
}

// CloneResult captures the outcome of a single clone attempt.
type CloneResult struct {
	Name string
	Err  error // nil=success, ErrAlreadyCloned=skipped, other=failure
}

// Service provides project management operations.
type Service struct {
	cfg *config.Config
	git GitRunner
}

// NewService creates a Service using the given config and GitRunner.
func NewService(cfg *config.Config, git GitRunner) *Service {
	return &Service{cfg: cfg, git: git}
}

// List returns all configured projects sorted by name. No filesystem access.
func (s *Service) List() []ProjectEntry {
	entries := make([]ProjectEntry, 0, len(s.cfg.Projects))
	for name, p := range s.cfg.Projects {
		var path string
		if rp, err := config.RepoPath(p.Repo); err == nil {
			path = filepath.Join(s.cfg.Defaults.ProjectsDir, rp)
		}
		entries = append(entries, ProjectEntry{
			Name:   name,
			Config: p,
			Path:   path,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// Show returns the full entry for a single project, including Cloned status.
func (s *Service) Show(name string) (ProjectEntry, error) {
	p, ok := s.cfg.Projects[name]
	if !ok {
		return ProjectEntry{}, fmt.Errorf("project %q is not configured", name)
	}
	repoPath, err := config.RepoPath(p.Repo)
	if err != nil {
		return ProjectEntry{}, fmt.Errorf("cannot parse repo URL %q: %w", p.Repo, err)
	}
	absPath := filepath.Join(s.cfg.Defaults.ProjectsDir, repoPath)
	_, statErr := os.Stat(absPath)
	return ProjectEntry{
		Name:   name,
		Config: p,
		Path:   absPath,
		Cloned: statErr == nil,
	}, nil
}

// Clone clones a single project into its derived path under projects_dir.
// Returns *AlreadyClonedError (wrapping ErrAlreadyCloned) if the path exists.
func (s *Service) Clone(name string) error {
	p, ok := s.cfg.Projects[name]
	if !ok {
		return fmt.Errorf("project %q is not configured", name)
	}
	repoPath, err := config.RepoPath(p.Repo)
	if err != nil {
		return fmt.Errorf("cannot parse repo URL %q: %w", p.Repo, err)
	}
	absPath := filepath.Join(s.cfg.Defaults.ProjectsDir, repoPath)
	if _, err := os.Stat(absPath); err == nil {
		return &AlreadyClonedError{Path: absPath}
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("creating parent directories: %w", err)
	}
	return s.git.Clone(p.Repo, absPath, p.DefaultBranch)
}

// CloneAll clones all configured projects in sorted order.
func (s *Service) CloneAll() []CloneResult {
	names := make([]string, 0, len(s.cfg.Projects))
	for name := range s.cfg.Projects {
		names = append(names, name)
	}
	sort.Strings(names)
	results := make([]CloneResult, 0, len(names))
	for _, name := range names {
		results = append(results, CloneResult{Name: name, Err: s.Clone(name)})
	}
	return results
}
```

**Step 2: Verify it compiles**

Run: `go build ./internal/project/...`
Expected: no output, exit 0

**Step 3: Commit**

```bash
git add internal/project/project.go
git commit -m "feat: add internal/project package skeleton"
```

---

## Task 2: Unit tests for `Service.List()`

**Files:**
- Create: `internal/project/project_test.go`

**Step 1: Write the failing test**

```go
package project_test

import (
	"testing"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/project"
)

// mockGitRunner records Clone calls and returns controlled errors.
type mockGitRunner struct {
	calls  []cloneCall
	errors map[string]error // keyed by repo
}

type cloneCall struct{ Repo, Path, Branch string }

func (m *mockGitRunner) Clone(repo, path, branch string) error {
	m.calls = append(m.calls, cloneCall{repo, path, branch})
	if m.errors != nil {
		return m.errors[repo]
	}
	return nil
}

func makeConfig(projectsDir string, projects map[string]config.ProjectConfig) *config.Config {
	cfg := &config.Config{}
	cfg.Defaults.ProjectsDir = projectsDir
	cfg.Projects = projects
	return cfg
}

func TestList_SortedByName(t *testing.T) {
	cfg := makeConfig("/home/user/projects", map[string]config.ProjectConfig{
		"zebra": {Repo: "git@github.com:user/zebra.git", DefaultBranch: "main"},
		"alpha": {Repo: "git@github.com:user/alpha.git", DefaultBranch: "develop"},
		"myapp": {Repo: "git@github.com:user/myapp.git"},
	})
	svc := project.NewService(cfg, &mockGitRunner{})
	entries := svc.List()

	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}
	if entries[0].Name != "alpha" || entries[1].Name != "myapp" || entries[2].Name != "zebra" {
		t.Errorf("wrong order: %v", []string{entries[0].Name, entries[1].Name, entries[2].Name})
	}
}

func TestList_PathDerivedFromRepo(t *testing.T) {
	cfg := makeConfig("/home/user/projects", map[string]config.ProjectConfig{
		"myapp": {Repo: "git@github.com:user/myapp.git"},
	})
	svc := project.NewService(cfg, &mockGitRunner{})
	entries := svc.List()

	want := "/home/user/projects/github.com/user/myapp"
	if entries[0].Path != want {
		t.Errorf("Path = %q, want %q", entries[0].Path, want)
	}
}

func TestList_ClonedAlwaysFalse(t *testing.T) {
	cfg := makeConfig("/home/user/projects", map[string]config.ProjectConfig{
		"myapp": {Repo: "git@github.com:user/myapp.git"},
	})
	svc := project.NewService(cfg, &mockGitRunner{})
	entries := svc.List()
	if entries[0].Cloned {
		t.Error("List should not check filesystem; Cloned should be false")
	}
}

func TestList_Empty(t *testing.T) {
	cfg := makeConfig("/home/user/projects", map[string]config.ProjectConfig{})
	svc := project.NewService(cfg, &mockGitRunner{})
	entries := svc.List()
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}
```

**Step 2: Run tests to verify they pass (List is already implemented)**

Run: `go test ./internal/project/... -run TestList -v`
Expected: all PASS

**Step 3: Commit**

```bash
git add internal/project/project_test.go
git commit -m "test: add Service.List unit tests"
```

---

## Task 3: Unit tests for `Service.Show()`

**Files:**
- Modify: `internal/project/project_test.go` (append tests)

**Step 1: Append tests**

```go
func TestShow_ValidProject(t *testing.T) {
	cfg := makeConfig("/home/user/projects", map[string]config.ProjectConfig{
		"myapp": {Repo: "git@github.com:user/myapp.git", DefaultBranch: "main"},
	})
	svc := project.NewService(cfg, &mockGitRunner{})
	e, err := svc.Show("myapp")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if e.Name != "myapp" {
		t.Errorf("Name = %q, want %q", e.Name, "myapp")
	}
	if e.Path != "/home/user/projects/github.com/user/myapp" {
		t.Errorf("Path = %q", e.Path)
	}
	// Cloned=false because path doesn't exist on this machine in tests
}

func TestShow_UnknownProject(t *testing.T) {
	cfg := makeConfig("/home/user/projects", map[string]config.ProjectConfig{})
	svc := project.NewService(cfg, &mockGitRunner{})
	_, err := svc.Show("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown project")
	}
}

func TestShow_ClonedTrue_WhenPathExists(t *testing.T) {
	dir := t.TempDir()
	cfg := makeConfig(dir, map[string]config.ProjectConfig{
		"myapp": {Repo: "git@github.com:user/myapp.git"},
	})
	// Create the expected path so Cloned=true
	expectedPath := dir + "/github.com/user/myapp"
	if err := os.MkdirAll(expectedPath, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := project.NewService(cfg, &mockGitRunner{})
	e, err := svc.Show("myapp")
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}
	if !e.Cloned {
		t.Error("Cloned should be true when path exists")
	}
}
```

Add `"os"` to the import block at the top of the test file.

**Step 2: Run tests**

Run: `go test ./internal/project/... -run TestShow -v`
Expected: all PASS

**Step 3: Commit**

```bash
git add internal/project/project_test.go
git commit -m "test: add Service.Show unit tests"
```

---

## Task 4: Unit tests for `Service.Clone()`

**Files:**
- Modify: `internal/project/project_test.go` (append tests)

**Step 1: Append tests**

```go
func TestClone_HappyPath(t *testing.T) {
	dir := t.TempDir()
	mock := &mockGitRunner{}
	cfg := makeConfig(dir, map[string]config.ProjectConfig{
		"myapp": {Repo: "git@github.com:user/myapp.git", DefaultBranch: "main"},
	})
	svc := project.NewService(cfg, mock)
	if err := svc.Clone("myapp"); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 git call, got %d", len(mock.calls))
	}
	call := mock.calls[0]
	if call.Repo != "git@github.com:user/myapp.git" {
		t.Errorf("Repo = %q", call.Repo)
	}
	if call.Path != dir+"/github.com/user/myapp" {
		t.Errorf("Path = %q", call.Path)
	}
	if call.Branch != "main" {
		t.Errorf("Branch = %q, want %q", call.Branch, "main")
	}
}

func TestClone_NoBranch(t *testing.T) {
	dir := t.TempDir()
	mock := &mockGitRunner{}
	cfg := makeConfig(dir, map[string]config.ProjectConfig{
		"myapp": {Repo: "git@github.com:user/myapp.git"},
	})
	svc := project.NewService(cfg, mock)
	if err := svc.Clone("myapp"); err != nil {
		t.Fatalf("Clone() error = %v", err)
	}
	if mock.calls[0].Branch != "" {
		t.Errorf("Branch should be empty when default_branch not set")
	}
}

func TestClone_AlreadyCloned(t *testing.T) {
	dir := t.TempDir()
	mock := &mockGitRunner{}
	cfg := makeConfig(dir, map[string]config.ProjectConfig{
		"myapp": {Repo: "git@github.com:user/myapp.git"},
	})
	// Pre-create the target path
	targetPath := dir + "/github.com/user/myapp"
	if err := os.MkdirAll(targetPath, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := project.NewService(cfg, mock)
	err := svc.Clone("myapp")
	if !errors.Is(err, project.ErrAlreadyCloned) {
		t.Fatalf("want ErrAlreadyCloned, got %v", err)
	}
	if len(mock.calls) != 0 {
		t.Error("git should not be called when path already exists")
	}
	var ace *project.AlreadyClonedError
	if !errors.As(err, &ace) {
		t.Fatal("want *AlreadyClonedError")
	}
	if ace.Path != targetPath {
		t.Errorf("AlreadyClonedError.Path = %q, want %q", ace.Path, targetPath)
	}
}

func TestClone_UnknownProject(t *testing.T) {
	cfg := makeConfig("/tmp", map[string]config.ProjectConfig{})
	svc := project.NewService(cfg, &mockGitRunner{})
	err := svc.Clone("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown project")
	}
}

func TestClone_GitFailure(t *testing.T) {
	dir := t.TempDir()
	gitErr := fmt.Errorf("repository not found")
	mock := &mockGitRunner{errors: map[string]error{
		"git@github.com:user/myapp.git": gitErr,
	}}
	cfg := makeConfig(dir, map[string]config.ProjectConfig{
		"myapp": {Repo: "git@github.com:user/myapp.git"},
	})
	svc := project.NewService(cfg, mock)
	err := svc.Clone("myapp")
	if err == nil {
		t.Fatal("expected error on git failure")
	}
}
```

Add `"errors"` and `"fmt"` to the import block.

**Step 2: Run tests**

Run: `go test ./internal/project/... -run TestClone -v`
Expected: all PASS

**Step 3: Commit**

```bash
git add internal/project/project_test.go
git commit -m "test: add Service.Clone unit tests"
```

---

## Task 5: Unit tests for `Service.CloneAll()`

**Files:**
- Modify: `internal/project/project_test.go` (append tests)

**Step 1: Append tests**

```go
func TestCloneAll_MixedResults(t *testing.T) {
	dir := t.TempDir()
	mock := &mockGitRunner{errors: map[string]error{
		"git@github.com:user/fail.git": fmt.Errorf("auth failed"),
	}}
	// Pre-create path for "existing" project
	if err := os.MkdirAll(dir+"/github.com/user/existing", 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := makeConfig(dir, map[string]config.ProjectConfig{
		"alpha":    {Repo: "git@github.com:user/alpha.git"},
		"existing": {Repo: "git@github.com:user/existing.git"},
		"fail":     {Repo: "git@github.com:user/fail.git"},
	})
	svc := project.NewService(cfg, mock)
	results := svc.CloneAll()

	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
	// Results must be sorted: alpha, existing, fail
	if results[0].Name != "alpha" || results[0].Err != nil {
		t.Errorf("alpha: got name=%q err=%v", results[0].Name, results[0].Err)
	}
	if results[1].Name != "existing" || !errors.Is(results[1].Err, project.ErrAlreadyCloned) {
		t.Errorf("existing: got name=%q err=%v", results[1].Name, results[1].Err)
	}
	if results[2].Name != "fail" || results[2].Err == nil {
		t.Errorf("fail: got name=%q err=%v", results[2].Name, results[2].Err)
	}
	// Only "alpha" and "fail" should have triggered git calls
	if len(mock.calls) != 2 {
		t.Errorf("expected 2 git calls, got %d", len(mock.calls))
	}
}

func TestCloneAll_Empty(t *testing.T) {
	cfg := makeConfig("/tmp", map[string]config.ProjectConfig{})
	svc := project.NewService(cfg, &mockGitRunner{})
	results := svc.CloneAll()
	if len(results) != 0 {
		t.Errorf("got %d results, want 0", len(results))
	}
}
```

**Step 2: Run all internal/project tests**

Run: `go test ./internal/project/... -v`
Expected: all PASS

**Step 3: Commit**

```bash
git add internal/project/project_test.go
git commit -m "test: add Service.CloneAll unit tests"
```

---

## Task 6: Wire `cmd/project.go` — `list` subcommand

**Files:**
- Modify: `cmd/project.go`

**Step 1: Replace the stub with the wired command**

Replace the entire file:

```go
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

func init() {
	projectCmd.AddCommand(projectListCmd)
	rootCmd.AddCommand(projectCmd)
}
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: no output, exit 0

**Step 3: Commit**

```bash
git add cmd/project.go
git commit -m "feat: implement devenv project list"
```

---

## Task 7: Wire `show` subcommand

**Files:**
- Modify: `cmd/project.go`

**Step 1: Add showCmd — append to the `init()` block and add the var before it**

Add the following `var` before `func init()`:

```go
var projectShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show config for a project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		svc := project.NewService(cfg, project.NewRealGitRunner())
		e, err := svc.Show(args[0])
		if err != nil {
			return err
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
```

In `init()`, add:
```go
projectCmd.AddCommand(projectShowCmd)
```

**Step 2: Verify it compiles**

Run: `go build ./...`
Expected: no output, exit 0

**Step 3: Commit**

```bash
git add cmd/project.go
git commit -m "feat: implement devenv project show"
```

---

## Task 8: Wire `clone` subcommand with `--all`

**Files:**
- Modify: `cmd/project.go`

**Step 1: Add cloneAll flag var and cloneCmd — append before `func init()`**

```go
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
				case errors.Is(r.Err, project.ErrAlreadyCloned):
					var ace *project.AlreadyClonedError
					errors.As(r.Err, &ace)
					fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", ace)
				default:
					fmt.Fprintf(cmd.ErrOrStderr(), "Error: failed to clone %s: %v\n", r.Name, r.Err)
					hadFailure = true
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
		case errors.Is(err, project.ErrAlreadyCloned):
			fmt.Fprintln(cmd.OutOrStdout()) // newline after "Cloning..."
			var ace *project.AlreadyClonedError
			errors.As(err, &ace)
			fmt.Fprintf(cmd.OutOrStdout(), "Warning: %s\n", ace)
		default:
			fmt.Fprintln(cmd.OutOrStdout()) // newline after "Cloning..."
			return fmt.Errorf("failed to clone %s: %w", name, err)
		}
		return nil
	},
}
```

In `init()`, add:
```go
projectCloneCmd.Flags().BoolVar(&cloneAll, "all", false, "clone all configured projects")
projectCmd.AddCommand(projectCloneCmd)
```

**Step 2: Verify it compiles and all tests pass**

Run: `go build ./... && go test ./...`
Expected: build clean, all tests PASS

**Step 3: Commit**

```bash
git add cmd/project.go
git commit -m "feat: implement devenv project clone [--all]"
```

---

## Task 9: Coverage check and final verification

**Step 1: Run coverage**

Run: `make coverage`
Expected: aggregate coverage >= 80%, target exits 0

If coverage is below 80%: add tests to `internal/project/project_test.go` for any uncovered branches (check the coverage report output for which lines are missed), then re-run.

**Step 2: Run lint**

Run: `make lint`
Expected: no issues

**Step 3: Final commit if any coverage fixes were needed**

```bash
git add internal/project/project_test.go
git commit -m "test: improve coverage for internal/project"
```

---

## Summary

| Task | Deliverable |
|------|-------------|
| 1 | `internal/project/project.go` — all types, interface, service methods |
| 2–5 | `internal/project/project_test.go` — full unit test suite with mockGitRunner |
| 6 | `cmd/project.go` — `list` subcommand |
| 7 | `cmd/project.go` — `show` subcommand |
| 8 | `cmd/project.go` — `clone [--all]` subcommand |
| 9 | Coverage + lint verification |
