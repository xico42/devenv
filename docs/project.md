# devenv — Remote Dev Environment CLI

## Purpose

`devenv` is a personal CLI tool for spinning up and destroying ephemeral cloud development environments on Digital Ocean. It is designed for developers who want to run Claude Code agents (or any heavy workloads) on a remote machine, accessible from any device — including mobile — without maintaining a persistent, always-on server.

The core principle: **pay only for what you use**. Start a droplet when you need it, work, then destroy it. Total cost for an active session is a few cents per hour.

---

## Inspiration

This project is inspired by the following article:

> **Claude Code On-The-Go** — https://granda.org/en/2026/01/02/claude-code-on-the-go/

The article describes running multiple Claude Code agents in parallel from a mobile device using:
- A Vultr cloud VM (ephemeral, ~$0.29/hr)
- Tailscale for secure private networking (no public IP)
- Termius + Mosh for resilient mobile SSH
- tmux for persistent sessions
- Git worktrees for parallel agent workspaces
- Push notifications via PreToolUse hooks → Poke webhook → mobile

`devenv` replaces Vultr with Digital Ocean and adds full lifecycle automation, so that spinning up and tearing down environments requires a single command.

---

## Goals

1. **One command to start working** — `devenv up` creates a fully provisioned droplet in under 2 minutes.
2. **One command to clean up** — `devenv down` destroys all resources with no orphaned costs.
3. **Reproducible environments** — every droplet is provisioned identically via cloud-init, with no manual steps.
4. **Mobile-first access** — environments are accessible from Android via Termius + Mosh over Tailscale.
5. **Docker + mise out of the box** — the two primary environment management tools are pre-installed and ready on every droplet.
6. **Self-contained binary** — a single Go binary with no runtime dependencies.

---

## Non-Goals

- Not a general-purpose infrastructure tool (use Terraform/Pulumi for that)
- Not multi-user
- Not a container orchestration tool
- Not designed for long-running persistent environments

---

## Use Cases

### Primary: Claude Code agent sessions
Start a droplet, SSH in, run multiple Claude Code agents in parallel tmux windows across git worktrees, pocket the phone, get notified when agents need input, destroy when done.

### Secondary: Heavy build jobs
Offload resource-intensive tasks (builds, tests, data processing) to a large droplet. Destroy after the job completes.

### Secondary: Isolated experiments
Spin up a clean environment to test something without polluting the local machine. Destroy after.

---

## Architecture

### Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.26 |
| CLI framework | [Cobra](https://github.com/spf13/cobra) |
| DO API client | [godo](https://github.com/digitalocean/godo) — Digital Ocean's official Go client |
| TUI | [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Provisioning | cloud-init (user-data YAML) |
| Config format | TOML |
| Linter | golangci-lint |

### Why godo over doctl?
`doctl` is a CLI tool, not a library. Using `godo` directly means the entire binary is self-contained — no dependency on `doctl` being installed. `doctl` itself uses `godo` internally.

### Networking
Droplets are accessed via **Tailscale** private network. No public IP is assigned or needed. This means:
- No exposed SSH port
- No fail2ban / firewall rules to manage
- Access from any device on the Tailscale network

Tailscale is installed and auto-authenticated during cloud-init provisioning using an **auth key**.

### Session Persistence
**Mosh** is installed on the droplet for resilient connections across network transitions (WiFi ↔ cellular, device sleep). **tmux** is configured to auto-attach, so reconnecting always drops you back into your session.

---

## Key File Locations

| Purpose | Path |
|---|---|
| CLI config | `~/.config/devenv/config.toml` |
| Active droplet state | `~/.local/share/devenv/state.json` |
| Binary | `~/.local/bin/devenv` |

### `~/.config/devenv/config.toml` structure (overview)
```toml
[defaults]
token = "..."           # DO API token (or via DIGITALOCEAN_TOKEN env var)
ssh_key_id = "..."      # DO SSH key ID
region = "nyc3"
size = "s-2vcpu-4gb"
tailscale_auth_key = "..."

[profiles.heavy]
size = "s-8vcpu-16gb"
region = "sfo3"
```

### `~/.local/share/devenv/state.json` structure (overview)
```json
{
  "droplet_id": 123456789,
  "droplet_name": "devenv-20260304-143012",
  "tailscale_ip": "100.x.y.z",
  "public_ip": "...",
  "region": "nyc3",
  "size": "s-2vcpu-4gb",
  "profile": "default",
  "created_at": "2026-03-04T14:30:12Z",
  "status": "active"
}
```

---

## Droplet Provisioning (cloud-init)

Every droplet is provisioned with:

- **OS**: Ubuntu 24.04 LTS
- **Docker**: installed and configured for non-root user (`ubuntu`)
- **mise**: installed globally, available to all users
- **Tailscale**: installed and authenticated via auth key
- **mosh**: installed
- **tmux**: installed with auto-attach configuration
- **Claude Code**: installed via npm (`@anthropic-ai/claude-code`)
- **Claude Code hooks**: `~/.claude/settings.json` bootstrapped with a `PostToolUse` hook that calls `devenv notify send` when Claude requests user input (see `docs/prds/prd-cmd-notify.md`)

The provisioning is entirely declarative — a YAML template rendered at `devenv up` time and passed as user-data to the droplet.

---

## Development Roadmap

### Dependency overview

```
[Phase 0: Scaffold]
       │
       ▼
[Phase 1: Core commands] ←── all parallel after scaffold
  config  up  down  status  ssh  notify
       │
       ▼
[Phase 2: TUI] ←── after Phase 1
       │
       ▼
[Phase 3: Agent workflow]
  internal/remote ←── sequential prerequisite
       │
       ├── project  ──┐
       ├── worktree   ├── parallel after internal/remote
       ├── session    │
       └── notify --session flag ──┘
```

**Runtime workflow sequence** (not a code dependency — commands can be implemented in parallel):
`devenv up` → `devenv project clone` → `devenv worktree new` → `devenv session start`

---

### Phase 0 — Scaffold  *(sequential — blocks everything)*
Internal packages must be built in this order (each depends on the previous):

1. [ ] `go.mod` init + dependencies
2. [ ] `internal/config` — TOML loading, env var overrides, profile resolution
3. [ ] `internal/state` — JSON state read/write, `Clear()`
4. [ ] `internal/do` — godo client + `DropletsService` interface
5. [ ] `internal/provision` — cloud-init template rendering
6. [ ] `cmd/root.go` — cobra root, persistent flags, config load on `PersistentPreRunE`
7. [ ] Command stubs + Makefile + `.golangci.yml`

See `docs/prds/prd-scaffolding.md`.

---

### Phase 1 — Core lifecycle  *(all parallel after Phase 0)*

All commands depend only on `internal/` packages built in Phase 0. None depend on each other.

- [ ] `devenv config` — `internal/config` only (see `docs/prds/prd-cmd-config.md`)
- [ ] `devenv up` — `internal/config` + `state` + `do` + `provision` (see `docs/prds/prd-cmd-up.md`)
- [ ] `devenv down` — `internal/config` + `state` + `do` (see `docs/prds/prd-cmd-down.md`)
- [ ] `devenv status` — `internal/config` + `state` + `do` (see `docs/prds/prd-cmd-status.md`)
- [ ] `devenv ssh` — `internal/config` + `state` only; `syscall.Exec` pattern (see `docs/prds/prd-cmd-ssh.md`)
- [ ] `devenv notify` (setup / test / status / send) — `internal/config` + HTTP client only; no droplet required (see `docs/prds/prd-cmd-notify.md`)

---

### Phase 2 — TUI  *(after Phase 1)*

Wraps Phase 1 commands. Depends on all `internal/` packages and the `syscall.Exec` pattern from `devenv ssh`.

- [ ] Interactive Bubble Tea v2 dashboard (see `docs/prds/prd-tui.md`)

---

### Phase 3 — Multi-repo / agent workflow

#### Step 1 — `internal/remote`  *(sequential prerequisite for Phase 3)*

New shared package for running SSH commands on the droplet programmatically (capture stdout/stderr). Distinct from `devenv ssh` which hands off the terminal interactively. Required by `project clone`, all `worktree` subcommands, and all `session` subcommands.

#### Step 2 — Agent commands  *(all parallel after `internal/remote`)*

- [ ] `devenv project` — project config + clone via `internal/remote` (see `docs/prds/prd-cmd-project.md`)
- [ ] `devenv worktree` — git worktree management via `internal/remote`; `shell` uses `syscall.Exec` (see `docs/prds/prd-cmd-worktree.md`)
- [ ] `devenv session` — tmux session lifecycle via `internal/remote`; `attach` uses `syscall.Exec`; session state files at `~/.local/share/devenv/sessions/` on droplet (see `docs/prds/prd-cmd-session.md`)
- [ ] `devenv notify --session` flag + `devenv session mark-running` — extends Phase 1 notify with session state file writes; adds PreToolUse hook to `settings.json` template (see `docs/prds/prd-cmd-notify.md`, `docs/prds/prd-cmd-session.md`)

---

### Phase 4 — Quality of life
- [ ] `devenv snapshot` — save/restore droplet snapshots
- [ ] Multi-environment support (`devenv list`, named environments)

---

## Agent Session Context

When starting a new agent session to continue development on this project, provide this file as context. Key decisions already made:

1. **Digital Ocean** as the cloud provider (not Vultr, AWS, etc.)
2. **godo** for DO API access (not doctl CLI, not Pulumi)
3. **Go + Cobra** for the CLI
4. **cloud-init** for droplet provisioning (not Ansible, not remote scripts)
5. **Tailscale** for networking (no public SSH port)
6. **Bubble Tea v2** for the TUI (not v1)
7. **XDG-compliant** file locations (config in `~/.config`, state in `~/.local/share`)
8. **SSH key in use**: `AbishaiV2` (DO key ID: `52790602`)
9. **Go version**: 1.26 (managed via mise)
10. **Binary install location**: `~/.local/bin/devenv`
