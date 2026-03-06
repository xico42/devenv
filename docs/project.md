# devenv — Remote Dev Environment CLI

## Purpose

`devenv` is a personal CLI tool for spinning up and destroying ephemeral cloud development environments on Digital Ocean. It is designed for developers who want to run Claude Code agents (or any heavy workloads) on a remote machine, accessible from any device — including mobile — without maintaining a persistent, always-on server.

The core principle: **pay only for what you use**. Start a droplet when you need it, work, then destroy it. Total cost for an active session is a few cents per hour.

---

## Inspiration

This project is inspired by the following article:

> **Claude Code On-The-Go** — https://granda.org/en/2026/01/02/claude-code-on-the-go/

### Article summary

The article describes running six Claude Code agents in parallel from an iOS device using an ephemeral Vultr cloud VM (~$0.29/hr). The full stack:

| Layer | Tool | Purpose |
|---|---|---|
| Infrastructure | Vultr VM (vhf-8c-32gb) | Compute |
| Security | Cloud firewall + nftables + fail2ban | Defense in depth; only Tailscale traffic allowed |
| Networking | Tailscale | Private overlay network; no public SSH port |
| Terminal | Termius (iOS) + Mosh | Resilient mobile SSH across network transitions |
| Sessions | tmux (auto-attach via .zshrc) | Persistent sessions that survive disconnects |
| Parallelism | Git worktrees | Independent branch checkouts for concurrent agents |
| Ports | Deterministic hash-based allocation | Avoids port conflicts across parallel dev servers |
| Notifications | Poke (iOS webhook service) | Push notifications when Claude needs input |
| Trust | Claude Code permissive mode | Unattended operation on isolated, ephemeral VM |
| Automation | Shell scripts + iOS Shortcut | `vm-start` waits for Tailscale then auto-connects via mosh; iOS Shortcut calls Vultr API and opens Termius in one tap |

Key workflow: start VM from phone -> auto-connect -> kick off tasks in parallel worktrees -> pocket the phone -> get push notification when Claude needs input -> respond -> repeat. The VM is halted (not destroyed) between sessions.

### How devenv maps to the article

| Article feature | devenv equivalent | Status |
|---|---|---|
| VM start/stop | `devenv up` / `devenv down` | Planned (Phase 2) |
| Tailscale networking | cloud-init provisions Tailscale | Planned (Phase 2) |
| Mosh + SSH | `devenv ssh --mosh` | Planned (Phase 2) |
| tmux auto-attach | tmux-native config via cloud-init | Planned (Phase 2) |
| Push notifications | `devenv notify` (Telegram, Slack, Discord, webhook) | Planned (Phase 2) |
| Git worktrees | `devenv worktree` (local execution) | Planned (Phase 2) |
| Parallel Claude sessions | `devenv session` (local execution) | Planned (Phase 2) |
| Deterministic port allocation | `.env.template` + `port "name"` (hash-based, no state) | Planned (Phase 2) |
| Project cloning | `devenv project clone` (local execution) | Planned (Phase 2) |
| Session management UI | tmux-native status bar + keybindings | Planned (Phase 2) |
| Rich TUI dashboard | `devenv tui` (Bubble Tea v2, remote) | Planned (Phase 4, optional) |

### Not yet handled or planned

These features from the article are not currently in the devenv roadmap. They are documented here for future consideration.

| Article feature | Gap | Notes |
|---|---|---|
| Halt/resume (not destroy) | `devenv down` destroys; no `stop`/`start` pair | DO supports power-off ($0.007/hr disk-only) — much faster than destroy + re-provision |
| Composite start-to-work command | Requires multiple commands today: `up` -> `project clone` -> `worktree new` -> `session start` | Intentional: primitives first, composite `setup` command later (see Design philosophy) |
| Firewall hardening | cloud-init installs Tailscale but no nftables/ufw/fail2ban | Defense in depth; Tailscale alone is not a firewall |
| Claude Code permissive mode | cloud-init does not configure Claude's permission model | Required for unattended "pocket the phone" workflow |
| ~~Deterministic port allocation~~ | `.env.template` processing with deterministic `port "name"` function | Planned (Phase 2) — see `prd-phase-02-env.md` |
| Mobile shortcut integration | No URL scheme, HTTP API, or Tasker/Automate profile | One-tap "phone -> working" flow |
| Session-aware reconnect | No "take me to the session that's waiting" quick-jump | Notifications say which session, but reconnecting is manual |
| Cost guard / auto-shutdown | Cost shown in `status` and `down`, but no ceiling or idle detection | Financial safety net for forgotten droplets |

