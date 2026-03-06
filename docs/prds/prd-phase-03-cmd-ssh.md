# PRD: `devenv ssh` (Phase 3 — Infrastructure)

## Overview

The `ssh` command connects to the active droplet via SSH (using the Tailscale IP by default). It is a thin wrapper that constructs and execs the correct SSH invocation so the user doesn't need to remember IPs or usernames.

**Plane:** Infrastructure (runs on local machine only).

---

## Command Interface

```
devenv ssh [flags] [-- <remote command>]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--user` | string | `"ubuntu"` | Remote username |
| `--identity` | string | `""` | Path to SSH private key (overrides default key lookup) |
| `--mosh` | bool | `false` | Use mosh instead of ssh (resilient connection, recommended for mobile) |
| `--public-ip` | bool | `false` | Force use of public IP instead of Tailscale IP |
| `--tmux` | bool | `true` | Automatically attach to (or create) a tmux session on connect |
| `--no-tmux` | bool | `false` | Skip tmux auto-attach |
| `--print` | bool | `false` | Print the SSH command instead of executing it |

### Arguments

Everything after `--` is passed as a remote command to execute non-interactively:

```bash
devenv ssh -- "tmux list-sessions"
devenv ssh -- "docker ps"
```

### Inherited flags (from root)
- `--config` — override config file path
- `--no-color` — disable colored output

---

## Behavior

### Pre-flight checks

1. Read state file. If empty: `Error: no active droplet. Run 'devenv up' first.`
2. Determine target IP:
   - Default: Tailscale IP from state
   - If `--public-ip`: use public IP from state
   - If neither IP is available in state: error

### SSH connection

The command builds an SSH invocation and `exec`s it (replaces the devenv process — no subprocess wrapping):

```bash
ssh -o StrictHostKeyChecking=no \
    -o UserKnownHostsFile=/dev/null \
    [-i <identity>] \
    ubuntu@100.x.y.z
```

When `--tmux` (default): appends `-t tmux new-session -A -s main` as the remote command, so connecting always lands in (or creates) a tmux session named `main`.

### Mosh mode (`--mosh`)

Constructs a `mosh` invocation instead:

```bash
mosh ubuntu@100.x.y.z -- tmux new-session -A -s main
```

`mosh` must be installed locally. If not found in PATH: `Error: mosh is not installed locally. Install it or use 'devenv ssh' without --mosh.`

### `--print` mode

Prints the command that would be executed, without executing it. Useful for scripting or debugging.

### Remote command mode (`-- <command>`)

When a remote command is provided:
- `--tmux` is automatically disabled (non-interactive)
- SSH is invoked with `-o BatchMode=yes`
- Exit code of `devenv ssh -- <cmd>` equals the remote command's exit code

---

## Output

This command produces no decorative output when executing normally — it simply hands off to SSH/mosh. Only error conditions produce output.

---

## Error Cases

| Condition | Output | Exit code |
|---|---|---|
| No active droplet | `Error: no active droplet. Run 'devenv up' first.` | 1 |
| No Tailscale IP in state | `Error: no Tailscale IP available. Try 'devenv ssh --public-ip'.` | 1 |
| mosh not installed locally | `Error: mosh is not installed locally.` | 1 |
| SSH connection refused | SSH's own error output (we exec, so its output is passed through) | SSH's exit code |

---

## StrictHostKeyChecking

`StrictHostKeyChecking=no` and `UserKnownHostsFile=/dev/null` are intentional defaults because:
- Droplets are ephemeral — the host key changes every time
- Tailscale provides the trust layer
- Persistent `known_hosts` entries for ephemeral IPs cause confusing "host key changed" errors

---

## Implementation Notes

- Use `syscall.Exec` (Unix exec, not `os/exec`) to replace the devenv process with SSH. This ensures signals (Ctrl+C, Ctrl+Z, window resize SIGWINCH) are handled naturally by SSH, not by devenv.
- On non-Unix systems (Windows), fall back to `os/exec` and document the limitation.
- The `--identity` flag maps to SSH's `-i` flag. When not set, rely on the SSH agent / default key discovery.
