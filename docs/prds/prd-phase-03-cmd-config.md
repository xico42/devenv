# PRD: `devenv config` (Phase 3 — Infrastructure)

## Overview

The `config` command manages the local configuration file at `~/.config/devenv/config.toml`. It provides subcommands for initial setup, reading and writing individual values, and managing named profiles for different droplet configurations.

**Plane:** Infrastructure (runs on local machine only).

---

## Command Interface

```
devenv config <subcommand> [flags]
```

### Subcommands

| Subcommand | Description |
|---|---|
| `init` | Interactive first-run setup wizard |
| `show` | Print the current config (redacting secrets) |
| `set <key> <value>` | Set a single config value |
| `get <key>` | Get a single config value |
| `profile create <name>` | Create a named profile interactively |
| `profile list` | List all profiles |
| `profile delete <name>` | Delete a profile |
| `profile show <name>` | Show a profile's settings |

---

## `devenv config init`

Interactive wizard. Recommended entry point for first-time setup.

### Flow

```
devenv config init

Welcome to devenv setup!

? Digital Ocean API token: **********************
? SSH key to use:
  > AbishaiV2 (52790602)
    Abishai (27381478)

? Default region:
  > nyc3 - New York 3
    sfo3 - San Francisco 3
    ams3 - Amsterdam 3

? Default droplet size:
  > s-2vcpu-4gb  ($18/mo, $0.027/hr)  -- recommended
    s-4vcpu-8gb  ($36/mo, $0.054/hr)
    s-8vcpu-16gb ($72/mo, $0.107/hr)

? Tailscale auth key (optional, press Enter to skip):

? Projects directory [~/projects]:

Config written to ~/.config/devenv/config.toml
```

### Behavior
- If config already exists, prompt: `Config already exists. Overwrite? [y/N]`
- Fetches available SSH keys and regions from DO API to populate selection lists
- Tailscale auth key is optional at init time; can be set later with `devenv config set`

---

## `devenv config show`

Prints the full config file contents with secrets redacted.

```
devenv config show

[defaults]
  token             = "do_pat_****...****" (redacted)
  ssh_key_id        = "52790602"
  region            = "nyc3"
  size              = "s-2vcpu-4gb"
  tailscale_auth_key = "tskey-auth-****...****" (redacted)
  image             = "ubuntu-24-04-x64"
  projects_dir      = "~/projects"

[profiles]
  heavy:
    size   = "s-8vcpu-16gb"
    region = "sfo3"

[projects]
  myapp:
    repo   = "git@github.com:user/myapp.git"
    branch = "main"

[notify]
  provider = "telegram"
```

Redacted values show the first 8 and last 4 characters with `****` in between.

---

## `devenv config set <key> <value>`

Sets a single config value by dot-notation key.

### Examples

```bash
devenv config set defaults.region sfo3
devenv config set defaults.projects_dir ~/code
devenv config set projects.myapp.repo git@github.com:user/myapp.git
devenv config set projects.myapp.default_branch main
devenv config set notify.provider telegram
devenv config set notify.telegram.bot_token "..."
devenv config set notify.telegram.chat_id "..."
```

### Output
```
Set defaults.region = "sfo3"
```

### Validation
- Unknown keys: `Error: unknown config key "defaults.foo". Run 'devenv config show' to see valid keys.`
- Invalid size slug: warn but allow (DO API will validate at `up` time)

---

## `devenv config get <key>`

Gets a single value by dot-notation key.

```bash
devenv config get defaults.region
# nyc3

devenv config get defaults.token
# do_pat_****...****  (redacted)
```

Secrets are always redacted in `get` output.

---

## `devenv config profile create <name>`

Interactive wizard to create a named profile.

```
devenv config profile create heavy

Creating profile "heavy"

? Size:
  > s-8vcpu-16gb  ($72/mo, $0.107/hr)
    s-4vcpu-8gb   ($36/mo, $0.054/hr)

? Region (press Enter to use default nyc3):

Profile "heavy" created
Use it with: devenv up --profile heavy
```

---

## `devenv config profile list`

```
devenv config profile list

  PROFILE    SIZE             REGION
  default    s-2vcpu-4gb      nyc3    (from [defaults])
  heavy      s-8vcpu-16gb     sfo3
```

---

## `devenv config profile delete <name>`

```
devenv config profile delete heavy

Delete profile "heavy"? [y/N] y
Profile "heavy" deleted
```

- Cannot delete `default` (it's not a real profile entry, it's `[defaults]`)

---

## Config File Format

```toml
[defaults]
token             = "do_pat_..."
ssh_key_id        = "52790602"
region            = "nyc3"
size              = "s-2vcpu-4gb"
image             = "ubuntu-24-04-x64"
tailscale_auth_key = "tskey-auth-..."
git_identity_file = "~/.ssh/id_ed25519"
projects_dir      = "~/projects"

[profiles.heavy]
size   = "s-8vcpu-16gb"
region = "sfo3"

[projects.myapp]
repo           = "git@github.com:user/myapp.git"
default_branch = "main"

[projects.api]
repo           = "git@github.com:user/api.git"
default_branch = "develop"

[notify]
provider = "telegram"

[notify.telegram]
bot_token = "..."
chat_id   = "..."
```

---

## Environment Variable Overrides

| Env var | Config key |
|---|---|
| `DIGITALOCEAN_TOKEN` | `defaults.token` |
| `DEVENV_REGION` | `defaults.region` |
| `DEVENV_SIZE` | `defaults.size` |
| `DEVENV_SSH_KEY_ID` | `defaults.ssh_key_id` |
| `TAILSCALE_AUTH_KEY` | `defaults.tailscale_auth_key` |
| `DEVENV_PROJECTS_DIR` | `defaults.projects_dir` |

Environment variables take precedence over the config file but are overridden by explicit CLI flags.

---

## Error Cases

| Condition | Output | Exit code |
|---|---|---|
| Config file not found | Creates a new empty one, prompts for init | 0 |
| Corrupt TOML | `Error: failed to parse config: <details>. Fix or delete ~/.config/devenv/config.toml` | 1 |
| Unknown key in `set` | `Error: unknown config key "<key>"` | 1 |
| Unknown profile in `delete` | `Error: profile "<name>" does not exist` | 1 |

---

## Implementation Notes

- The `set` and `get` subcommands are designed for scripting — no prompts, clean single-line output.
- Config file is read once at startup (in `root.go`'s `PersistentPreRunE`) and injected into the cobra context. Subcommands do not re-read it.
- `config set` re-reads the file before writing to avoid clobbering concurrent changes.
