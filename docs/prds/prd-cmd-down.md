# PRD: `devenv down`

## Overview

The `down` command destroys the active droplet tracked in local state, removes all associated resources, and clears the state file. It is the counterpart to `devenv up`.

---

## Command Interface

```
devenv down [flags]
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--force` | bool | `false` | Skip the confirmation prompt |
| `--snapshot` | bool | `false` | Save a DO snapshot before destroying (incurs snapshot storage cost) |
| `--snapshot-name` | string | auto-generated | Name for the snapshot (default: `devenv-snapshot-YYYYMMDD-HHMMSS`) |

### Inherited flags (from root)
- `--token` — override DO API token
- `--config` — override config file path
- `--no-color` — disable colored output

---

## Behavior

### Pre-flight checks

1. Read `~/.local/share/devenv/state.json`
   - If no state file / empty state: print `No active devenv droplet found.` and exit 0 (idempotent)
2. Unless `--force`: prompt for confirmation:
   ```
   Destroy devenv-20260304-143012 (nyc3, s-2vcpu-4gb)?
   This will permanently delete the droplet and all its data.
   Type the droplet name to confirm: _
   ```
   - The user must type the exact droplet name (not just "yes") to prevent accidental destruction
   - `--force` skips this entirely

### Snapshot (optional)

If `--snapshot`:
1. Inform user that snapshotting can take several minutes
2. Call `godo.DropletActionsService.Snapshot`
3. Poll the action until complete
4. Print snapshot ID and name on success

### Droplet destruction

1. Call `godo.DropletsService.Delete` with the droplet ID from state
2. On success: clear `~/.local/share/devenv/state.json`

### Output (normal flow)

```
Destroying devenv-20260304-143012 (nyc3, s-2vcpu-4gb)...

Type the droplet name to confirm: devenv-20260304-143012

Deleting droplet...  ✓
State cleared.

Droplet destroyed. Session duration: 1h 23m. Estimated cost: $0.08.
```

### Output (--force)

```
Destroying devenv-20260304-143012 (nyc3, s-2vcpu-4gb)...
Deleting droplet...  ✓
State cleared.

Droplet destroyed. Session duration: 1h 23m. Estimated cost: $0.08.
```

### Output (--snapshot)

```
Destroying devenv-20260304-143012 (nyc3, s-2vcpu-4gb)...
Saving snapshot devenv-snapshot-20260304-153512... ✓  (2m 14s)
Deleting droplet...  ✓
State cleared.

Droplet destroyed. Snapshot saved: devenv-snapshot-20260304-153512 (id: 987654321)
Session duration: 1h 23m. Estimated cost: $0.08.
```

### Output (no active droplet)

```
No active devenv droplet found.
```

### Error cases

| Condition | Output | Exit code |
|---|---|---|
| No token configured | `Error: no Digital Ocean token found.` | 1 |
| Confirmation mismatch | `Error: name did not match. Aborting.` | 1 |
| DO API delete error | `Error: failed to delete droplet: <api error>` | 1 |
| Droplet already deleted externally | Warning: `Droplet not found in DO (may have been deleted manually). Clearing local state.` then clears state | 0 |
| Snapshot timeout | `Error: snapshot timed out. Droplet NOT destroyed. Retry with --no-snapshot or wait and retry.` | 1 |

---

## Cost Estimation

The estimated cost displayed at the end is calculated from:
- `state.created_at` → now = session duration in fractional hours
- Droplet size hourly rate (hard-coded lookup table of common sizes, or fetched from DO sizes API at `up` time and stored in state)

This is an estimate only. Actual billing is handled by Digital Ocean.

---

## State After Completion

On success, `~/.local/share/devenv/state.json` is deleted (not zeroed — deleted).

---

## Implementation Notes

- The name-confirmation prompt is intentional UX friction to prevent `devenv down` from being accidentally run. `--force` exists for scripted use.
- If the droplet was deleted externally (API returns 404), the command should still exit 0 after clearing state — this is a valid recovery path, not an error.
- The DO snapshot action is asynchronous — poll `godo.ActionsService.Get` with the action ID until `status == "completed"`.
