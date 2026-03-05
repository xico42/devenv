# Design: Project Scaffolding (Phase 00)

## Overview

Bootstrap the Go project structure, build tooling, linting, and testing infrastructure for `devenv`. All subsequent PRDs depend on this baseline.

## Approach

Follow the PRD implementation order exactly (Option A). Each `internal/` package gets a real implementation and tests before moving to `cmd/`. Lint and build verified at the end.

## Module & Entrypoint

- **Module path:** `github.com/xico42/devenv`
- **Go version:** 1.26
- **`main.go`:** Minimal entrypoint — calls `cmd.Execute()`, exits with code 1 on error. No other logic.

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

## Internal Packages

Implemented in this order:

### `internal/config`

- Loads `~/.config/devenv/config.toml` via `BurntSushi/toml`
- Overlays env vars (`DIGITALOCEAN_TOKEN`, `DEVENV_REGION`, etc.)
- Overlays CLI flags passed from root command
- Exposes `Load()`, `Profile(name)`, `Save()`
- Tests use `t.TempDir()`

### `internal/state`

- Reads/writes `~/.local/share/devenv/state.json`
- Missing file → returns empty state, no error
- Corrupt file → returns descriptive error
- `Clear()` removes the state file
- Tests use `t.TempDir()`

### `internal/do`

- Thin `Client` wrapping `godo.Client`
- Depends on `DropletsService` interface (not concrete godo type) for mockability
- Tests mock the interface — no real API calls

### `internal/provision`

- Renders `templates/user-data.yaml.tmpl` via `text/template`
- Returns rendered string for droplet `UserData`
- Tests assert rendered YAML is valid and contains expected strings

### `internal/remote`

- Uses `golang.org/x/crypto/ssh` directly (not interactive terminal)
- Connection established once, reused for multiple calls, closed explicitly

```go
type Client interface {
    Run(ctx context.Context, cmd string) (stdout, stderr string, err error)
    RunStream(ctx context.Context, cmd string, stdout io.Writer) error
    Close() error
}

func Dial(ctx context.Context, host, user, identityFile string) (Client, error)
```

- Tests mock the `Client` interface — no real SSH connections
- Integration tests tagged `//go:build integration`, require `DEVENV_TEST_HOST`

## Command Layer

### `cmd/root.go`

- Defines root `devenv` cobra command
- Persistent flags: `--config`, `--token`, `--no-color`
- Loads config via `internal/config` on `PersistentPreRunE`
- Exposes package-level `Execute()`

### Stub commands

`up`, `down`, `status`, `ssh`, `config`, `notify`, `project`, `worktree`, `session` — each a minimal `cobra.Command` printing `"not implemented"`. Sufficient for `devenv --help` to list all subcommands.

## Tooling

### Makefile

```makefile
BIN_NAME  := devenv
INSTALL   := $(HOME)/.local/bin/$(BIN_NAME)
LDFLAGS   := -ldflags "-s -w -X main.version=$(shell git describe --tags --always --dirty 2>/dev/null || echo dev)"

.PHONY: build install test test-integration lint clean deps setup

deps:
	go mod download

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

setup: deps test test-integration lint build
	@echo "Setup complete"
```

### `.golangci.yml`

Linters enabled: `errcheck`, `govet`, `staticcheck`, `unused`, `goimports`, `misspell`, `gosec`, `wrapcheck`.

- `goimports` local prefix: `github.com/xico42/devenv`
- Test files exempt from `wrapcheck` and `gosec`

### `.gitignore`

```
devenv
*.test
.env
*.toml.local
```

## Testing Strategy

- Table-driven tests throughout
- All `internal/` packages have corresponding `_test.go` files
- `internal/config` and `internal/state` tests use `t.TempDir()`
- `internal/do` tests mock `DropletsService` — no real API calls
- `internal/remote` tests mock `Client` interface — no real SSH connections
- `internal/provision` tests assert rendered YAML validity and expected content
- Integration tests tagged `//go:build integration`, require env vars:
  - `DIGITALOCEAN_TOKEN` for DO API tests
  - `DEVENV_TEST_HOST` for remote SSH tests

## Implementation Order

1. `go mod init` + add dependencies
2. `internal/config` + tests
3. `internal/state` + tests
4. `internal/do` (client + droplet) + tests (mocked)
5. `internal/provision` + template + tests
6. `internal/remote` + tests (mocked)
7. `cmd/root.go` wiring config load
8. Stub remaining commands
9. Makefile + `.golangci.yml` + `.gitignore`
10. Verify `make setup` passes clean
