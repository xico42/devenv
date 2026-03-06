# PRD: Environment Templates (Phase 2 — Workload)

## Overview

Environment template processing generates `.env` files for worktrees, solving port conflicts and project-specific configuration across parallel Claude agent sessions. Each worktree gets its own `.env` with deterministic, collision-free ports and project-specific variables.

**Plane:** Workload (runs anywhere — no SSH, no droplet required).

---

## Problem

When running 3+ parallel agent sessions, each with its own dev server (Docker Compose, Go, PHP), ports collide. The article solves this with a deterministic hash. Sprout solves it with random allocation + scanning. devenv needs a solution that:

- Works across heterogeneous projects (Docker, Go, PHP — each with different env var needs)
- Produces stable ports (same branch = same ports, even after droplet destroy/recreate)
- Requires no state files or scanning
- Fits the primitives-first philosophy

---

## Template Sources (precedence)

1. **Repo-local**: `<worktree_path>/.env.template` — versioned in the project repo. Always wins.
2. **Config per-project**: `projects.<name>.env_template` in `config.toml` — for projects where you don't control the repo or don't want to version-control the template.

If neither exists, no `.env` is generated (not an error).

```toml
[projects.myapp]
repo = "git@github.com:user/myapp.git"
default_branch = "main"
# no env_template — uses repo-local .env.template if present

[projects.work-api]
repo = "git@github.com:corp/api.git"
default_branch = "develop"
env_template = "~/.config/devenv/templates/work-api.env.template"
```

---

## Template Syntax

Go `text/template` with custom functions. The template produces a standard `.env` file (KEY=VALUE lines).

### Example `.env.template`

```
# Ports (deterministic per worktree)
API_PORT={{ port "api" }}
DB_PORT={{ port "db" }}
REDIS_PORT={{ port "redis" }}

# Context
APP_BRANCH={{ .Branch }}
DOCKER_NETWORK=devenv-{{ .Project }}-{{ .Branch }}

# Secrets (from shell env, with fallback)
JWT_SECRET={{ env "JWT_SECRET" "dev-secret-change-me" }}
DATABASE_URL=postgres://dev:dev@localhost:{{ port "db" }}/{{ .Project }}

# Docker Compose passthrough (not processed)
COMPOSE_PROJECT_NAME={{ .Project }}-{{ .Branch }}
```

### Template Context

```go
type EnvTemplateContext struct {
    Project      string // project name from config (e.g. "myapp")
    Branch       string // branch name (e.g. "feature")
    WorktreePath string // absolute path to worktree (e.g. "/home/ubuntu/projects/github.com/user/myapp__worktrees/feature")
    SessionName  string // "<project>-<branch>" (e.g. "myapp-feature")
}
```

### Template Functions

| Function | Signature | Description |
|---|---|---|
| `port` | `port "name"` | Deterministic port from `hash(project, branch, name)`. See Port Allocation below. |
| `env` | `env "VAR" "default"` | Reads `$VAR` from shell environment. Returns default if unset or empty. |

Only two functions. Intentionally minimal.

---

## Port Allocation

### Algorithm

Deterministic, hash-based, no state:

```go
func deterministicPort(project, branch, name string) int {
    key := project + "\x00" + branch + "\x00" + name
    h := fnv.New32a()
    h.Write([]byte(key))
    return int(h.Sum32()%50000) + 10000 // range: 10000-59999
}
```

- **Input**: project name + branch name + port name (null-byte separated to avoid ambiguity)
- **Hash**: FNV-1a 32-bit (fast, good distribution, stdlib)
- **Range**: 10000–59999 (avoids privileged ports 0–1023 and the ephemeral port range 32768–60999 on Linux)
- **Deterministic**: same inputs always produce the same port

### Collision handling

Different `(project, branch, name)` tuples can theoretically hash to the same port. This is rare (50,000 slots) and acceptable for a personal tool. If it happens:

- The dev server will fail to bind — the error message makes the cause obvious
- The user can rename the port label (e.g. `port "api2"`) to get a different hash

No automatic collision detection or retry. Simplicity over edge-case handling.

### Branch name normalization

