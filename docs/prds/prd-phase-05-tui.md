# PRD: Remote TUI (Phase 5 — Optional)

## Overview

An interactive terminal dashboard built with [Bubble Tea v2](https://github.com/charmbracelet/bubbletea) that runs **on the droplet** in its own tmux session (Layout A). It provides a visual interface for managing sessions, worktrees, and projects.

This is an optional phase. The tmux-native configuration from Phase 2 provides the baseline session management experience. The TUI adds polish: forms with validation, live-updating session status, formatted resource usage display.

**Plane:** Workload (runs on the droplet, in the `devenv` tmux session).

---

## Architecture: Layout A

The TUI runs in the `devenv` tmux session. Claude agent sessions run in separate tmux sessions:

```
tmux sessions:
  devenv           <- TUI runs here
  myapp-feature    <- Claude agent
  api-experiment   <- Claude agent
  myapp-fix-123    <- Claude agent
```

### Navigation

tmux intercepts its prefix key (`Ctrl+B`) before Bubble Tea sees any input. All tmux navigation works normally:

- `Ctrl+B s` — tmux session picker (shows all sessions)
- `Ctrl+B ( / )` — previous / next session
- `Ctrl+B T` — custom binding: jump back to devenv TUI session

When the TUI wants to send the user to a Claude session, it runs:
```bash
tmux switch-client -t myapp-feature
```

This switches the terminal view instantly. The TUI keeps running in the background.

For worktree shells, the TUI creates a new tmux window in its own session:
```bash
tmux new-window -t devenv -n "myapp-feature" -c <worktree_path>
```

`Ctrl+B 0` returns to the TUI window. `exit` closes the shell window automatically.

---

## What the TUI does NOT do

The TUI is a **workload-plane** tool. It does NOT manage droplet lifecycle:
- No "spin up" screen (use `devenv up` from local machine)
- No "destroy" screen (use `devenv down` from local machine)
- No droplet status (use `devenv status` from local machine)

---

## Screens

### 1. Session Dashboard (default view)

Shows all active sessions with live status. Refreshes every 5 seconds.

```
devenv
----

  SESSION          STATUS              CPU    MEM     SINCE
  myapp-feature    WAITING FOR INPUT    0%    420MB   3m ago
  api-experiment   running             14%    380MB   12m ago
  myapp-fix-123    running              8%    390MB   7m ago

  Worktrees: 4 active across 2 projects

----
[a]ttach  [n]ew session  [s]top  [w]orktrees  [p]rojects  [q]uit
```

Key bindings:
- `a` — select a session to attach (switches tmux client)
- `n` — new session form
- `s` — stop a session (with confirmation)
- `w` — switch to Worktrees view
- `p` — switch to Projects view
- `q` / `Ctrl+C` — quit TUI (returns to shell in devenv tmux session)

A session with status `WAITING FOR INPUT` is highlighted.

### 2. Session Detail (on Enter)

Shows full detail for the selected session, including the question text if waiting.

```
myapp-feature
----

  Project:   myapp
  Branch:    feature
  Path:      ~/projects/github.com/user/myapp__worktrees/feature
  Status:    WAITING FOR INPUT
  Question:  "Should I overwrite the existing migration? [y/N]"
  Since:     3m ago
  CPU:       0%
  MEM:       420MB
  Started:   2026-03-04T16:22:01Z

----
[a]ttach  [s]top  [Esc] back
```

### 3. New Session Form

```
New Session
----

  Project:  [myapp     v]     <- select from configured projects
  Branch:   [feature_____ ]   <- text input, tab-completes from existing branches

  Worktree ~/projects/github.com/user/myapp__worktrees/feature will be created if it doesn't exist.

----
[Enter] start  [Esc] cancel
```

On submit:
- Creates worktree if needed (`devenv worktree new`)
- Starts session (`devenv session start`)
- Optionally attaches (`--attach` equivalent)

### 4. Worktrees View

```
Worktrees
----

  PROJECT    BRANCH        SESSION
  myapp      main          --
  myapp      feature       myapp-feature (WAITING)
  myapp      fix-123       myapp-fix-123 (running)
  api        develop       --
  api        experiment    api-experiment (running)

----
[n]ew worktree  [d]elete  [s]hell  [Esc] back
```

### 5. Projects View

```
Projects
----

  NAME      REPO                                 BRANCH    CLONED
  myapp     git@github.com:user/myapp.git        main      yes
  api       git@github.com:user/api.git          develop   yes
  frontend  git@github.com:user/frontend.git     main      no

----
[c]lone  [Esc] back
```

---

## Library Stack

| Library | Version | Purpose |
|---|---|---|
| `github.com/charmbracelet/bubbletea/v2` | v2.x | Core event loop and model |
| `github.com/charmbracelet/lipgloss/v2` | v2.x | Styling, layout |
| `github.com/charmbracelet/bubbles` | latest | Spinner, text input, list, table |

---

## Design Constraints for Mosh

The TUI must work over mosh (the primary mobile connection method):

- **256 colors only** — no true color (mosh doesn't support it)
- **No mouse** — mosh mouse support is limited and conflicts with tmux
- **Keyboard-only navigation** — all actions via key bindings
- **Slow animation intervals** (250ms+) — mosh batches updates, fast spinners look stuttery
- **No cursor shape changes** — mosh doesn't support them
- **Handle delayed resize events** — mosh propagates SIGWINCH with latency

---

## Responsive Layout

The TUI must handle terminal resize events (`tea.WindowSizeMsg`). Cap at a max width of 80 characters to remain readable on mobile (Termius on Android).

---

## Integration with CLI Commands

The TUI does NOT duplicate logic. It calls the same workload commands internally:
- `devenv session list` logic for the dashboard
- `devenv session start` logic for new session form
- `devenv worktree list/new` logic for worktree views
- `devenv project list/clone` logic for project views

The TUI is purely a presentation layer over the existing primitives.

---

## Implementation Notes

- Bubble Tea v2 has breaking API changes from v1. Use v2 examples as reference.
- The TUI replaces the shell in the `devenv` tmux session — when it exits, the user gets a shell back
- `tmux switch-client` commands are run via `os/exec` from within the TUI process — this works because tmux client commands can be executed from inside tmux
- Test the TUI with `--no-color` to ensure it degrades gracefully
- The TUI is launched by `devenv tui` command. Cloud-init can optionally set it as the default command in the `devenv` tmux session
