# Design: `devenv tui`

## Overview

An interactive terminal dashboard built with Bubble Tea v2 that provides a visual interface for managing worktrees, agent sessions, and shell sessions. Launched via `devenv tui`.

**Plane:** Workload (runs anywhere, no SSH, no droplet required).

## Decisions

### Single list view with `bubbles/v2/list`

One screen, one list. No tabs, no separate screens. The `bubbles/list` component provides built-in fuzzy filtering (`/`), pagination, keyboard navigation (`j/k`, arrows), and a help bar. A custom `ItemDelegate` renders each item as two lines with styled status tags.

Alternatives considered:
- **`bubbles/table`** — natural for columnar data but lacks filtering, inflexible layout, wastes space with empty cells.
- **Two tabs (Active / Projects)** — adds state management and navigation complexity. The single list's ordering and filtering achieve the same result with less code.
- **Separate screens (PRD approach)** — all three workflows start from "which project/branch?", making separate screens unnecessary navigation.

### Bubble Tea v2 stack

All TUI code uses the v2 releases:

| Package | Version | Import path |
|---|---|---|
| bubbletea | v2.0.1 | `github.com/charmbracelet/bubbletea/v2` |
| bubbles | v2.0.0 | `github.com/charmbracelet/bubbles/v2/*` |
| lipgloss | v2.0.0 | `github.com/charmbracelet/lipgloss/v2` |

Key v2 differences from v1:
- `View()` returns `tea.View` (use `tea.NewView()`), not `string`.
- Key messages are `tea.KeyPressMsg`, not `tea.KeyMsg`.
- `Init()` returns `tea.Cmd`.

The existing `huh` v0.8.0 (used in `cmd/config.go`) stays on bubbletea v1. Both coexist — different Go module paths. The TUI does not use `huh`.

### Item rendering: tags, not columns

Each list item is two lines rendered by a custom `ItemDelegate`:

- **Line 1:** `project / branch` (or just `project` for bare project items).
- **Line 2:** Styled tags — `WAITING FOR INPUT`, `running`, `shell`, `cloned`, `not cloned`.

Tags are lipgloss-styled inline badges. `WAITING FOR INPUT` gets bold/highlight treatment. `running` is subtle. `shell` is a distinct color. Missing tags are omitted (no `--` placeholders).

### Three-group ordering

Items are sorted into three priority groups. Within each group, alphabetical by project, then by branch.

1. **Worktrees with active agents** — any worktree whose agent session (`project-branch`) exists in tmux. These are what you're actively working on.
2. **Worktrees without agents** — worktrees that exist on disk but have no agent session. May have a shell session.
3. **Projects without worktrees** — configured projects (from `config.Projects`) that have no worktrees. Shows clone status (`cloned` / `not cloned`).

This ordering puts "needs attention" at the top and "available to start" at the bottom. Ordering is stable — items only move between groups when you explicitly start/stop agents or create/delete worktrees. The filter (`/`) searches by project and branch name across all groups.

### Shell sessions: tmux-only, `~sh` suffix

Shell sessions are persistent tmux sessions for interactive work in a worktree. They follow the same model as agent sessions but without a state file.

- Naming: `project-branch~sh` (e.g., `myapp-feature~sh`).
- New semconv function: `ShellSessionName(project, branch)`.
- Discovery: `tmux has-session -t name`. No JSON state file.
- Creation: `tmux new-session -d -s name -c <worktree_path>` (no command — drops into `$SHELL`).

The `~sh` suffix avoids collisions with agent sessions (`myapp-feature`).

### Actions and key bindings

| Key | Action | Groups | Behavior |
|---|---|---|---|
| `a` / `Enter` | Attach agent | 1, 2, 3 | Start agent if needed (clone + create worktree if needed), then `tmux switch-client` |
| `s` | Open shell | 1, 2, 3 | Create shell tmux session if needed (clone + create worktree if needed), then switch |
| `c` | Clone project | 3 (not cloned) | Clone the project |
| `n` | New worktree | all | Inline form: project select + branch text input. Clones if needed. |
| `d` | Delete worktree | 1, 2 | Kill agent session + shell session + remove worktree. Confirmation prompt. |
| `r` | Force refresh | all | Re-poll immediately |
| `/` | Filter | all | Built-in list fuzzy search |
| `?` | Toggle help | all | Built-in help component |
| `q` / `Ctrl+C` | Quit | all | Exit TUI |

When pressing `a` on a group 3 item (project without worktree): clone if needed, create worktree (prompts for branch), start agent, switch. The full chain runs automatically.

### Attach behavior: `tmux attach-session`

