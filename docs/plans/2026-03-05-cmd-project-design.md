# Design: `devenv project` command

Date: 2026-03-05

## Overview

Implements `devenv project list|show|clone` as specified in `docs/prds/prd-phase-02-cmd-project.md`. All business logic lives in `internal/project`; `cmd/project.go` is thin orchestration only.

## `internal/project` package

### `GitRunner` interface

```go
type GitRunner interface {
    Clone(repo, path, branch string) error
}
```

`RealGitRunner` implements this via `os/exec`. When `branch` is non-empty, passes `--branch <branch>` to `git clone`. The interface exists solely to enable mocking in unit tests.

### `Service`

```go
type Service struct {
    cfg *config.Config
    git GitRunner
}

func NewService(cfg *config.Config, git GitRunner) *Service
```

### Methods

- `List() []ProjectEntry` — returns all configured projects sorted by name; no filesystem access
- `Show(name string) (ProjectEntry, error)` — returns project config + derived path + `Cloned bool` (via `os.Stat`); errors on unknown project or unparseable URL
- `Clone(name string) error` — derives path via `config.RepoPath`, `os.MkdirAll`, delegates to `GitRunner.Clone`; returns `ErrAlreadyCloned` sentinel when path already exists
- `CloneAll() []CloneResult` — iterates all projects in sorted order, calls `Clone` on each, collects per-project results

### Types

```go
type ProjectEntry struct {
    Name   string
    Config config.ProjectConfig
    Path   string   // absolute path derived from repo URL + projects_dir
    Cloned bool     // true if Path exists on filesystem
}

type CloneResult struct {
    Name string
    Err  error  // nil = success, ErrAlreadyCloned = skipped, other = failure
}

var ErrAlreadyCloned = errors.New("already cloned")
```

## `cmd/project.go`

Subcommand tree:

```
projectCmd
├── listCmd        no args; tabwriter table: NAME / REPO / BRANCH
├── showCmd        <name>; tabwriter key-value: Name, Repo, Branch, Path, Cloned
└── cloneCmd       <name> | --all; progress lines per project
```

Each command constructs `project.NewService(cfg, project.NewRealGitRunner())` and delegates. Output format:

**list:**
```
NAME        REPO                                    BRANCH
myapp       git@github.com:user/myapp.git           main
api         git@github.com:user/api.git             develop
```

**show:**
```
Name:    myapp
Repo:    git@github.com:user/myapp.git
Branch:  main
Path:    ~/projects/github.com/user/myapp
Cloned:  yes
```

**clone:**
```
Cloning myapp...    done
Warning: /home/user/projects/github.com/user/api already exists, skipping.
Error: failed to clone work-api: <git stderr>
```

Exit codes: 0 for success and already-cloned warnings; 1 for unknown project, git failure, or unparseable URL.

## Testing

Unit tests in `internal/project/service_test.go` using a `mockGitRunner`:

- `List`: sorted order, all fields populated correctly
- `Show`: valid project with and without existing path (Cloned true/false), unknown project error
- `Clone`: happy path (mock called with correct repo/path/branch), `ErrAlreadyCloned` when path exists, unknown project, bad URL, git failure propagated
- `CloneAll`: mix of success, already-cloned, and failure results

No integration tests — `RepoPath` is already covered; `RealGitRunner` is a trivial `os/exec` wrapper.

Coverage target: maintains >= 80% aggregate.