Branch names are used as-is for hashing (e.g. `feature/login`). The hash input uses the original branch name, not the directory-flattened version. This ensures `feature/login` always produces the same port regardless of how the directory is named.

---

## Integration Points

### `devenv worktree new <project> <branch>`

After creating the git worktree, automatically:

1. Resolve template: check `<worktree_path>/.env.template`, fall back to config `env_template`
2. If template found: process it with Go `text/template` + custom funcs
3. Write output to `<worktree_path>/.env`
4. Print: `  Env: ~/projects/github.com/user/myapp__worktrees/feature/.env (3 ports allocated)`

If no template exists, skip silently.

If template processing fails (syntax error, missing required env var): the worktree is still created, but an error is printed and exit code is 1. The user can fix the template and run `devenv worktree env` to retry.

### `devenv worktree env <project> <branch>`

New subcommand. (Re)generates `.env` from the template.

```
devenv worktree env myapp feature

Processing .env.template...  done
  Env: ~/projects/github.com/user/myapp__worktrees/feature/.env (3 ports allocated)
  Ports: api=34821 db=18293 redis=42107
```

Use cases:
- Template was updated after worktree creation
- Initial generation failed (missing env var, now set)
- User wants to see what ports were allocated

`--dry-run` flag prints the generated `.env` to stdout without writing it.

### `devenv session start <project> <branch>` (optional env export)

If `<worktree_path>/.env` exists, optionally export its variables into the tmux session environment:

```bash
tmux new-session -d \
  -s "myapp-feature" \
  -e "DEVENV_SESSION=myapp-feature" \
  -e "API_PORT=34821" \
  -e "DB_PORT=18293" \
  ...
```

Controlled by flag:

| Flag | Description |
|---|---|
| `--export-env` | Read `.env` and set all variables in the tmux session environment |

Default: off. When off, the `.env` file still exists on disk for tools that read it (Docker Compose, direnv, etc.).

When on, every KEY=VALUE line in `.env` (skipping comments and blank lines) is passed as `-e KEY=VALUE` to `tmux new-session`.

---

## `.env` File Format

The generated `.env` follows the standard format:

```bash
# Generated by devenv — do not edit (regenerate with: devenv worktree env myapp feature)
# Template: .env.template (repo-local)

API_PORT=34821
DB_PORT=18293
REDIS_PORT=42107
APP_BRANCH=feature
DOCKER_NETWORK=devenv-myapp-feature
JWT_SECRET=dev-secret-change-me
DATABASE_URL=postgres://dev:dev@localhost:18293/myapp
COMPOSE_PROJECT_NAME=myapp-feature
```

A header comment identifies the file as generated and tells the user how to regenerate.

---

## Package Location

`internal/envtemplate/` — new package.

```go
package envtemplate

// Process reads a template, executes it with the given context, and returns
// the rendered .env content.
func Process(templateContent string, ctx EnvTemplateContext) (string, error)

// DeterministicPort returns a stable port for the given project/branch/name.
func DeterministicPort(project, branch, name string) int

// ParseEnvFile reads a .env file and returns key-value pairs (skipping
// comments and blank lines). Used by session start --export-env.
func ParseEnvFile(path string) (map[string]string, error)
```

---

## Config Changes (Phase 1 addition)

Add `EnvTemplate` to `ProjectConfig`:

```go
type ProjectConfig struct {
    Repo          string `toml:"repo"`
    DefaultBranch string `toml:"default_branch"`
    EnvTemplate   string `toml:"env_template"`   // NEW: path to .env.template (~ expanded)
}
```

---

## Implementation Notes

- FNV-1a is in Go stdlib (`hash/fnv`) — no external dependency
- Go `text/template` is already used for cloud-init — same engine, different funcs
- The `env` template func should NOT error on missing vars without defaults — return empty string (same as shell behavior)
- `.env` files should be added to `.gitignore` by convention — devenv does not manage `.gitignore`
- Template path in config (`env_template`) has `~` expanded to `$HOME` at load time, same as `projects_dir`
- The `port` function is a pure function — no side effects, no state, no network
- `devenv worktree env` is idempotent — running it twice produces the same output
