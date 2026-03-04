# PRD: Project Scaffolding

## Overview

Set up the Go project structure, build tooling, linting, and testing infrastructure for `devenv`. This is a one-time setup that all subsequent PRDs depend on.

## Acceptance Criteria

- `go build ./...` produces a working binary
- `go test ./...` runs all unit tests (zero failures on a fresh clone)
- `make install` builds and installs the binary to `~/.local/bin/devenv`
- `golangci-lint run ./...` passes with zero errors
- `devenv --help` prints usage

---

## Repository Layout

```
devenv/                          ← repo root (current dir: ~/Projects/remote-dev)
├── main.go
├── go.mod
├── go.sum
├── Makefile
├── .golangci.yml
├── .gitignore
│
├── cmd/
│   ├── root.go
│   ├── up.go
│   ├── down.go
│   ├── status.go
│   ├── ssh.go
│   ├── config.go
│   ├── notify.go
│   ├── project.go
│   ├── worktree.go
│   └── session.go
│
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   │
│   ├── state/
│   │   ├── state.go
│   │   └── state_test.go
│   │
│   ├── do/
│   │   ├── client.go
│   │   ├── droplet.go
│   │   └── droplet_test.go
│   │
│   ├── provision/
│   │   ├── cloudinit.go
│   │   ├── cloudinit_test.go
│   │   └── templates/
│   │       └── user-data.yaml.tmpl
│   │
│   └── remote/
│       ├── remote.go
│       └── remote_test.go
│
└── docs/
    ├── project.md
    └── prds/
        └── *.md
```

---

## `go.mod`

- Module path: `github.com/OWNER/devenv` (replace OWNER with actual GitHub username)
- Go version: `1.26`

### Direct dependencies

| Package | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/digitalocean/godo` | DO API client |
| `github.com/BurntSushi/toml` | TOML config parsing |
| `github.com/charmbracelet/bubbletea/v2` | TUI framework |
| `github.com/charmbracelet/lipgloss/v2` | TUI styling |
| `github.com/charmbracelet/bubbles` | TUI components |
| `golang.org/x/crypto` | SSH client for `internal/remote` |

---

## `main.go`

Minimal entrypoint. Only responsibility: call `cmd.Execute()` and exit with the correct code.

```go
package main

import (
    "os"
    "github.com/OWNER/devenv/cmd"
)

func main() {
    if err := cmd.Execute(); err != nil {
        os.Exit(1)
    }
}
```

---

## `cmd/root.go`

- Defines the root `devenv` cobra command
- Persistent flags available to all subcommands:
  - `--config string` — override config file path (default: `~/.config/devenv/config.toml`)
  - `--token string` — override DO API token (takes precedence over config; also reads `DIGITALOCEAN_TOKEN` env var)
  - `--no-color` — disable colored output
- Initializes config via `internal/config` on `PersistentPreRunE`
- Exposes a package-level `Execute()` function

---

## `internal/config/config.go`

Responsibilities:
- Define `Config` struct with all fields
- Load from TOML file (XDG path: `~/.config/devenv/config.toml`)
- Override with environment variables (`DIGITALOCEAN_TOKEN`, `DEVENV_REGION`, etc.)
- Override with CLI flags passed from root command
- Provide `Profile` lookup by name
- Expose `Save()` for `devenv config set`

### Config struct (key fields)
```go
type Config struct {
    Defaults  DefaultsConfig             `toml:"defaults"`
    Profiles  map[string]ProfileConfig   `toml:"profiles"`
}

type DefaultsConfig struct {
    Token             string `toml:"token"`
    SSHKeyID          string `toml:"ssh_key_id"`
    Region            string `toml:"region"`
    Size               string `toml:"size"`
    TailscaleAuthKey  string `toml:"tailscale_auth_key"`
    Image             string `toml:"image"`  // default: "ubuntu-24-04-x64"
}