---

## Design Philosophy

### Primitives first

`devenv` is being built **primitives-first**. The core commands (`up`, `down`, `project`, `worktree`, `session`, `notify`) are independent, composable building blocks. Each is useful on its own and testable in isolation.

Higher-level automation (e.g., a single `devenv setup` command that provisions a droplet, clones projects, creates worktrees, and starts sessions in one shot) will be built **on top of** these primitives, not instead of them. This layering is intentional:

1. **Primitives are stable** — the individual operations (create a droplet, clone a repo, create a worktree) are well-defined and unlikely to change.
2. **Composite commands are opinionated** — a "start everything" command embeds workflow assumptions that differ between use cases. Building it on stable primitives means the opinions live in one place.
3. **Debuggability** — when something fails in a composite flow, the user can drop down to the primitive that failed and run it in isolation.

### Commands run locally

Workload commands (`project`, `worktree`, `session`) execute git, tmux, and filesystem operations **directly on the machine where devenv is invoked**. They have no SSH logic and no awareness of whether they're running on a droplet or a laptop.

- On the droplet (after `devenv ssh`): commands operate on the droplet's filesystem
- On a local machine: commands operate on the local filesystem — useful for testing and local development

This eliminates the need for an `internal/remote` SSH package for workload commands. Infrastructure commands (`up`, `down`, `status`, `ssh`) remain local-machine-only and interact with the DO API.

### tmux-native before TUI

Session management uses tmux's built-in capabilities first:
- Status bar showing all sessions and their state (running/waiting)
- Custom keybindings for session switching, creation, and navigation
- `choose-tree` for session picking

A Bubble Tea TUI is a later optional phase that adds polish (forms, live-updating views, formatted output) but is not required for the workflow to function.

### Use cases beyond mobile

The article targets mobile-first, phone-in-pocket async development. `devenv` targets that **and** a second use case: **offloading the local machine**.

When running heavy workloads (multiple Claude agents, Docker builds, test suites), the local machine becomes sluggish. Spinning up a beefy droplet and moving work there frees local resources entirely. This is not about mobility — it's about compute.

The two use cases have different workflows:

| Concern | Mobile (phone) | Offload (laptop/desktop) |
|---|---|---|
| Terminal | Termius + Mosh | Local terminal + SSH |
| Connection | Intermittent, async | Persistent, synchronous |
| Interaction | Respond to notifications | Actively monitor sessions |
| Environment | Homogeneous (Claude agents) | Heterogeneous (Docker, Go, PHP, etc.) |
| Composite flow | One-tap start -> auto-connect | Project-specific setup scripts |

The heterogeneous environment case is important: different projects need different toolchains (Docker Compose for one, Go + mise for another, PHP + Composer for a third). A single monolithic provisioning step can't cover all of them. The primitives (`project`, `worktree`, `session`) let each project define its own setup while sharing the underlying infrastructure.

---

## Goals

1. **One command to start working** — `devenv up` creates a fully provisioned droplet in under 2 minutes.
2. **One command to clean up** — `devenv down` destroys all resources with no orphaned costs.
3. **Reproducible environments** — every droplet is provisioned identically via cloud-init, with no manual steps.
4. **Mobile and desktop** — environments are accessible from Android via Termius + Mosh over Tailscale, and from any desktop terminal via SSH.
5. **Composable primitives** — individual commands (`up`, `project`, `worktree`, `session`) work independently and compose into higher-level workflows.
6. **Heterogeneous projects** — supports Docker-based, Go, PHP, and any other toolchain side by side, because provisioning provides the base and projects bring their own setup.
7. **Docker + mise out of the box** — the two primary environment management tools are pre-installed and ready on every droplet.
8. **Self-contained binary** — a single Go binary with no runtime dependencies.

---

## Non-Goals

- Not a general-purpose infrastructure tool (use Terraform/Pulumi for that)
- Not multi-user
- Not a container orchestration tool
- Not designed for long-running persistent environments

---

## Use Cases

### Primary: Claude Code agent sessions (mobile)
Start a droplet, SSH in from phone, run multiple Claude Code agents in parallel tmux sessions across git worktrees, pocket the phone, get notified when agents need input, respond, destroy when done.

