# PRD: `devenv project`

## Overview

The `project` command manages project definitions and their presence on the active droplet. Projects are configured locally in `~/.config/devenv/config.toml` and cloned manually onto the droplet on demand.

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
| `clone <name>` | Clone a project's repo onto the active droplet |
| `clone --all` | Clone all configured projects onto the active droplet |

---

## Config Schema

Projects are defined in `~/.config/devenv/config.toml`:

```toml
[projects.myapp]
repo           = "git@github.com:user/myapp.git"
default_branch = "main"

[projects.api]
repo           = "git@github.com:user/api.git"
default_branch = "develop"
```

### Fields

| Field | Required | Description |
|---|---|---|
| `repo` | yes | Git remote URL (SSH or HTTPS) |
| `default_branch` | no | Branch to check out on clone (default: repo's default branch) |

---

## `devenv project list`

Lists all projects defined in local config. No droplet connection required.

```
devenv project list

  NAME      REPO                                 BRANCH
  myapp     git@github.com:user/myapp.git        main
  api       git@github.com:user/api.git          develop
```

---

## `devenv project show <name>`

Shows full config for a single project.

```
devenv project show myapp

  Name:    myapp
  Repo:    git@github.com:user/myapp.git
  Branch:  main
```

---

## `devenv project clone <name>`

Clones the project's repo onto the active droplet.

```
devenv project clone myapp

Cloning myapp onto devenv-20260304-143012...  ✓
  Path: ~/projects/myapp
```

### Behavior

- Requires an active droplet (reads from `~/.local/share/devenv/state.json`)
- SSHes into the droplet and runs `git clone <repo> ~/projects/<name>`
- If `default_branch` is set, checks out that branch after cloning
- If `~/projects/<name>` already exists: prints a warning and exits 0 — does not re-clone or overwrite

### `--all` flag

Clones all configured projects sequentially. Skips already-cloned ones with a warning.

```
devenv project clone --all

Cloning myapp...   ✓
Cloning api...     ✓
```

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| No active droplet | `Error: no active droplet. Run 'devenv up' first.` | 1 |
| Project not in config | `Error: project "foo" is not configured.` | 1 |
| Already cloned | `Warning: ~/projects/myapp already exists, skipping.` | 0 |
| git clone fails | `Error: failed to clone myapp: <git error>` | 1 |

---

## Implementation Notes

- All remote operations are SSH commands over the Tailscale IP using the same connection logic as `devenv ssh`
- `git clone` on the droplet uses the git identity copied by `devenv up` (see `prd-cmd-up.md`)
- Project config is local-only — there is no sync of project definitions to the droplet