type ProfileConfig struct {
    Size    string `toml:"size"`
    Region  string `toml:"region"`
    Image   string `toml:"image"`
}
```

---

## `internal/state/state.go`

Responsibilities:
- Define `State` struct
- Read/write JSON from `~/.local/share/devenv/state.json`
- Handle missing file gracefully (returns empty state, no error)
- Handle corrupt file (returns descriptive error)
- `Clear()` — removes the state file on successful `devenv down`

### State struct
```go
type State struct {
    DropletID    int       `json:"droplet_id"`
    DropletName  string    `json:"droplet_name"`
    TailscaleIP  string    `json:"tailscale_ip"`
    PublicIP     string    `json:"public_ip"`
    Region       string    `json:"region"`
    Size         string    `json:"size"`
    Profile      string    `json:"profile"`
    CreatedAt    time.Time `json:"created_at"`
    Status       string    `json:"status"`
}
```

---

## `internal/do/client.go`

Responsibilities:
- Build an authenticated `*godo.Client` from a token
- Wrap the client in a thin `Client` struct that exposes only the methods `devenv` needs
- The `Client` struct should depend on interfaces (not concrete godo types) to allow mocking in tests

### Interface design (enables mocking)
```go
type DropletsService interface {
    Create(ctx context.Context, req *godo.DropletCreateRequest) (*godo.Droplet, *godo.Response, error)
    Get(ctx context.Context, dropletID int) (*godo.Droplet, *godo.Response, error)
    Delete(ctx context.Context, dropletID int) (*godo.Response, error)
}
```

---

## `internal/provision/cloudinit.go`

Responsibilities:
- Render `templates/user-data.yaml.tmpl` with provisioning parameters
- Return the rendered string to be passed as `UserData` in the droplet create request
- Template parameters: Tailscale auth key, non-root username, any feature flags

---

## `internal/remote/remote.go`

Shared utility for running commands on the active droplet over SSH and capturing their output. Used by `devenv project`, `devenv worktree`, and `devenv session` — all of which need programmatic SSH execution, not an interactive terminal.

This is distinct from `devenv ssh`, which hands off the terminal via `syscall.Exec`. `internal/remote` uses `golang.org/x/crypto/ssh` directly to open a session, pipe stdout/stderr, and return the result.

### Interface design

```go
// Client executes commands on a remote host over SSH.
type Client interface {
    Run(ctx context.Context, cmd string) (stdout string, stderr string, err error)
    RunStream(ctx context.Context, cmd string, stdout io.Writer) error
}

// Dial opens an SSH connection to the given host using the provided key.
func Dial(ctx context.Context, host, user, identityFile string) (Client, error)
```

- `Run` — captures stdout and stderr as strings; returns when the command exits
- `RunStream` — streams stdout to a writer in real time (used for `git clone` progress, etc.)
- The connection is established once and reused for multiple `Run` calls within a command
- Closes cleanly when the caller is done (caller calls `Close()`)

### Testing

Tests use a mock `Client` interface — no real SSH connections in unit tests. Integration tests (tagged `//go:build integration`) dial a real droplet using `DEVENV_TEST_HOST` env var.

---

## Makefile

```makefile
BIN_NAME  := devenv
INSTALL   := $(HOME)/.local/bin/$(BIN_NAME)
LDFLAGS   := -ldflags "-s -w -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)"

.PHONY: build install test test-integration lint clean

build:
	go build $(LDFLAGS) -o $(BIN_NAME) .

install: build
	mv $(BIN_NAME) $(INSTALL)
	@echo "Installed to $(INSTALL)"

test:
	go test ./...

test-integration:
	go test -tags integration ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(BIN_NAME)
```

---

## `.golangci.yml`

```yaml
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
    - goimports
    - misspell
    - gosec
    - wrapcheck

linters-settings:
  goimports:
    local-prefixes: github.com/OWNER/devenv

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - wrapcheck
        - gosec
```

---

## `.gitignore`

```
devenv
*.test
.env
*.toml.local
```

---

## Testing Strategy

- All packages under `internal/` must have corresponding `_test.go` files
- Tests use **table-driven** style throughout
- `internal/do/` tests mock `DropletsService` interface — no real API calls
- `internal/state/` tests use `t.TempDir()` to avoid touching real state file
- `internal/config/` tests use `t.TempDir()` for config file
- `internal/provision/` tests assert rendered YAML is valid and contains expected strings
- `internal/remote/` tests mock the `Client` interface — no real SSH connections
- Integration tests are tagged `//go:build integration` and require env vars:
  - `DIGITALOCEAN_TOKEN` for DO API tests
  - `DEVENV_TEST_HOST` for `internal/remote` SSH tests

---

## Implementation Order

1. `go mod init` + add dependencies
2. `internal/config` package + tests
3. `internal/state` package + tests
4. `internal/do` package (client + droplet) + tests (mocked)
5. `internal/provision` package + template + tests
6. `cmd/root.go` wiring config load
7. Stub remaining commands (print "not implemented" — enough for `devenv --help` to work)
8. Makefile + `.golangci.yml`
9. Verify `make build`, `make test`, `make lint` all pass
