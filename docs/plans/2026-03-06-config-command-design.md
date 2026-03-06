# Design: `devenv config` Command

Date: 2026-03-06
PRD: `docs/prds/prd-phase-03-cmd-config.md`

---

## Summary

Implements the `devenv config` command and all subcommands: `init`, `show`, `set`, `get`, `profile create/list/delete/show`. Extends `internal/config` with surgical TOML editing (comment-preserving), struct validation, and a key-path resolution mechanism. Adds three new dependencies.

---

## New Dependencies

| Package | Purpose |
|---|---|
| `charmbracelet/huh` | Interactive form wizard for `config init` and `profile create` |
| `pelletier/go-toml` (v1) | Replaces `BurntSushi/toml`; adds comment-preserving `Tree` API |
| `go-playground/validator/v10` | Struct-level value validation on `Config` after any mutation |

`BurntSushi/toml` is removed. The `pelletier/go-toml` v1 API is near-identical for struct ops (same `toml:""` tags, same `Unmarshal`/`Encode` signatures), so existing `internal/config` code and tests require minimal changes.

---

## Architecture

### `internal/config` changes

#### go-toml v1 migration

Replace `github.com/BurntSushi/toml` with `github.com/pelletier/go-toml` throughout
`internal/config`. The struct op signatures are identical; only the import path changes.

#### Validation tags on structs

```go
type DefaultsConfig struct {
    Token            string `toml:"token"             validate:"omitempty"`
    SSHKeyID         string `toml:"ssh_key_id"         validate:"omitempty"`
    Region           string `toml:"region"             validate:"omitempty"`
    Size             string `toml:"size"               validate:"omitempty"`
    TailscaleAuthKey string `toml:"tailscale_auth_key" validate:"omitempty"`
    Image            string `toml:"image"              validate:"omitempty"`
    ProjectsDir      string `toml:"projects_dir"       validate:"omitempty"`
    GitIdentityFile  string `toml:"git_identity_file"  validate:"omitempty"`
}

type ProjectConfig struct {
    Repo          string `toml:"repo"           validate:"omitempty,url|startswith=git@"`
    DefaultBranch string `toml:"default_branch" validate:"omitempty"`
    EnvTemplate   string `toml:"env_template"   validate:"omitempty"`
}
```

Most fields are `omitempty` — nothing is required at config-file level (env vars and CLI
flags fill gaps at runtime). `ProjectConfig.Repo` validates it's a URL or SSH git address.

#### Secret tagging

Fields containing secrets carry a `secret:"true"` struct tag, used by `redact()` and
`config get` to avoid leaking values:

```go
Token            string `toml:"token" validate:"omitempty" secret:"true"`
TailscaleAuthKey string `toml:"tailscale_auth_key" validate:"omitempty" secret:"true"`
```

#### New methods

**`Path() string`** — getter for `c.path`; used by commands that open the raw file via Tree.

**`Validate() error`** — runs `go-playground/validator` against the Config struct:

```go
var validate = validator.New()

func (c *Config) Validate() error {
    return validate.Struct(c)
}
```

**`SetKey(dotPath, value string) error`**:
1. Validate `dotPath` against the reflect-built valid-key set → error if unknown
2. Load file as go-toml Tree (re-reads fresh from disk, preserves comments)
3. `tree.Set(dotPath, value)`
4. Write Tree back to file
5. Reload into struct + run `Validate()` → return any value errors

**`DeleteSection(dotPath string) error`**:
1. Load file as go-toml Tree
2. `tree.Delete(dotPath)`
3. Write back

#### Key-path validation via reflection

A `validKeys` set is built once from the Config struct's `toml:""` tags using `reflect`,
cached in a `sync.Once`. This enumerates all leaf paths (e.g. `defaults.token`,
`defaults.region`) plus wildcard sections for `profiles.*` and `projects.*`. Unknown paths
return an error before the file is touched. The set stays in sync with the struct
automatically — no manual maintenance.

---

### `cmd/config.go` subcommands

#### `config init`

