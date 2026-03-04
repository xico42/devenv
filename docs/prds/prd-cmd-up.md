# PRD: `devenv up`

## Overview

The `up` command creates a new Digital Ocean droplet, waits for it to become active, and then waits for cloud-init provisioning to complete. It writes the droplet details to local state so other commands can reference it.

---

## Command Interface

```
devenv up [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--profile` | string | `"default"` | Named profile from config (controls size, region, image) |
| `--size` | string | from profile/config | Override droplet size slug (e.g. `s-2vcpu-4gb`) |
| `--region` | string | from profile/config | Override region slug (e.g. `nyc3`) |
| `--name` | string | auto-generated | Override droplet name (default: `devenv-YYYYMMDD-HHMMSS`) |
| `--wait` | bool | `true` | Wait for SSH to become available before returning |
| `--no-wait` | bool | `false` | Return immediately after droplet is created (don't wait for SSH) |
| `--dry-run` | bool | `false` | Print what would be created without creating it |
| `--no-git-key` | bool | `false` | Skip copying the git identity file to the droplet |

### Inherited flags (from root)
- `--token` — override DO API token
- `--config` — override config file path
- `--no-color` — disable colored output

---

## Behavior

### Pre-flight checks

Before creating anything, `up` must:

1. Verify config is loaded and DO token is present (fail fast with a clear error if not)
2. Verify no active state exists at `~/.local/share/devenv/state.json`
   - If state exists: print an error showing the existing droplet name and IP, and exit with a non-zero code
   - Do NOT silently destroy and recreate
3. If `--dry-run`: print the resolved config (profile, size, region, image, ssh key) and exit 0

### Droplet creation

1. Render cloud-init user-data from template (see `internal/provision`)
2. Call `godo.DropletsService.Create` with:
   - Name: `devenv-YYYYMMDD-HHMMSS` (or `--name` value)
   - Region: resolved region slug
   - Size: resolved size slug
   - Image: `ubuntu-24-04-x64` (or profile override)
   - SSHKeys: [`[ssh_key_id from config]`]
   - UserData: rendered cloud-init YAML
   - Tags: `["devenv"]`
3. Write initial state immediately after creation (droplet ID, name, region, size, created_at, status: `"provisioning"`)

### Waiting

Unless `--no-wait`:

1. Poll `godo.DropletsService.Get` every 5 seconds until droplet `status == "active"` — capture public IP at this point
2. Poll for Tailscale IP: once droplet is active, poll DO droplet networks or wait for Tailscale to register (configurable timeout: 120s)
3. Update state with public IP and Tailscale IP, status: `"active"`
4. Poll TCP port 22 until SSH is accepting connections (timeout: 120s)
5. Update state status: `"ready"`
6. If `defaults.git_identity_file` is configured and `--no-git-key` is not set: copy the private key to `~/.ssh/` on the droplet via SCP over Tailscale IP (see Git Identity below)

### Output (success)

```
Creating droplet devenv-20260304-143012...  ✓
Waiting for droplet to become active...      ✓  (23s)
Waiting for Tailscale IP...                  ✓  100.x.y.z
Waiting for SSH...                           ✓
Copying git identity...                      ✓  ~/.ssh/id_ed25519

  Droplet:  devenv-20260304-143012
  Region:   nyc3
  Size:     s-2vcpu-4gb
  IP:       100.x.y.z (Tailscale)
  SSH:      ssh ubuntu@100.x.y.z

Ready. Run 'devenv ssh' to connect.
```

### Output (dry-run)

```
[dry-run] Would create:
  Name:     devenv-20260304-143012
  Region:   nyc3
  Size:     s-2vcpu-4gb
  Image:    ubuntu-24-04-x64
  SSH key:  AbishaiV2 (52790602)
  Profile:  default
```

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| No token configured | `Error: no Digital Ocean token found. Set token in config or DIGITALOCEAN_TOKEN env var` | 1 |
| Active state exists | `Error: a droplet is already running (devenv-..., 100.x.y.z). Run 'devenv down' first.` | 1 |
| DO API error | `Error: failed to create droplet: <api error message>` | 1 |
| Droplet creation timeout | `Error: droplet did not become active within 5 minutes` | 1 |
| SSH timeout | Warning (non-fatal): droplet created but SSH not yet ready. State is written. | 0 |
| Git identity file not found | `Warning: git_identity_file ~/.ssh/id_ed25519 not found, skipping` | 0 |
| SCP copy fails | `Warning: failed to copy git identity: <error>. Run 'devenv ssh' and copy manually.` | 0 |

---

## Resolved Config Priority

For each parameter (size, region, image), resolution order (highest to lowest priority):

1. CLI flag (`--size`, `--region`)
2. Named profile (`--profile heavy` → `[profiles.heavy]` in config)
3. `[defaults]` in config
4. Built-in fallbacks: region=`nyc3`, size=`s-2vcpu-4gb`, image=`ubuntu-24-04-x64`

---

## Git Identity

If `defaults.git_identity_file` is set in config, `devenv up` copies the private key to the droplet after SSH is ready. This enables `git clone` via SSH for any host the key is registered with (GitHub, GitLab, etc.).

### Config

```toml
[defaults]
git_identity_file = "~/.ssh/id_ed25519"   # path to private key on local machine
```

### Behavior

- The private key is copied to `~ubuntu/.ssh/` on the droplet via `scp` over Tailscale
- Permissions are set to `600` on the remote file
- The corresponding `.pub` file is also copied if it exists alongside the private key
- `~/.ssh/config` on the droplet is written with `StrictHostKeyChecking=accept-new` so the first `git clone` doesn't prompt
- Both copy steps are non-fatal: failure prints a warning and `devenv up` exits 0

### Security

- Transfer is over Tailscale (WireGuard-encrypted) — not exposed to the public internet
- The key lives only on the ephemeral droplet filesystem; it is destroyed with the droplet on `devenv down`
- The key is **never** written to cloud-init user-data

---

## Cloud-Init Requirements

The user-data passed at creation time must:

- Install Docker (stable channel) and add the `ubuntu` user to the `docker` group
- Install mise globally (`/usr/local/bin/mise`)
- Install Tailscale and authenticate with `tailscale_auth_key` from config
- Install mosh
- Install tmux with a minimal config that auto-attaches on login
- Install Node.js via mise + install Claude Code globally (`npm install -g @anthropic-ai/claude-code`)
- Bootstrap `~/.claude/settings.json` for the `ubuntu` user with a `PostToolUse` hook that calls `devenv notify send` when Claude emits an `AskUserQuestion` tool use (see `docs/prds/prd-cmd-notify.md` for the hook script spec)
- Set hostname to the droplet name

See `docs/prds/prd-scaffolding.md` for the template location (`internal/provision/templates/user-data.yaml.tmpl`).

---

## State Written

On success, `~/.local/share/devenv/state.json`:
```json
{
  "droplet_id": 123456789,
  "droplet_name": "devenv-20260304-143012",
  "tailscale_ip": "100.x.y.z",
  "public_ip": "1.2.3.4",
  "region": "nyc3",
  "size": "s-2vcpu-4gb",
  "profile": "default",
  "created_at": "2026-03-04T14:30:12Z",
  "status": "ready"
}
```

---

## Implementation Notes

- All polling loops must respect `context.WithTimeout` and return clean errors on timeout
- The Tailscale IP polling step may be the most uncertain — consider a fallback of connecting via public IP if Tailscale IP is unavailable after timeout, logging a warning
- DO NOT use `time.Sleep` in a busy loop — use a ticker
