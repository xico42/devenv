# PRD: Bubble Tea v2 TUI

## Overview

An interactive terminal dashboard built with [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) that provides a visual interface for managing devenv droplets. Invoked with `devenv tui` (or simply `devenv` with no subcommand).

The TUI is a quality-of-life layer on top of the existing commands — all actions it performs are the same as `devenv up`, `devenv down`, etc. It is optional; the CLI commands remain fully functional standalone.

---

## Command Interface

```
devenv tui [flags]
devenv         (no subcommand — launches TUI by default)
```

### Flags

| Flag | Type | Default | Description |
|---|---|---|---|
| `--no-tui` | bool | `false` | Force plain CLI output even when TUI is available (useful for scripts) |

---

## Library Stack

| Library | Version | Purpose |
|---|---|---|
| `github.com/charmbracelet/bubbletea/v2` | v2.x | Core event loop and model |
| `github.com/charmbracelet/lipgloss/v2` | v2.x | Styling, layout, borders |
| `github.com/charmbracelet/bubbles` | latest | Spinner, text input, list, progress bar components |

**Important:** Use Bubble Tea **v2**, not v1. The v2 API has breaking changes from v1 (notably the `Update` function signature and how `tea.Cmd` works with `WithContext`).

---

## Screens

### 1. Dashboard (default view)

Shown when a droplet is active. Refreshes every 5 seconds.

```
╭─ devenv ──────────────────────────────────────────────╮
│                                                        │
│  ● devenv-20260304-143012                    nyc3      │
│                                                        │
│  Status:    active                                     │
│  Size:      s-2vcpu-4gb  (2 vCPU / 4 GB)             │
│  Profile:   default                                    │
│                                                        │
│  Tailscale: 100.x.y.z                                 │
│  Public IP: 1.2.3.4                                   │
│                                                        │
│  Uptime:    1h 23m          Est. cost: ~$0.08          │
│                                                        │
│  ──────────────────────────────────────────────────   │
│  [s] SSH    [m] Mosh    [d] Destroy    [q] Quit        │
╰────────────────────────────────────────────────────────╯
```

Key bindings:
- `s` — exec SSH (exits TUI, hands off to SSH)
- `m` — exec Mosh (exits TUI, hands off to mosh)
- `d` — go to Destroy confirmation screen
- `r` — force refresh
- `q` / `Ctrl+C` — quit TUI

---

### 2. No Droplet Screen

Shown when no active state exists.

```
╭─ devenv ──────────────────────────────────────────────╮
│                                                        │
│  No active droplet.                                    │
│                                                        │
│  ──────────────────────────────────────────────────   │
│  [u] Spin up    [c] Config    [q] Quit                 │
╰────────────────────────────────────────────────────────╯
```

Key bindings:
- `u` — go to Spin Up screen
- `c` — go to Config screen
- `q` — quit

---

### 3. Spin Up Screen

Profile selection + confirmation before calling `up`.

```
╭─ devenv — Spin Up ────────────────────────────────────╮
│                                                        │
│  Select profile:                                       │
│                                                        │
│  ▸ default     s-2vcpu-4gb   nyc3   ~$0.027/hr        │
│    heavy       s-8vcpu-16gb  sfo3   ~$0.107/hr        │
│    minimal     s-1vcpu-1gb   nyc3   ~$0.009/hr        │
│                                                        │
│  ──────────────────────────────────────────────────   │
│  [↑↓] navigate   [Enter] confirm   [Esc] back          │
╰────────────────────────────────────────────────────────╯
```

After confirmation, transitions to Provisioning screen.

---

### 4. Provisioning Screen

Live status while `up` is running.

```
╭─ devenv — Provisioning ───────────────────────────────╮
│                                                        │
│  Creating devenv-20260304-163022...                    │
│                                                        │
│  ✓  Droplet created (id: 123456789)           2s      │
│  ✓  Droplet active                           18s      │
│  ⠸  Waiting for Tailscale IP...                       │
│  ·  Waiting for SSH...                                 │
│  ·  Ready                                             │
│                                                        │
│  ──────────────────────────────────────────────────   │
│  [Ctrl+C] cancel                                       │
╰────────────────────────────────────────────────────────╯
```

Steps use:
- `✓` (green) — completed
- `⠸` (yellow, animated spinner) — in progress
- `·` (dim) — pending

On completion: auto-transitions to Dashboard screen.
On Ctrl+C during provisioning: prompts "Droplet is being created. Destroy it? [y/N]"

---

### 5. Destroy Confirmation Screen

