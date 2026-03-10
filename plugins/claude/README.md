# devenv-session-status

A Claude Code plugin that tracks devenv session status and sends desktop notifications when agents need attention.

## What it does

- Monitors Claude Code hook events (PreToolUse, Notification, Stop)
- Marks sessions as "waiting" when the agent needs user input
- Marks sessions as "running" when the agent resumes work
- Adds a ⚡ prefix to tmux session names for quick visibility in `Ctrl+b s`
- Sends desktop notifications when a session needs attention
- Shows status annotations in the devenv TUI dashboard

## Prerequisites

- `devenv` must be installed and available in PATH
- Sessions must be started via `devenv session start`

## Installation

### From marketplace

Add the devenv marketplace and install:

```
/plugin marketplace add xico42/devenv
/plugin install devenv-session-status@devenv-plugins
```

### Local development

```bash
claude --plugin-dir ./plugins/claude
```
