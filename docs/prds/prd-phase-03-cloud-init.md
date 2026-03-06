# PRD: Cloud-Init Template (Phase 3 — Provisioning)

## Overview

The cloud-init user-data template defines what gets installed and configured on every droplet created by `devenv up`. It is rendered at creation time with values from the local config and passed as user-data to the DO API.

This PRD specifies the full template. It can be implemented in parallel with all other Phase 2 commands.

---

## Template Location

`internal/provision/templates/user-data.yaml.tmpl`

The existing template from Phase 0 is a minimal stub. This PRD replaces it with the full provisioning spec.

---

## Template Parameters

```go
type Params struct {
    Username         string // default: "ubuntu"
    Hostname         string // droplet name, e.g. "devenv-20260304-143012"
    TailscaleAuthKey string // from config
    NotifyConfig     string // rendered [notify] section of config.toml (for devenv notify send)
    ProjectsDir      string // from config, default: "~/projects"
    DevenvVersion    string // version tag for downloading the devenv binary
}
```

---

## What Gets Installed

### System packages
- `curl`, `git`, `jq`, `unzip`, `build-essential`
- `tmux` (>= 3.2 for popup support)
- `mosh`

### Docker
- Docker Engine (stable channel) via official apt repo
- `ubuntu` user added to `docker` group

### mise
- Installed globally to `/usr/local/bin/mise`
- Available to all users

### Tailscale
- Installed via official install script
- Authenticated with `{{ .TailscaleAuthKey }}`
- `tailscale up --authkey={{ .TailscaleAuthKey }} --ssh`

### Node.js + Claude Code
- Node.js installed via mise (`mise use -g node@lts`)
- Claude Code installed globally (`npm install -g @anthropic-ai/claude-code`)

### devenv binary
- Downloaded from GitHub releases (or built from source if no release exists)
- Installed to `/usr/local/bin/devenv`
- This is the same Go binary used locally — workload commands (`session`, `worktree`, `project`, `notify send`) run directly on the droplet

### devenv config on droplet
- `~/.config/devenv/config.toml` written with:
  - `[notify]` section (provider credentials for `devenv notify send`)
  - `[defaults]` section with `projects_dir`
  - `[projects]` section (project definitions for `devenv project clone`)
- This enables workload commands to work on the droplet without additional setup

---

## tmux Configuration (tmux-native session management)

### `~/.tmux.conf` for the `ubuntu` user

```tmux
# Prefix: Ctrl+B (default, no change)

# Status bar — shows all sessions and their devenv state
set -g status-interval 5
set -g status-left "[devenv] "
set -g status-left-length 20
set -g status-right " #(devenv-tmux-status) | %H:%M"
set -g status-right-length 80

# Session navigation keybindings
bind T switch-client -t devenv        # Ctrl+B T — jump to devenv management session
bind N command-prompt -p "project:,branch:" \
  "run-shell 'devenv session start %1 %2'"
bind S display-popup -E -w 80 -h 20 "devenv session list"
bind W display-popup -E -w 80 -h 20 "devenv worktree list"

# General settings
set -g mouse off                      # no mouse — avoids conflicts with mosh + mobile
set -g default-terminal "tmux-256color"
set -g escape-time 10                 # fast escape for mosh compatibility
set -g history-limit 50000
set -g focus-events on

# Window/pane settings
set -g base-index 1
setw -g pane-base-index 1
set -g renumber-windows on
```

### `devenv-tmux-status` script

A shell script at `/usr/local/bin/devenv-tmux-status` called by the tmux status bar:

```bash
#!/bin/bash
# Reads session state files and formats them for the tmux status bar
sessions_dir="${HOME}/.local/share/devenv/sessions"
output=""
if [ -d "$sessions_dir" ]; then
  for f in "$sessions_dir"/*.json; do
    [ -f "$f" ] || continue
    name=$(jq -r '.session' "$f")
    status=$(jq -r '.status' "$f")
    if [ "$status" = "waiting" ]; then
      output="${output} ${name}:WAITING"
    else
      output="${output} ${name}:${status}"
    fi
  done
fi
echo "${output:-no sessions}"
```

### Shell auto-attach

`~/.zshrc` (or `~/.bashrc`) for the `ubuntu` user:

```bash
# Auto-attach to devenv management tmux session on login
if [[ -z "$TMUX" ]]; then
    tmux attach -t devenv 2>/dev/null || tmux new -s devenv
fi
```

This means SSHing in always lands in the `devenv` tmux session — the management session in Layout A.

---

## Claude Code Hook Configuration

### `~/.claude/settings.json` for the `ubuntu` user

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

---

## Directory Structure Created

```
/home/ubuntu/
├── .config/devenv/config.toml     <- notify + projects + projects_dir config
├── .local/share/devenv/sessions/  <- session state files (created by devenv session start)
├── .claude/settings.json          <- Claude Code hooks
├── .tmux.conf                     <- tmux-native config
├── .zshrc                         <- auto-attach to devenv tmux session
└── projects/                      <- default projects_dir (or as configured)
    ├── github.com/
    │   └── <user>/
    │       ├── <project>/              <- cloned by devenv project clone
    │       └── <project>__worktrees/   <- worktrees (created by devenv worktree new)
    └── gitlab.com/
        └── ...
```

Clone paths mirror the repo URL structure (see `prd-phase-02-cmd-project.md`).

---

## Hostname

Set to the droplet name (e.g. `devenv-20260304-143012`) for easy identification in tmux, shell prompts, and Tailscale admin.

---

## Implementation Notes

- The template must be valid YAML after rendering — test with various parameter combinations
- All file writes use `write_files` cloud-init module (not `runcmd` with heredocs)
- The devenv binary download URL should be configurable (for private repos or local builds)
- Template renders notification credentials into the droplet's config.toml — see security note in `prd-phase-02-cmd-notify.md`
- The `devenv-tmux-status` script is intentionally simple (shell + jq) — it runs every 5 seconds in the status bar and must be fast
- `escape-time 10` is important for mosh — the default 500ms makes Escape key feel sluggish on mobile