1. If config exists: `huh.Confirm` — "Config already exists. Overwrite? [y/N]"
2. `huh.Input` (masked) for DO API token
3. Call DO API with entered token to fetch SSH keys and regions; on API failure, fall back
   to plain text input with a warning printed to stderr
4. `huh.Select` for SSH key (name + ID), region (slug + label), size (slug + pricing)
5. `huh.Input` (optional) for Tailscale auth key
6. `huh.Input` for projects dir (default `~/projects` shown as placeholder)
7. Call `cfg.Save()` — fresh file write; no prior comments to preserve here

#### `config show`

Prints the loaded `cfg` with secrets redacted. Uses `redact()`:

```go
func redact(s string) string {
    if len(s) <= 12 {
        return "****"
    }
    return s[:8] + "****" + s[len(s)-4:]
}
```

Applied to all fields tagged `secret:"true"`. Output format matches the PRD example.

#### `config set <key> <value>`

Delegates entirely to `cfg.SetKey(key, value)`. Prints `Set <key> = "<value>"` on success.

#### `config get <key>`

Uses the reflect-built key map to read the value from the already-loaded struct. Applies
`redact()` to secret fields. Prints the bare value (no quotes) for scripting compatibility.

#### `config profile create <name>`

- `huh.Select` for size (with pricing labels)
- `huh.Input` for region (pressing Enter uses default from `cfg.Defaults.Region`)
- Calls `cfg.SetKey("profiles.<name>.size", ...)` and `cfg.SetKey("profiles.<name>.region", ...)`
- Prints: `Profile "<name>" created\nUse it with: devenv up --profile <name>`

#### `config profile list`

Tabwriter table: `PROFILE / SIZE / REGION`. First row is always `default` derived from
`cfg.Defaults`.

#### `config profile delete <name>`

- Guard: returns error if `name == "default"`
- `huh.Confirm` — "Delete profile \"<name>\"? [y/N]"
- Calls `cfg.DeleteSection("profiles.<name>")`

#### `config profile show <name>`

Calls `cfg.Profile(name)` and prints the profile's fields.

---

## Error Handling

| Condition | Output | Exit code |
|---|---|---|
| Config missing at `set`/`get` | `Config not initialized. Run 'devenv config init' first.` | 1 |
| Unknown key in `set` | `Error: unknown config key "<key>". Run 'devenv config show' to see valid keys.` | 1 |
| Invalid value (validator) | Human-readable message from validator tag | 1 |
| Corrupt TOML | `Error: failed to parse config: <details>. Fix or delete ~/.config/devenv/config.toml` | 1 |
| Profile not found in `delete`/`show` | `Error: profile "<name>" does not exist` | 1 |
| Attempt to delete `default` | `Error: "default" is not a profile` | 1 |
| DO API failure during `init` | Print warning, fall back to plain text input for affected fields | 0 |

---

## Testing Strategy

### `internal/config` unit tests

- `TestSetKey_ValidKey` — sets a known key, re-reads file, verifies value and that an
  existing comment in the file is preserved
- `TestSetKey_UnknownKey` — returns error, file not modified
- `TestSetKey_InvalidValue` — Tree write succeeds but Validate() fails, error returned
- `TestDeleteSection_ExistingProfile` — profile removed, other content intact
- `TestDeleteSection_NonExistent` — no error (idempotent)
- `TestValidate_ValidConfig` — no error
- `TestValidate_BadRepoURL` — error from validator

### `cmd/` command tests

- Command-level tests via `cobra.Command.Execute()` with temp config files
- `config show` — verify secrets are redacted, non-secrets are not
- `config get` — verify correct value returned; verify secret fields are redacted
- `config profile list` — verify default row always present
- `config profile delete default` — verify error returned
- Interactive `huh` forms are not unit-tested (require TTY); integration tested manually

### DO API in `config init`

The DO client call is injected via a thin interface (consistent with the existing
`DropletsService` pattern in `internal/do`), allowing tests to inject a mock that returns
canned SSH keys and regions.

### Coverage

Target: ≥80% aggregate (enforced by `make coverage`). The untestable surface (huh forms)
is isolated to `cmd/` and is small relative to the tested `internal/config` logic.
