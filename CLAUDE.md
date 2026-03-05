# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

`devenv` is a personal Go CLI for spinning up and destroying ephemeral Digital Ocean droplets for remote dev work. Full context in `docs/project.md`.

## Build and development commands

```bash
make build           # build ./devenv binary
make install         # build + install to ~/.local/bin/devenv
make test            # go test ./...
make test-integration # go test -tags integration ./...
make lint            # golangci-lint run ./...
make setup           # deps → test → test-integration → lint → build (full verification)
make clean           # remove local binary
```

Run a single package's tests:
```bash
go test ./internal/config/...
go test ./internal/do/...
# etc.
```

Build with version embedded (done automatically via Makefile):
```bash
go build -ldflags "-s -w -X main.version=$(git describe --tags --always)" -o devenv .
```

## Coverage requirement

Before marking any task complete, run:
```bash
make coverage
```
This enforces a minimum of 80% aggregate test coverage across all packages. The target fails with a non-zero exit code if coverage drops below the threshold. New code must include tests sufficient to keep aggregate coverage at or above 80%.

## Architecture

### Package layout

- **`main.go`** — entrypoint; delegates to `cmd.Execute()`; `version` var set via `-ldflags`
- **`cmd/`** — Cobra commands; `root.go` wires `PersistentPreRunE` to load config; each command is a separate file
- **`internal/config`** — TOML config at `~/.config/devenv/config.toml`; `Load()` returns defaults on missing file; `ApplyEnv()` overlays env vars; `ApplyFlags()` overlays CLI flags
- **`internal/state`** — JSON state at `~/.local/share/devenv/state.json`; tracks active droplet ID, Tailscale IP, etc.; `Load/Save/Clear` functions
- **`internal/do`** — thin wrapper around godo; `DropletsService` interface enables mocking in tests; `Client{Droplets: ...}` struct
- **`internal/provision`** — renders cloud-init user-data via `embed.FS` + `text/template`; template at `internal/provision/templates/user-data.yaml.tmpl`
- **`internal/remote`** — `Client` interface for running SSH commands programmatically (stdout/stderr capture); `Dial()` returns an `sshClient`; distinct from `devenv ssh` which does `syscall.Exec`

### Config and state paths (XDG-compliant)

| Purpose | Path |
|---|---|
| Config | `~/.config/devenv/config.toml` |
| State | `~/.local/share/devenv/state.json` |
| Binary | `~/.local/bin/devenv` |

### Key design patterns

- **Mocking**: `internal/do` exposes `DropletsService` interface so tests use a `mockDroplets` struct without real API calls
- **Missing file = empty defaults**: both `config.Load()` and `state.Load()` return zero-value structs (not errors) when the file doesn't exist
- **`syscall.Exec` for interactive commands**: `devenv ssh`, `devenv worktree shell`, `devenv session attach` replace the process rather than spawning a child
- **Integration tests**: tagged with `//go:build integration` and run separately via `make test-integration`; require real DO credentials

### Droplet provisioning

Cloud-init template (`internal/provision/templates/user-data.yaml.tmpl`) installs: Docker, mise, Tailscale (auth via `TailscaleAuthKey`), mosh, tmux, Claude Code, and Claude Code hooks that call `devenv notify send`.

### Implementation phases

See `docs/project.md` for the full roadmap. Phase 0 (scaffold) is complete. Phase 1 commands (`up`, `down`, `status`, `ssh`, `config`, `notify`) are next — they are all independent and can be implemented in parallel.

Phase 3 commands (`project`, `worktree`, `session`) require `internal/remote` and are also parallel after that prerequisite.
