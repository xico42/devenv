# Contextual Worktree Creation Form — Design

## Problem

The TUI `n` key opens a worktree creation form that ignores the selected item. The user must manually pick a project, and there is no way to branch from an existing worktree. After creating a worktree, starting a coding session requires a separate action.

The CLI (`devenv worktree new`) only accepts a project — there is no way to specify a base branch.

## Solution

Make `n` contextual: pre-fill the form from the selected item, show a combined worktree-creation-plus-session-attach form using `charm/huh`, and add a `--from` flag to the CLI.

## Huh v2 Upgrade

Upgrade `charmbracelet/huh` from v0.8.0 to v2.0.1 (Bubble Tea v2 compatible). This is a prerequisite isolated into its own commit. Existing huh usage in `cmd/config.go` (config init wizard, profile create, profile delete confirm) must be updated to the v2 API.

## TUI Form

### Selected item → form context

| Selected item | Base project | Base branch |
|---|---|---|
| Project (no worktrees) | that project | project's default branch |
| Worktree (with or without session) | worktree's project | worktree's branch |

### Form layout

Single `huh.Form` with two groups:

**Group 1 — Worktree details (always visible):**

| Field | Type | Behavior |
|---|---|---|
| Base info | `huh.Note` | Read-only. Shows project name and base branch |
| Branch name | `huh.Input` | Text input for the new branch name. Validated non-empty |
| Attach session? | `huh.Confirm` | Yes/No toggle, defaults to Yes |

**Group 2 — Agent selection (conditional):**

| Field | Type | Behavior |
|---|---|---|
| Agent | `huh.Select[string]` | Populated from `cfg.AgentNames()` |

Group 2 is hidden via `WithHideFunc` when:
- Attach session is No, OR
- Only one agent is configured (auto-selected silently)

### Navigation

Standard huh keybindings — no customization needed:

- **Tab / Enter** — next field
- **Shift+Tab** — previous field
- **Left/Right** — toggle Confirm yes/no
- **Up/Down** — navigate Select options
- **Enter** on last field — submit
- **Esc** — cancel (handled by parent TUI model before forwarding to huh)

### Form completion

1. Create worktree via `wtSvc.New()` with `FromBranch` set to base branch (when base differs from default branch)
2. If attach=yes, start session (using selected agent, or the single configured agent)
3. Set `PendingAttach` on the TUI model to quit and attach to the session

### TUI model changes

- Replace `formModel` struct with embedded `*huh.Form`
- `showForm()` receives the selected list item, extracts project/branch, builds the huh form
- `screenAgentPicker` remains for the "start session on existing worktree" flow (pressing `a`)
- Check `form.State == huh.StateCompleted` to read bound values and trigger actions

## CLI Changes

### `devenv worktree new`

New flags:

| Flag | Type | Description |
|---|---|---|
| `--from` | string | Base branch to create the new worktree from. Omitted = default branch (current behavior) |
| `--attach` | bool | Start a coding session after creating the worktree |
| `--agent` | string | Agent to use for the session. Required if `--attach` is set and multiple agents are configured |

```
devenv worktree new myproject my-feature --from feature-auth --attach --agent claude
```

## Worktree Service Changes

### `NewRequest` struct

Add `FromBranch string` field.

### `Service.New()` git strategy

1. If `FromBranch` is set → `git worktree add -b <new-branch> <path> <from-branch>`
2. If `FromBranch` is empty → current behavior (try existing branch, then create from HEAD)

No changes to `.env` template processing, path resolution, or validation.

## Testing

| Area | Approach |
|---|---|
| Worktree service | Unit tests for `New()` with `FromBranch`, verify correct git command via `WorktreeRunner` mock |
| CLI flags | Test `--from`, `--attach`, `--agent` parsing and forwarding |
| TUI form | Test `showForm()` builds correct huh form for project vs worktree items; test completion triggers correct actions |
| Huh v2 upgrade | Run existing tests after upgrade to verify `cmd/config.go` forms still work |

## Changes by area

| Area | Change |
|---|---|
| `go.mod` | Upgrade `charmbracelet/huh` to v2.0.1 |
| `cmd/config.go` | Update huh usage to v2 API |
| `internal/tui/form.go` | Replace hand-rolled form with embedded `huh.Form` |
| `internal/tui/model.go` | Wire huh form into screen state machine |
| `internal/tui/actions.go` | Form completion → worktree create → optional session start → optional attach |
| `internal/worktree/worktree.go` | Add `FromBranch` to `NewRequest`; update git command logic |
| `cmd/worktree.go` | Add `--from`, `--attach`, `--agent` flags |
| Tests | All areas above |

## Scope

- Local sessions only (remote deferred)
- Agent picker screen unchanged for the "attach to existing worktree" flow
- Delete flow unchanged
- List view grouping unchanged

## Implementation order

1. Upgrade huh to v2 (isolated commit)
2. Add `FromBranch` to worktree service + tests
3. Add `--from`, `--attach`, `--agent` CLI flags
4. Replace TUI form with huh-based form
5. Wire form completion → worktree creation → optional session start → optional attach
