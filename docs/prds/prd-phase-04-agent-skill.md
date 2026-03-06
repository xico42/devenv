# PRD: Agent Skill — `using-devenv` (Phase 4)

## Overview

Claude Code agents on the droplet need to know how to use devenv — for session management, worktrees, notifications, etc. Currently there's no structured way to teach them. An agent skill solves this: a `SKILL.md` with reference files that Claude Code discovers and loads on demand.

This is a **Reference** type skill (tool documentation). One skill with progressive disclosure via reference files — agents asking "how do I start a remote session" shouldn't need to discover and load 3 separate skills.

**Phase:** 4 (alongside composite commands). The skill documents command interfaces from Phases 2-3 and complements `devenv setup`.

---

## Skill Identity

| Property | Value |
|---|---|
| Name | `using-devenv` |
| Type | Reference (tool documentation) |
| Description | `Use when managing remote development environments, spinning up Digital Ocean droplets, managing projects and git worktrees, running Claude Code agent sessions in tmux, or configuring notifications for async mobile development with devenv CLI` |

---

## Directory Structure

```
.claude/skills/using-devenv/
  SKILL.md
  references/
    infrastructure-commands.md
    workload-commands.md
    notifications.md
    workflows.md
    concepts.md
```

---

## SKILL.md Specification

Target: 150-200 lines.

### Frontmatter

```yaml
name: using-devenv
description: Use when managing remote development environments, spinning up Digital Ocean droplets, managing projects and git worktrees, running Claude Code agent sessions in tmux, or configuring notifications for async mobile development with devenv CLI
```

### Body Sections

| Section | Content | Target lines |
|---|---|---|
| Overview | What devenv is, two command planes (infrastructure vs workload), when to use which | 15-20 |
| Quick Reference | Table of every command with one-line description, grouped by plane | 40-50 |
| Decision Tree | "What do you want to do?" conditional workflow pointing to reference files | 30-40 |
| Key Concepts | One-liner each: directory layout, session naming, env templates, cloud-init — with pointers to `references/concepts.md` | 20-30 |
| Common Mistakes | Wrong plane usage, forgetting to clone before worktree, forgetting notify setup, DEVENV_SESSION not set | 15-20 |

---

## Reference File Specifications

### `references/infrastructure-commands.md` (~100-120 lines)

Documents commands that run on the local machine and interact with the DO API.

| Section | Content |
|---|---|
| `devenv up` | Flags (`--profile`, `--region`, `--size`), behavior (cloud-init wait, Tailscale ready), output |
| `devenv down` | Confirmation prompt, force flag, state cleanup |
| `devenv status` | Output format, API status vs local state, cost display |
| `devenv ssh` | Default SSH, `--mosh` flag, `syscall.Exec` replacement behavior |
| `devenv config` | Subcommands summary (`init`, `show`, `set`, `get`, `profile`), dot-notation keys |

### `references/workload-commands.md` (~120-150 lines)

Documents commands that run anywhere (droplet or local). No SSH, no droplet required.

| Section | Content |
|---|---|
| `devenv project` | `list`, `show`, `clone` (single and `--all`), directory layout after clone |
| `devenv worktree` | `list`, `new`, `delete`, `shell`, `env` — relationship to git worktrees, `__worktrees` directory convention |
| `devenv session` | `start`, `list`, `attach`, `stop`, `show`, `mark-running` — tmux session lifecycle, DEVENV_SESSION env var, state files |

### `references/notifications.md` (~80-100 lines)

Documents the notification system and its integration with Claude Code hooks.

| Section | Content |
|---|---|
| Setup | `notify setup` (provider selection), `notify test`, `notify status` |
| Sending | `notify send --session <name>` — used by Claude Code hooks |
| Providers | Telegram (recommended), Slack, Discord, generic webhook — config keys for each |
| Hook integration | How cloud-init bootstraps hooks in `~/.claude/settings.json`, PostToolUse/AskUserQuestion triggers, fail-open philosophy |

### `references/workflows.md` (~100-130 lines)

End-to-end user journeys with step-by-step commands.

| Workflow | Steps |
|---|---|
| Mobile-first async | `up` -> `ssh` -> `project clone` -> `worktree new` -> `session start` -> pocket phone -> respond to notifications -> `down` |
| Desktop offload | Same primitives, persistent terminal, multiple projects |
| Isolated experiment | `up` -> `ssh` -> ad-hoc work -> `down` (no project/worktree setup) |
| First-time setup | `config init` -> `config set` for projects -> first `up` |
| Daily operations | Resume existing droplet, create new sessions, check status |
| One-command start | `devenv setup` composite (when available) |

### `references/concepts.md` (~80-100 lines)

Explains the mental model behind devenv's design.

