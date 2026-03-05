# PRD: `devenv status`

## Overview

The `status` command displays information about the currently active droplet: its name, IPs, region, size, uptime, and estimated cost so far. It reads from both local state and the DO API to show live status.

---

## Command Interface

```
devenv status [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--json` | bool | `false` | Output as JSON instead of human-readable |
| `--watch` | bool | `false` | Continuously refresh output every 5 seconds (like `watch`) |
| `--refresh-interval` | duration | `5s` | Interval for `--watch` mode |

### Inherited flags (from root)
- `--token` — override DO API token
- `--config` — override config file path
- `--no-color` — disable colored output

---

## Behavior

### No active droplet

If state file is empty or missing:
```
No active devenv droplet.
Run 'devenv up' to create one.
```
Exit 0.

### Normal output

Reads local state, then makes a single `godo.DropletsService.Get` call to fetch live status from DO.

```
devenv-20260304-143012
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Status:    ● active
  Region:    nyc3
  Size:      s-2vcpu-4gb (2 vCPU / 4 GB RAM)
  Profile:   default

  Tailscale: 100.x.y.z
  Public IP: 1.2.3.4

  Uptime:    1h 23m
  Est. cost: ~$0.08

  SSH:       ssh ubuntu@100.x.y.z
  Mosh:      mosh ubuntu@100.x.y.z
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

Status indicator uses color:
- `● active` — green
- `● provisioning` — yellow
- `● off` — red

### `--json` output

```json
{
  "droplet_id": 123456789,
  "droplet_name": "devenv-20260304-143012",
  "status": "active",
  "region": "nyc3",
  "size": "s-2vcpu-4gb",
  "profile": "default",
  "tailscale_ip": "100.x.y.z",
  "public_ip": "1.2.3.4",
  "created_at": "2026-03-04T14:30:12Z",
  "uptime_seconds": 4980,
  "estimated_cost_usd": 0.08
}
```

### `--watch` mode

Clears the terminal and re-renders the status block every `--refresh-interval`. Press `q` or `Ctrl+C` to exit.

```
devenv-20260304-143012                  Last updated: 15:53:42
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Status:    ● active
  ...
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
[q] quit
```

---

## Error Cases

| Condition | Output | Exit code |
|---|---|---|
| No token | `Error: no Digital Ocean token found.` | 1 |
| State exists but DO API 404 | `Warning: droplet not found in DO (deleted externally?). Run 'devenv down' to clear state.` | 0 |
| DO API error | `Error: failed to fetch droplet status: <message>` | 1 |

---

## Implementation Notes

- `--watch` mode does NOT use Bubble Tea — it uses simple terminal clear + reprint. The TUI PRD handles the interactive dashboard.
- The `--json` flag should always exit immediately (no `--watch` combination).
- `--watch` and `--json` together should error: `Error: --watch and --json cannot be used together`.
- Uptime and cost are derived from `state.created_at` — no additional API calls needed for those fields.
