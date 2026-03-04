# PRD: `devenv notify`

## Overview

The `notify` command manages a pluggable notification system that alerts the user on their mobile device when a Claude Code agent requires input. It is the automation backbone of the "async development from anywhere" workflow.

The system has two sides:
1. **Configuration** (`devenv notify setup`) — run locally to configure which provider to use and store credentials
2. **Dispatch** (`devenv notify send`) — called on the remote droplet by the Claude Code hook whenever Claude needs input

---

## Motivation

Claude Code agents can run unattended for minutes or hours. The core mobile workflow depends on being notified the moment an agent blocks on a question — without that, the user must poll by opening Termius and checking manually. The notification hook turns passive polling into active push.

The article this project is based on used **Poke** (an iOS-only webhook-to-notification service). Since this setup targets **Android**, and different users prefer different messaging platforms, the notification mechanism is fully pluggable.

---

## Supported Providers

| Provider | Mechanism | Android support | Notes |
|---|---|---|---|
| `telegram` | Bot API HTTP POST | Yes (Telegram app) | Free, no extra app needed if Telegram is installed |
| `slack` | Incoming Webhook URL | Yes (Slack app) | Good for work setups |
| `discord` | Webhook URL | Yes (Discord app) | Good for personal setups |
| `webhook` | Generic HTTP POST | Any | Escape hatch for custom setups (ntfy.sh, Pushover, etc.) |

New providers can be added without breaking existing ones — the provider is a config value, not compiled-in logic.

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
  ▸ telegram   — Telegram bot (recommended for Android)
    slack      — Slack incoming webhook
    discord    — Discord webhook
    webhook    — Generic HTTP POST

--- Telegram setup ---

? Bot token (from @BotFather):
? Chat ID (your personal chat ID):

Sending test notification... ✓

Provider "telegram" configured ✓
Notifications will be sent when Claude agents need your input.
```

Each provider has its own setup flow with only the fields it needs.

### Telegram setup fields
- `bot_token` — token from @BotFather
- `chat_id` — user's personal chat ID (tip: get it from `@userinfobot`)

### Slack setup fields
- `webhook_url` — Incoming Webhook URL from Slack app configuration

### Discord setup fields
- `webhook_url` — Discord channel webhook URL

### Generic webhook setup fields
- `url` — endpoint to POST to
- `method` — `POST` (default) or `GET`
- `headers` — optional map of custom headers (e.g. for auth tokens)
- `body_template` — Go template string for the request body (default: `{"text": "{{.Message}}"}`)

---

## `devenv notify test`

Sends a test notification through the configured provider. Exits 0 on success, 1 on failure with a clear error.

```
devenv notify test

Sending test notification via telegram... ✓
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

---

## `devenv notify status`

```
devenv notify status

  Provider:  telegram
  Chat ID:   ********  (redacted)
  Bot token: ****...****  (redacted)
  Status:    configured ✓
```

If unconfigured:
```
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

The PostToolUse hook fires every time Claude emits an `AskUserQuestion` tool use. `$CLAUDE_TOOL_INPUT_QUESTION` is set by Claude Code and contains the question text. `$DEVENV_SESSION` is set by `devenv session start` in the tmux environment and identifies the session (see `prd-cmd-session.md`).

The PreToolUse hook fires before every tool use to reset session status back to `running` when Claude resumes after a question. Both hooks are fire-and-forget and exit 0 on failure.

### Hook script

Because cloud-init writes files before `devenv` binary is available on the droplet, the hook command uses `devenv notify send` — which means the `devenv` binary must also be installed on the droplet during provisioning.

The binary is downloaded during cloud-init from the GitHub releases page (or a DO Spaces bucket if a private distribution is preferred). This is the same binary used locally, since it's a static Go binary.

### Provider config on the droplet

The notification provider config must be available on the droplet for `devenv notify send` to work. `devenv up` injects it during cloud-init by rendering the provider credentials into the `~/.config/devenv/config.toml` on the droplet. The credentials are taken from the local config at `up` time and embedded in user-data.

**Security note:** user-data is not encrypted at rest on DO. Treat notification credentials with the same sensitivity as the Tailscale auth key — they are embedded in the provisioning payload. For high-sensitivity tokens, consider a `--no-notify` flag to skip injection and configure manually after SSH-ing in.

---

## Config Schema Extension

The `[notify]` section is added to `~/.config/devenv/config.toml`:

```toml
[notify]
provider = "telegram"

[notify.telegram]
bot_token = "..."
chat_id   = "..."

# OR:

[notify.slack]
webhook_url = "https://hooks.slack.com/services/..."

# OR:

[notify.discord]
webhook_url = "https://discord.com/api/webhooks/..."

# OR:

[notify.webhook]
url           = "https://ntfy.sh/mychannel"
method        = "POST"
body_template = "{{.Message}}"

[notify.webhook.headers]
Authorization = "Bearer ..."
```

---

## Provider Implementation

Each provider is a small struct implementing a `Notifier` interface:

```go
type Notifier interface {
    Send(ctx context.Context, title, message string) error
}

type TelegramNotifier struct { ... }
type SlackNotifier     struct { ... }
type DiscordNotifier   struct { ... }
type WebhookNotifier   struct { ... }
```

A `NewNotifier(cfg config.NotifyConfig) (Notifier, error)` factory function selects the right implementation based on `cfg.Provider`. Adding a new provider means implementing the interface and registering it in the factory — no changes to the command layer.

---

## Error Handling Philosophy

Notification failures must never crash or block a Claude Code agent. The hook is fire-and-forget:
- If the `devenv` binary is missing: the hook command fails silently
- If the provider returns an error: `devenv notify send` logs the error to stderr and exits 0 (Claude Code's hook runner does not treat stderr as fatal)
- If no provider is configured: exits 0 immediately

---

## Implementation Notes

- `devenv notify send` must work with zero user interaction (no prompts, no TTY required) — it's called non-interactively by Claude Code
- The Telegram provider is the recommended default for Android — it requires only a free Telegram account, a bot (created via @BotFather in 30 seconds), and no third-party apps beyond Telegram itself
- The generic `webhook` provider enables compatibility with self-hosted notification services like [ntfy.sh](https://ntfy.sh) (Android-native push, fully self-hostable) — worth highlighting in docs as a privacy-first option
