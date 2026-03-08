# Design: Auto-create worktree on `session start`

## Problem

`devenv session start <project> <branch>` fails with `ErrWorktreeNotFound` when the
worktree does not exist. The user must first run `devenv worktree new <project> <branch>`
manually before starting a session — an unnecessary extra step.

## Goal

When a worktree is missing, `session start` creates it automatically, prints what it is
doing, and proceeds. An opt-out flag preserves the old fail-fast behavior.

## Command interface

```
devenv session start [--attach] [--no-create] <project> <branch>
```

New flag:

| Flag | Default | Description |
|---|---|---|
| `--no-create` | false | Skip auto-creation; fail if worktree does not exist |

## Logic change in `sessionStartCmd.RunE`

Replace the current single call to `wtSvc.WorktreePath()` with this sequence:

1. Call `wtSvc.WorktreePath(project, branch)`.
2. If it returns `ErrWorktreeNotFound` **and** `--no-create` is not set:
   - Print `"Worktree <project>/<branch> not found, creating...  "`
   - Call `wtSvc.New(project, branch)`; on error route through `worktreeErr()` and return.
   - Print `"done"`
   - Use `result.Path` as the resolved worktree path.
3. Otherwise (error with `--no-create`, or any other error) fall through to the existing
   `sessionErr()` handler unchanged.
4. Proceed with the resolved path into `svc.Start(...)`.

No changes to `worktree.Service`, `session.Service`, or any other package.

## Error handling

| Scenario | Behavior |
|---|---|
| Worktree missing, `--no-create` not set | Auto-create, then start |
| Worktree missing, `--no-create` set | Fail via `sessionErr` (existing message) |
| Project not cloned | Fail via `worktreeErr` — nothing to auto-create |
| `wtSvc.New()` fails during auto-creation | Fail via `worktreeErr` |

## Tests

- Existing `session start` tests are unaffected (they pre-create the worktree).
- **New:** `session start` with missing worktree and no `--no-create` → worktree created,
  session starts successfully.
- **New:** `session start` with missing worktree and `--no-create` → fails with
  `ErrWorktreeNotFound`.