### Primary: Offloading the local machine (desktop)
Move heavy workloads (multiple Claude agents, Docker Compose stacks, test suites) to a beefy remote droplet so the local machine stays responsive. Work from a local terminal over SSH/Tailscale with persistent sessions. Heterogeneous projects (Go, PHP, Docker-based) coexist on the same droplet, each in its own worktree.

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
| TUI (optional, Phase 4) | [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) + [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Provisioning | cloud-init (user-data YAML) |
| Config format | TOML |
| Linter | golangci-lint |

### Why godo over doctl?
`doctl` is a CLI tool, not a library. Using `godo` directly means the entire binary is self-contained — no dependency on `doctl` being installed. `doctl` itself uses `godo` internally.

### Command planes

`devenv` commands are split into two planes:

| Plane | Commands | Runs on | Needs |
|---|---|---|---|
| **Infrastructure** | `up`, `down`, `status`, `ssh`, `config`, `notify setup/test/status` | Local machine | DO API token, Tailscale |
| **Workload** | `project`, `worktree`, `session`, `notify send` | Wherever devenv is invoked | git, tmux, filesystem |

Workload commands execute locally — they run `git`, `tmux`, and filesystem operations directly. They don't SSH anywhere. On the droplet (after `devenv ssh`) they operate on the droplet's filesystem. On a laptop they operate on the laptop's filesystem. The command doesn't care where it runs.

Infrastructure commands run on the local machine and interact with the DO API or SSH into the droplet for specific tasks (copying git keys, checking readiness).

### Networking
Droplets are accessed via **Tailscale** private network. No public IP is assigned or needed. This means:
- No exposed SSH port
- No fail2ban / firewall rules to manage
- Access from any device on the Tailscale network

Tailscale is installed and auto-authenticated during cloud-init provisioning using an **auth key**.

### Session Persistence
**Mosh** is installed on the droplet for resilient connections across network transitions (WiFi <-> cellular, device sleep). **tmux** provides persistent sessions with a custom status bar and keybindings for session management.

### tmux session architecture (Layout A)

Each Claude agent session runs in its own tmux session. A management session (`devenv`) is the landing point:

```
tmux sessions:
  devenv           <- landing session (status bar, management)
  myapp-feature    <- Claude agent
  api-experiment   <- Claude agent
  myapp-fix-123    <- Claude agent
```

Navigation uses tmux's built-in session switching:
- `Ctrl+B s` — session picker (shows all sessions)
- `Ctrl+B ( / )` — previous / next session
- `Ctrl+B T` — custom binding: jump back to devenv management session

The tmux status bar (visible in every session) shows all sessions and their state:
```
[devenv] myapp-feature:running | api-experiment:WAITING | myapp-fix:running    2h 15m
```

---

## Key File Locations

| Purpose | Path |
|---|---|
| CLI config | `~/.config/devenv/config.toml` |
| Active droplet state | `~/.local/share/devenv/state.json` |
| Session state (on droplet) | `~/.local/share/devenv/sessions/<name>.json` |
| Binary | `~/.local/bin/devenv` |

### `~/.config/devenv/config.toml` structure (overview)
```toml
[defaults]
token = "..."                    # DO API token (or via DIGITALOCEAN_TOKEN env var)
ssh_key_id = "..."               # DO SSH key ID
region = "nyc3"
size = "s-2vcpu-4gb"
tailscale_auth_key = "..."
projects_dir = "~/projects"      # base directory for project clones and worktrees

[profiles.heavy]
size = "s-8vcpu-16gb"
region = "sfo3"

[projects.myapp]
repo = "git@github.com:user/myapp.git"
default_branch = "main"
# cloned to: ~/projects/github.com/user/myapp/
```

### Project directory layout

Clone paths mirror the repo URL structure under `projects_dir` (like `ghq` / Go module paths):

```
~/projects/
  github.com/
    user/
      myapp/                       <- main clone
      myapp__worktrees/
        feature/                   <- worktree for branch "feature"
        fix-123/                   <- worktree for branch "fix-123"
      api/                         <- different project
  gitlab.com/
    corp/
      backend/
        api/                       <- clone from gitlab
        api__worktrees/
          develop/                 <- worktree
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
- **tmux**: installed with devenv-specific config (status bar, keybindings, Layout A sessions)
- **devenv binary**: installed to `/usr/local/bin/devenv` (same binary, for workload commands and hooks)
- **devenv config**: notification provider credentials copied from local config
- **Claude Code**: installed via npm (`@anthropic-ai/claude-code`)
- **Claude Code hooks**: `~/.claude/settings.json` bootstrapped with hooks that call `devenv notify send` and `devenv session mark-running`

The provisioning is entirely declarative — a YAML template rendered at `devenv up` time and passed as user-data to the droplet.

---

## Development Roadmap

### Dependency overview

```
[Phase 0: Scaffold] .............. DONE
       |
       v
[Phase 1: Adapt scaffold] ........ sequential (removes internal/remote, adds projects_dir)
       |
       v
[Phase 2: Core commands] ......... ALL PARALLEL after Phase 1
       |
       |--- Infrastructure plane (local machine):
       |      config, up, down, status, ssh, notify
       |
       |--- Workload plane (runs anywhere):
       |      project, worktree, session
       |
       |--- Provisioning:
       |      cloud-init template (tmux-native config, devenv binary, hooks)
       |
       v
[Phase 3: Composite + skill] ..... after Phase 2
       |      devenv setup (one-command start-to-work)
       |      using-devenv agent skill (embed.FS + devenv skill install)
       |
       v
[Phase 4: Remote TUI] ............ optional, after Phase 3
              Bubble Tea v2 dashboard running on the droplet
```

**Key insight: ALL Phase 2 commands are independent and can be implemented in parallel.** Infrastructure commands and workload commands have no dependencies on each other. The only prerequisite is Phase 1 (scaffold adaptation).

---

### Phase 0 — Scaffold (DONE)

Internal packages built in order:

1. [X] `go.mod` init + dependencies
2. [X] `internal/config` — TOML loading, env var overrides, profile resolution
3. [X] `internal/state` — JSON state read/write, `Clear()`
4. [X] `internal/do` — godo client + `DropletsService` interface
5. [X] `internal/provision` — cloud-init template rendering
6. [X] `cmd/root.go` — cobra root, persistent flags, config load on `PersistentPreRunE`
7. [X] Command stubs + Makefile + `.golangci.yml`

See `docs/prds/prd-phase-00-scaffolding.md`.

---

### Phase 1 — Adapt scaffold (sequential, blocks Phase 2)

Adapt the Phase 0 scaffold to reflect the local-execution architecture. See `docs/prds/prd-phase-01-adapt-scaffold.md`.

- [ ] Remove `internal/remote` package (SSH execution for workload commands is eliminated)
- [ ] Remove `golang.org/x/crypto` dependency (no longer needed without `internal/remote`)
- [ ] Add `projects_dir` to config (`DefaultsConfig.ProjectsDir`, default: `~/projects`)
- [ ] Add `[projects]` section to config (map of project name -> repo + default_branch)
- [ ] Add `[notify]` section to config (provider + provider-specific settings)
- [ ] Add session state types to `internal/state` (session state files at `~/.local/share/devenv/sessions/`)
- [ ] Verify `make test`, `make lint`, `make coverage` still pass

---

### Phase 2 — Core commands (ALL PARALLEL after Phase 1)

Every command in this phase depends only on `internal/` packages from Phase 0+1. **None depend on each other.** They can all be implemented in parallel by independent agents or in any order.

#### Infrastructure plane (runs on local machine)

- [ ] `devenv config` — `internal/config` only (see `docs/prds/prd-phase-02-cmd-config.md`)
- [ ] `devenv up` — `internal/config` + `state` + `do` + `provision` (see `docs/prds/prd-phase-02-cmd-up.md`)
- [ ] `devenv down` — `internal/config` + `state` + `do` (see `docs/prds/prd-phase-02-cmd-down.md`)
- [ ] `devenv status` — `internal/config` + `state` + `do` (see `docs/prds/prd-phase-02-cmd-status.md`)
- [ ] `devenv ssh` — `internal/config` + `state` only; `syscall.Exec` pattern (see `docs/prds/prd-phase-02-cmd-ssh.md`)
- [ ] `devenv notify` (setup / test / status / send) — `internal/config` + HTTP client (see `docs/prds/prd-phase-02-cmd-notify.md`)

#### Workload plane (runs anywhere — no SSH, no droplet required)

- [ ] `devenv project` — `internal/config` + git + filesystem (see `docs/prds/prd-phase-02-cmd-project.md`)
- [ ] `devenv worktree` — `internal/config` + git + filesystem (see `docs/prds/prd-phase-02-cmd-worktree.md`)
- [ ] `devenv session` — `internal/config` + `internal/state` + tmux + filesystem (see `docs/prds/prd-phase-02-cmd-session.md`)
- [ ] env templates — `internal/envtemplate` + `text/template` (see `docs/prds/prd-phase-02-env.md`)

#### Provisioning

- [ ] cloud-init template — tmux-native config, devenv binary, Claude Code hooks (see `docs/prds/prd-phase-02-cloud-init.md`)

---

### Phase 3 — Composite commands & agent skill (after Phase 2)

- [ ] `devenv setup` — one-command start-to-work: `up` + `project clone --all` + worktree creation + session start
- [ ] `using-devenv` agent skill — reference skill teaching Claude Code agents how to use devenv; embedded in binary via `devenv skill install` (see `docs/prds/prd-phase-03-agent-skill.md`)

---

### Phase 4 — Remote TUI (optional, after Phase 3)

Bubble Tea v2 dashboard running **on the droplet** in its own tmux session (Layout A). Replaces the tmux-native management experience with a richer interface.

- [ ] Interactive Bubble Tea v2 dashboard (see `docs/prds/prd-phase-04-tui.md`)

The TUI is a **workload-plane** tool — it manages sessions, worktrees, and projects. It does not manage droplet lifecycle (that stays in the CLI on the local machine).

---

### Future — Quality of life (not yet planned)

- `devenv stop` / `devenv start` — halt/resume droplet without destroying
- `devenv snapshot` — save/restore droplet snapshots
- Multi-environment support (`devenv list`, named environments)
- Cost guard / auto-shutdown
- Firewall hardening in cloud-init
- Claude Code permissive mode configuration
- Mobile shortcut integration (Tasker/Automate)
- Session-aware reconnect (notification deep-links to waiting session)
- Bidirectional chat bot (Telegram bot for management commands)

---

## Package Layout

```
devenv/
├── main.go
├── go.mod / go.sum
├── Makefile
├── .golangci.yml
│
├── cmd/
│   ├── root.go          — cobra root, persistent flags, config load
│   ├── up.go            — infrastructure: create droplet
│   ├── down.go          — infrastructure: destroy droplet
│   ├── status.go        — infrastructure: show droplet status
│   ├── ssh.go           — infrastructure: connect to droplet
│   ├── config.go        — infrastructure: manage config
│   ├── notify.go        — both planes: setup (local), send (anywhere)
│   ├── project.go       — workload: manage project clones
│   ├── worktree.go      — workload: manage git worktrees
│   └── session.go       — workload: manage tmux sessions
│
├── internal/
│   ├── config/          — TOML config: defaults, profiles, projects, notify; RepoPath() URL→path derivation
│   ├── state/           — JSON state: droplet state + session state files
│   ├── do/              — godo wrapper: DropletsService interface
│   ├── envtemplate/     — .env.template processing: deterministic ports, env var interpolation
│   └── provision/       — cloud-init template rendering
│       └── templates/
│           └── user-data.yaml.tmpl
│
└── docs/
    ├── project.md
    └── prds/
        └── *.md
```

Note: `internal/remote` has been removed. Workload commands execute locally.

---

## Agent Session Context

When starting a new agent session to continue development on this project, provide this file as context. Key decisions already made:

1. **Digital Ocean** as the cloud provider (not Vultr, AWS, etc.)
2. **godo** for DO API access (not doctl CLI, not Pulumi)
3. **Go + Cobra** for the CLI
4. **cloud-init** for droplet provisioning (not Ansible, not remote scripts)
5. **Tailscale** for networking (no public SSH port)
6. **Bubble Tea v2** for the optional TUI (not v1) — Phase 4 only
7. **XDG-compliant** file locations (config in `~/.config`, state in `~/.local/share`)
8. **SSH key in use**: `AbishaiV2` (DO key ID: `52790602`)
9. **Go version**: 1.26 (managed via mise)
10. **Binary install location**: `~/.local/bin/devenv`
11. **Workload commands run locally** — no `internal/remote`, no SSH indirection for project/worktree/session
12. **tmux-native session management** — status bar + keybindings before any TUI
13. **Layout A** — each Claude session in its own tmux session, management in `devenv` session
14. **projects_dir is configurable** — default `~/projects`
15. **Clone paths mirror repo URL** — `~/projects/github.com/user/myapp/` (like `ghq` / Go modules), set via config

---

## TODO / Deferred Ideas

Ideas and features that came up during development but were intentionally deferred. Each item should eventually become a PRD or be added to the roadmap.

| Idea | Context | Notes |
|---|---|---|
| `devenv config project add <name>` wizard | Came up during `config` command design | Interactive prompts for repo URL + default branch; alternative to `config set projects.<name>.repo ...` + `config set projects.<name>.default_branch ...`; no API calls needed |
