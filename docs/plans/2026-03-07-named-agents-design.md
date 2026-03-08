# Named Agents Configuration

## Summary

Replace the current inline agent config (`[defaults.agent]` with per-project overrides) with named agent definitions under `[agents.<name>]`. Agent selection happens at runtime via CLI flag or TUI picker, never tied to a project in config.

## Config Changes

### Remove

- `Agent AgentConfig` field from `DefaultsConfig`
- `Agent AgentConfig` field from `ProjectConfig`
- `ResolveAgent()` method and its merge logic

### Add

Top-level `Agents` map on `Config`:

```go
Agents map[string]AgentConfig `toml:"agents"`
```

`Agent` string field on `DefaultsConfig` (references a key in `[agents]`):

```go
Agent string `toml:"agent"`
```

New methods:

- `AgentByName(name string) (AgentConfig, error)` -- returns the named agent config or error if not found.
- `AgentNames() []string` -- returns sorted list of defined agent names.

Validation: if `defaults.agent` is set, it must reference an existing key in `[agents]`.

### Example config

```toml
[defaults]
agent = "claude"

[agents.claude]
cmd = "claude"
args = ["--dangerously-skip-permissions"]

[agents.claude.env]
CLAUDE_CONFIG_DIR = "/custom"

[agents.aider]
cmd = "aider"
args = ["--model", "opus"]
```

## CLI Changes

Add `--agent` flag to `session start`:

```
devenv session start <project> <branch> [--agent=claude] [--attach] [--no-create]
```

Resolution order:

1. `--agent` flag value
2. `defaults.agent` from config
3. Error: "no agent specified; use --agent or set defaults.agent in config"

Look up the resolved name via `cfg.AgentByName(name)` to get cmd/args/env, pass to `session.Service.Start()`.

Remove old `resolveAgentCmd()` and `resolveAgentEnv()` helpers. Replace with a single name-based lookup.

## TUI Changes

When a user presses enter on a worktree or project item with no running agent session:

1. Show a compact `huh.Select` picker with the list from `cfg.AgentNames()`.
2. Pre-select the agent matching `cfg.Defaults.Agent` (if set).
3. If only one agent is defined, skip the picker and use it directly.
4. If no agents are defined, show an error message.
5. On selection, look up config via `cfg.AgentByName(name)`, build cmd/env, call `session.Service.Start()`.

No changes to: worktree creation form, attach flow for already-running agents, or shell action.

## Testing

- **Config:** Remove `ResolveAgent` tests. Add tests for `AgentByName()`, `AgentNames()`, and validation that `defaults.agent` references an existing agent.
- **CLI:** Test `--agent` flag resolution (explicit flag > default > error).
- **TUI:** Test that `attachAction` calls the picker when no agent is running and passes the correct config to `session.Start()`.
- **Coverage:** Maintain 80% threshold via `make coverage`.