| Section | Content |
|---|---|
| Two command planes | Infrastructure vs workload, when to use which, plane-awareness table |
| Directory layout | `projects_dir`, repo path mirroring (`github.com/user/myapp`), `__worktrees` convention |
| Session model | tmux sessions (Layout A), `devenv` management session, DEVENV_SESSION env var, session state files at `~/.local/share/devenv/sessions/` |
| Env templates | `.env.template` processing, `port "name"` (deterministic FNV-1a hash), `env "VAR" "default"`, repo-local vs config path |
| Cloud-init | What gets installed (Docker, mise, Tailscale, mosh, tmux, devenv, Claude Code, hooks), declarative YAML template |

---

## Installation & Distribution

The skill needs to be available in two contexts:

1. **In the devenv repo** — `.claude/skills/using-devenv/` (project-level discovery for contributors)
2. **Globally on droplets** — `~/.claude/skills/using-devenv/` (available in any project repo the agent works on)

### Project-level

The skill files live in the repo at `.claude/skills/using-devenv/`. Claude Code discovers them automatically when working in the devenv project.

### Global on droplet (`devenv skill install`)

Skill files are embedded in the binary via Go `embed.FS` and written to a target directory by a new command.

#### New package: `internal/skill/`

```go
package skill

import "embed"

//go:embed all:files
var Files embed.FS
```

The `files/` subdirectory mirrors the skill structure:
```
internal/skill/files/
  using-devenv/
    SKILL.md
    references/
      infrastructure-commands.md
      workload-commands.md
      notifications.md
      workflows.md
      concepts.md
```

These are copies of (or symlinks to) `.claude/skills/using-devenv/`. The build process must ensure they stay in sync.

#### New command: `cmd/skill.go`

```
devenv skill install <path>
```

- `<path>` is a required positional argument — the directory to install skills into
- Creates `<path>/using-devenv/` with `SKILL.md` and all reference files
- Overwrites existing files if present (idempotent)
- Creates parent directories as needed

```bash
# Install for Claude Code (user-level)
devenv skill install ~/.claude/skills

# Install for Codex (user-level)
devenv skill install ~/.agents/skills

# Install to any custom location
devenv skill install /path/to/agent/skills
```

Output:
```
Installed skill "using-devenv" to ~/.claude/skills/using-devenv/
```

#### Cloud-init integration

Add to the cloud-init template (after devenv binary installation):

```yaml
- devenv skill install /home/ubuntu/.claude/skills
```

This ensures the skill is globally available to Claude Code on every new droplet.

### Keeping files in sync

The canonical source is `.claude/skills/using-devenv/`. The embedded copy at `internal/skill/files/` must match. Options:

1. **Symlinks** — `internal/skill/files/using-devenv` symlinks to `../../.claude/skills/using-devenv`. Go `embed` follows symlinks. Simplest approach.
2. **Copy in CI** — a Makefile target copies files before build. More explicit but adds a step.
3. **Build-time check** — `make lint` verifies the two directories are identical.

Recommended: option 1 (symlink) with option 3 (lint check) as a safety net.

---

## Acceptance Criteria

### Skill content
- [ ] `SKILL.md` exists at `.claude/skills/using-devenv/SKILL.md`
- [ ] Frontmatter has `name: using-devenv` and description starting with `Use when`
- [ ] SKILL.md body has all 5 sections (Overview, Quick Reference, Decision Tree, Key Concepts, Common Mistakes)
- [ ] SKILL.md is under 250 lines (target 150-200)
- [ ] All 5 reference files exist under `references/`
- [ ] Each reference file is under 150 lines
- [ ] All commands documented match Phase 2 command interfaces
- [ ] No workflow summaries or step-by-step procedures in SKILL.md description
- [ ] Reference files use third person (not "you should" but "the agent should" or imperative)

### Installation
- [ ] `internal/skill/` package embeds skill files via `embed.FS`
- [ ] `devenv skill install <path>` creates `<path>/using-devenv/` with all files
- [ ] Command is idempotent (re-running overwrites cleanly)
- [ ] Command creates parent directories
- [ ] Embedded files match `.claude/skills/using-devenv/` (enforced by lint or symlink)

### Integration
- [ ] Cloud-init template includes `devenv skill install ~/.claude/skills` after binary installation
- [ ] `make test` passes with new package
- [ ] `make lint` passes
- [ ] `make coverage` stays at or above 80%

---

## Implementation Notes

- The skill content should be written **after** Phase 2 commands are finalized, since it documents their interfaces. The PRD specifies the structure and targets now; content is filled in during Phase 3 implementation.
- `devenv skill install` is a simple file-copy command — no config, no state, no API calls. It belongs in the workload plane (runs anywhere).
- The `embed.FS` approach means the skill content is versioned with the binary. Updating the skill requires a new binary release, which is appropriate since the skill documents that binary's commands.
- Reference files should avoid duplicating `--help` output verbatim. Focus on behavior, gotchas, and how commands compose — information that `--help` doesn't convey.
- The description field is critical for skill discovery. It must mention the key triggers: "remote development environments", "Digital Ocean droplets", "projects", "git worktrees", "agent sessions", "tmux", "notifications", "async mobile development", "devenv CLI".
