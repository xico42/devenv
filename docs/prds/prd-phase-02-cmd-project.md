# PRD: `devenv project` (Phase 2 — Workload)

## Overview

The `project` command manages project definitions and their presence on the local filesystem. Projects are configured in `~/.config/devenv/config.toml` and cloned into `projects_dir` on demand.

**Plane:** Workload (runs anywhere — no SSH, no droplet required). Commands execute `git` directly on the machine where devenv is invoked.

---

## Command Interface

```
devenv project <subcommand> [flags]
```

### Subcommands

| Subcommand | Description |
|---|---|
| `list` | List all configured projects |
| `show <name>` | Show config for a project |
| `clone <name>` | Clone a project's repo into projects_dir |
| `clone --all` | Clone all configured projects |

---

## Config Schema

Projects are defined in `~/.config/devenv/config.toml`:

```toml
[defaults]
projects_dir = "~/projects"

[projects.myapp]
repo           = "git@github.com:user/myapp.git"
default_branch = "main"

[projects.api]
repo           = "git@github.com:user/api.git"
default_branch = "develop"

[projects.work-api]
repo           = "git@gitlab.com:corp/backend/api.git"
default_branch = "develop"
env_template   = "~/.config/devenv/templates/work-api.env.template"
```

### Fields

| Field | Required | Description |
|---|---|---|
| `repo` | yes | Git remote URL (SSH or HTTPS) |
| `default_branch` | no | Branch to check out on clone (default: repo's default branch) |
| `env_template` | no | Path to `.env.template` for projects where you don't version-control it |

---

## Directory Layout

The clone directory mirrors the repo URL path under `projects_dir`:

```
~/projects/
  github.com/
    user/
      myapp/                       <- clone of [projects.myapp]
      myapp__worktrees/            <- worktrees for myapp (see prd-phase-02-cmd-worktree.md)
        feature/
        fix-123/
      api/                         <- clone of [projects.api]
  gitlab.com/
    corp/
      backend/
        api/                       <- clone of [projects.work-api]
```

The repo URL is parsed to extract `<host>/<path>`:

| Repo URL | Clone path |
|---|---|
| `git@github.com:user/myapp.git` | `~/projects/github.com/user/myapp` |
| `git@gitlab.com:corp/backend/api.git` | `~/projects/gitlab.com/corp/backend/api` |
| `https://github.com/user/myapp.git` | `~/projects/github.com/user/myapp` |
| `ssh://git@github.com/user/myapp.git` | `~/projects/github.com/user/myapp` |

The `.git` suffix is always stripped. This convention matches `ghq` and Go module paths.

---

## `devenv project list`

Lists all projects defined in config. No network access required.

```
devenv project list

  NAME        REPO                                       BRANCH
  myapp       git@github.com:user/myapp.git              main
  api         git@github.com:user/api.git                develop
  work-api    git@gitlab.com:corp/backend/api.git        develop
```

---

## `devenv project show <name>`

Shows full config for a single project.

```
devenv project show myapp

  Name:    myapp
  Repo:    git@github.com:user/myapp.git
  Branch:  main
  Path:    ~/projects/github.com/user/myapp
  Cloned:  yes
```

The `Path` is derived from the repo URL. The `Cloned` field checks if that directory exists on the filesystem.

---

## `devenv project clone <name>`

Clones the project's repo into its derived path under `projects_dir`.

```
devenv project clone myapp

Cloning myapp...  done
  Path: ~/projects/github.com/user/myapp
```

### Behavior

- Derives the clone path from the repo URL: `<projects_dir>/<repo_path>`
- Creates all intermediate directories (e.g. `github.com/user/`)
- Runs `git clone <repo> <clone_path>` directly (no SSH — local execution)
- If `default_branch` is set, checks out that branch after cloning
- If `<clone_path>` already exists: prints a warning and exits 0 — does not re-clone or overwrite

### `--all` flag

Clones all configured projects sequentially. Skips already-cloned ones with a warning.

```
devenv project clone --all

Cloning myapp...       done
Cloning api...         done
Cloning work-api...    done
```

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| Project not in config | `Error: project "foo" is not configured.` | 1 |
| Already cloned | `Warning: ~/projects/github.com/user/myapp already exists, skipping.` | 0 |
| git clone fails | `Error: failed to clone myapp: <git error>` | 1 |
| Invalid repo URL | `Error: cannot parse repo URL "...": <details>` | 1 |

---

## Repo Path Derivation

A helper function `RepoPath(repoURL string) (string, error)` parses git remote URLs and returns the directory path. Supported formats:

| Format | Example | Result |
|---|---|---|
| SSH (scp-style) | `git@github.com:user/myapp.git` | `github.com/user/myapp` |
| SSH (URL-style) | `ssh://git@github.com/user/myapp.git` | `github.com/user/myapp` |
| HTTPS | `https://github.com/user/myapp.git` | `github.com/user/myapp` |

Rules:
- Strip `.git` suffix
- Strip leading `/` from path
- Preserve all path components (supports nested groups like GitLab's `corp/backend/api`)
- Error on unparseable URLs

This function lives in `internal/config` (or `internal/repopath` if it grows).

---

## Implementation Notes

- All operations are local filesystem + git commands — no SSH, no droplet state check
- `git clone` is invoked via `os/exec` — the command runs on whatever machine devenv is on
- On the droplet (after `devenv ssh`): clones use the git identity copied by `devenv up`
- On a local machine: clones use the local git/SSH configuration
- `projects_dir` is resolved from config at runtime (`~` expanded to `$HOME`)
- Intermediate directories are created with `os.MkdirAll` — no manual mkdir needed
- Project config is read-only by this command — use `devenv config set` to modify