```
╭─ devenv — Destroy ────────────────────────────────────╮
│                                                        │
│  Destroy devenv-20260304-143012?                       │
│  This will permanently delete the droplet.             │
│                                                        │
│  Type the droplet name to confirm:                     │
│  > _                                                   │
│                                                        │
│  ──────────────────────────────────────────────────   │
│  [Enter] confirm   [Esc] cancel                        │
╰────────────────────────────────────────────────────────╯
```

Uses a `bubbles/textinput` component. Input is validated against the droplet name before enabling the Enter key action.

Transitions to Destroying screen on confirm, back to Dashboard on Esc.

---

### 6. Destroying Screen

```
╭─ devenv — Destroying ─────────────────────────────────╮
│                                                        │
│  ⠸  Deleting droplet devenv-20260304-143012...         │
│                                                        │
╰────────────────────────────────────────────────────────╯
```

On completion: auto-transitions to No Droplet screen.

---

### 7. Config Screen

Read-only view of current config (matches `devenv config show` output).

```
╭─ devenv — Config ─────────────────────────────────────╮
│                                                        │
│  [defaults]                                            │
│  token              do_pat_****...****                 │
│  ssh_key_id         52790602                           │
│  region             nyc3                               │
│  size               s-2vcpu-4gb                        │
│  tailscale_auth_key tskey-auth-****...****             │
│                                                        │
│  [profiles]                                            │
│  heavy: s-8vcpu-16gb / sfo3                           │
│                                                        │
│  ──────────────────────────────────────────────────   │
│  [Esc] back                                            │
╰────────────────────────────────────────────────────────╯
```

Config editing is out of scope for the TUI (use `devenv config set` for that).

---

## Model Design

The TUI uses a single top-level `Model` struct with a `screen` enum field to track which screen is active. Each screen has its own sub-model if it carries local state (e.g., text input content, list cursor position).

```go
type Screen int

const (
    ScreenDashboard Screen = iota
    ScreenNoDroplet
    ScreenSpinUp
    ScreenProvisioning
    ScreenDestroyConfirm
    ScreenDestroying
    ScreenConfig
)

type Model struct {
    screen      Screen
    state       *state.State        // active droplet info (may be nil)
    config      *config.Config
    doClient    do.Client

    // sub-models
    spinner     spinner.Model
    textInput   textinput.Model
    profileList list.Model

    // provisioning progress
    provSteps   []ProvisionStep

    // window size
    width, height int

    // error state
    err error
}
```

---

## Async Operations

Long-running operations (create droplet, poll for active, poll for SSH) run as `tea.Cmd` goroutines that send messages back to the update loop. They must NOT block the UI thread.

```go
type MsgDropletCreated struct{ dropletID int }
type MsgDropletActive  struct{ publicIP, tailscaleIP string }
type MsgSSHReady       struct{}
type MsgDestroyDone    struct{}
type MsgError          struct{ err error }
type MsgTick           struct{}   // for periodic refresh
```

---

## Styling

Use Lip Gloss v2. Define a central `styles` package with:

```go
var (
    BorderColor = lipgloss.Color("#7C3AED")  // purple accent
    GreenColor  = lipgloss.Color("#10B981")
    YellowColor = lipgloss.Color("#F59E0B")
    RedColor    = lipgloss.Color("#EF4444")
    DimColor    = lipgloss.Color("#6B7280")

    Box = lipgloss.NewStyle().
        Border(lipgloss.RoundedBorder()).
        BorderForeground(BorderColor).
        Padding(1, 2)

    Title = lipgloss.NewStyle().Bold(true)
    Dim   = lipgloss.NewStyle().Foreground(DimColor)
    // ...
)
```

---

## Responsive Layout

The TUI must handle terminal resize events (`tea.WindowSizeMsg`). The outer box should expand to fill the terminal but cap at a max width of 60 characters to remain readable on mobile (Termius on Android).

---

## Integration with CLI Commands

The TUI does NOT duplicate logic — it calls the same `internal/do`, `internal/state`, and `internal/provision` packages that the CLI commands use. The TUI is purely a presentation layer.

---

## Implementation Notes

- Bubble Tea v2 changed how `Init()`, `Update()`, and `View()` work compared to v1. Refer to the official v2 examples, not v1 tutorials.
- The provisioning screen's step list communicates progress via a channel that sends `tea.Msg` values — the `up` logic runs in a goroutine and sends step-completion messages.
- `devenv ssh` and `devenv ssh --mosh` require exiting the TUI (restoring the terminal) before exec-ing SSH. Use `tea.Quit` followed by `syscall.Exec`.
- Test the TUI with `--no-color` to ensure it degrades gracefully in environments that don't support ANSI.