The TUI always runs outside tmux. Attach uses `tmux attach-session -t <name>`, which takes over the terminal. The TUI process suspends while the user is in the tmux session. When the user detaches (`Ctrl+B d`), they return to the TUI.

New tmux client method: `AttachSession(name string) error` — wraps `syscall.Exec` to replace the process with `tmux attach-session -t <name>`. The TUI exits before attaching; the user re-launches `devenv tui` after detaching.

### "New worktree" inline form

Pressing `n` replaces the list with a two-field form built with `bubbles/v2/textinput`:

```
New Worktree
----
  Project:  ▸ myapp  /  api  /  frontend
  Branch:   [feature-auth          ]
----
Enter: create  |  Esc: cancel
```

Project selection cycles through configured projects. Branch is free text. On submit: clone if needed, create worktree, return to list. The new worktree appears in the list at group 2.

This is a separate "screen" in the multi-screen enum pattern (list view vs. form view), routed in the top-level `Update`.

### Auto-refresh

`tea.Tick` fires every 3 seconds. On each tick, the TUI re-polls:
- `worktree.Service.List()` for all worktrees and their linked sessions.
- `tmux has-session` for shell sessions (via the `~sh` naming convention).
- Config projects for group 3 items.

The list is rebuilt from fresh data. The selected item is preserved by matching on project+branch after refresh.

### Delete behavior

Pressing `d` on a worktree item (groups 1-2):
1. Shows inline confirmation: `Delete myapp/feature? [y/n]`
2. Kills agent session (`myapp-feature`) if it exists.
3. Kills shell session (`myapp-feature~sh`) if it exists.
4. Removes the git worktree.

This extends the existing `worktree.Delete` behavior to also handle shell sessions.

### Services: direct usage, no abstraction

The TUI directly instantiates `session.Service` and `worktree.Service`, same as the CLI commands in `cmd/`. There is no Provider, Facade, or Backend abstraction.

Code duplication between `cmd/` and the TUI (e.g., orchestrating "check worktree, create if needed, start session") is accepted. A reuse strategy will be considered later once usage patterns are clear.

## Layout

```
devenv                                                 3 agents

  ▸ myapp / feature
    WAITING FOR INPUT  shell

    myapp / fix-123
    running

    api / experiment
    shell

    api / develop

    frontend
    cloned

    infra
    not cloned

a agent  s shell  n new  d delete  / filter  ? help  q quit
```

Structure (top to bottom):
- **Title bar:** app name + agent count. Styled with lipgloss.
- **List:** `bubbles/v2/list` with custom delegate. Handles its own pagination and scrolling.
- **Help bar:** `bubbles/v2/help` with custom key map. Shows short help by default, full help on `?`.

Terminal resize is handled via `tea.WindowSizeMsg`. The list resizes to fill available height. Max width capped at 80 characters for readability over mosh on mobile.

## Mosh constraints

The TUI must work over mosh (primary mobile connection):
- 256 colors only (no true color).
- Keyboard-only navigation (no mouse).
- Slow animation intervals (250ms+ for spinners).
- Handle delayed resize events.

## New code

| Location | Purpose |
|---|---|
| `internal/tui/` | Bubble Tea app: model, update, view, delegates, keys |
| `internal/tui/model.go` | Top-level model, screen enum, refresh logic |
| `internal/tui/delegate.go` | Custom `ItemDelegate` for worktree/project items |
| `internal/tui/keys.go` | Key map and help integration |
| `internal/tui/items.go` | Item types implementing `list.Item` |
| `internal/tui/form.go` | "New worktree" inline form |
| `cmd/tui.go` | `devenv tui` Cobra command |
| `internal/semconv/semconv.go` | Add `ShellSessionName()` |
| `internal/tmux/client.go` | Add `AttachSession()` |

## Future considerations

These are not in scope but informed the design:

- **Provider abstraction.** The existing `session.Service` and `worktree.Service` could become interfaces with local/remote implementations. The TUI would hold multiple providers and merge results. This enables managing both local and remote (droplet) sessions from a single view.
- **Wish (SSH server).** A `devenv serve` command could run a Wish server on the droplet, providing: (a) a management API with push-based status updates instead of polling, (b) serving the TUI itself over SSH for thin clients like mobile. The TUI's attach action would adapt based on context — `tmux switch-client` when local, `ssh -t tmux attach` when remote.
- **Facade / orchestrator.** The orchestration logic duplicated between CLI and TUI (clone → create worktree → start session) could be extracted into a `devenv.Devenv` struct that both consume. Deferred until usage patterns clarify the right interface.
