# PRD: `devenv notify` (Phase 3 — Both Planes)

## Overview

The `notify` command manages a pluggable notification system that alerts the user on their mobile device when a Claude Code agent requires input. It is the automation backbone of the "async development from anywhere" workflow.

The system has two sides:
1. **Configuration** (`devenv notify setup/test/status`) — run locally to configure which provider to use and store credentials
2. **Dispatch** (`devenv notify send`) — called on the remote droplet by the Claude Code hook whenever Claude needs input

**Plane:** Infrastructure (setup/test/status on local machine) + Workload (send runs anywhere).

---

## Motivation

Claude Code agents can run unattended for minutes or hours. The core mobile workflow depends on being notified the moment an agent blocks on a question. The notification hook turns passive polling into active push.

The article this project is based on used **Poke** (an iOS-only webhook-to-notification service). Since this setup targets **Android**, and different users prefer different messaging platforms, the notification mechanism is fully pluggable.

---

## Supported Providers

| Provider | Mechanism | Android support | Notes |
|---|---|---|---|
| `telegram` | Bot API HTTP POST | Yes (Telegram app) | Free, no extra app needed if Telegram is installed |
| `slack` | Incoming Webhook URL | Yes (Slack app) | Good for work setups |
| `discord` | Webhook URL | Yes (Discord app) | Good for personal setups |
| `webhook` | Generic HTTP POST | Any | Escape hatch for custom setups (ntfy.sh, Pushover, etc.) |

---

## Command Interface

```
devenv notify <subcommand> [flags]
```

### Subcommands

| Subcommand | Description |
|---|---|
| `setup` | Interactive wizard to configure a notification provider |
| `test` | Send a test notification through the configured provider |
| `send` | Send a notification (called by the Claude Code hook on the droplet) |
| `status` | Show currently configured provider |

---

## `devenv notify setup`

Interactive wizard. Run locally. Stores provider config in `~/.config/devenv/config.toml`.

```
devenv notify setup

? Notification provider:
  > telegram   -- Telegram bot (recommended for Android)
    slack      -- Slack incoming webhook
    discord    -- Discord webhook
    webhook    -- Generic HTTP POST

--- Telegram setup ---

? Bot token (from @BotFather):
? Chat ID (your personal chat ID):

Sending test notification... done

Provider "telegram" configured
Notifications will be sent when Claude agents need your input.
```

---

## `devenv notify test`

Sends a test notification through the configured provider. Exits 0 on success, 1 on failure with a clear error.

```
devenv notify test

Sending test notification via telegram... done
Check your device.
```

---

## `devenv notify send`

Sends a single notification. This is the command called by the Claude Code hook on the droplet.

```
devenv notify send [flags]
```

### Flags

| Flag | Type | Required | Description |
|---|---|---|---|
| `--message` | string | yes | Notification body text |
| `--title` | string | no | Notification title (default: `devenv`) |
| `--provider` | string | no | Override configured provider |
| `--session` | string | no | Session name — if set, updates the session state file to `waiting` as a side effect |

### Example (called by hook)
```bash
devenv notify send \
  --title "Claude needs input" \
  --message "Should I overwrite existing migrations? [y/n]" \
  --session "$DEVENV_SESSION"
```

### Behaviour
- Reads provider config from `~/.config/devenv/config.toml`
- POSTs/dispatches to the configured provider
- Exits 0 on success, 1 on failure (hook can log the failure without crashing Claude)
- If no provider is configured: exits 0 silently (fail-open — don't interrupt the agent)
- If `--session` is passed: writes the "waiting" state to the session state file as a side effect

---

## `devenv notify status`

```
devenv notify status

  Provider:  telegram
  Chat ID:   ********  (redacted)
  Bot token: ****...****  (redacted)
  Status:    configured

If unconfigured:
  No notification provider configured.
  Run 'devenv notify setup' to configure one.
```

---

## Claude Code Hook Integration

### What gets bootstrapped on the droplet

`devenv up` writes `~/.claude/settings.json` for the `ubuntu` user during cloud-init:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "AskUserQuestion",
        "hooks": [
          {
            "type": "command",
            "command": "devenv notify send --title 'Claude needs input' --message \"$CLAUDE_TOOL_INPUT_QUESTION\" --session \"$DEVENV_SESSION\""
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "devenv session mark-running --session \"$DEVENV_SESSION\""
          }
        ]
      }
    ]
  }
}
```

The PostToolUse hook fires every time Claude emits an `AskUserQuestion` tool use. `$DEVENV_SESSION` is set by `devenv session start` in the tmux environment.

The PreToolUse hook fires before every tool use to reset session status back to `running`. Both hooks are fire-and-forget and exit 0 on failure.

### Provider config on the droplet

The notification provider config must be available on the droplet for `devenv notify send` to work. `devenv up` injects it during cloud-init by rendering the provider credentials into `~/.config/devenv/config.toml` on the droplet.

**Security note:** user-data is not encrypted at rest on DO. Treat notification credentials with the same sensitivity as the Tailscale auth key.

---

## Provider Implementation

Each provider is a small struct implementing a `Notifier` interface:

```go
type Notifier interface {
    Send(ctx context.Context, title, message string) error
}
```

A `NewNotifier(cfg config.NotifyConfig) (Notifier, error)` factory function selects the right implementation based on `cfg.Provider`.

---

## Error Handling Philosophy

Notification failures must never crash or block a Claude Code agent. The hook is fire-and-forget:
- If the `devenv` binary is missing: the hook command fails silently
- If the provider returns an error: `devenv notify send` logs the error to stderr and exits 0
- If no provider is configured: exits 0 immediately
