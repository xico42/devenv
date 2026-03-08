# Named Agents Configuration — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace inline agent config with named agent definitions under `[agents.<name>]`, selected at runtime via CLI flag or TUI picker.

**Architecture:** Config gains a top-level `Agents map[string]AgentConfig` and `Defaults.Agent` becomes a string referencing a key in that map. CLI resolves agent name from `--agent` flag or default. TUI shows a compact picker before starting a session.

**Tech Stack:** Go, Cobra (CLI), Bubble Tea + lipgloss (TUI), TOML config

---

### Task 1: Remove `Agent AgentConfig` from `DefaultsConfig` and `ProjectConfig`

**Files:**
- Modify: `internal/config/config.go:32-42` (DefaultsConfig struct)
- Modify: `internal/config/project.go:10-15` (ProjectConfig struct)
- Modify: `internal/config/agent.go` (remove `ResolveAgent`, add new methods)
- Modify: `internal/config/agent_test.go` (rewrite tests)

**Step 1: Write the failing tests**

Replace `internal/config/agent_test.go` entirely:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xico42/devenv/internal/config"
)

func TestAgentByName_found(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[agents.claude]
cmd = "claude"
args = ["--dangerously-skip-permissions"]

[agents.claude.env]
CLAUDE_CONFIG_DIR = "/custom"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := cfg.AgentByName("claude")
	if err != nil {
		t.Fatalf("AgentByName(claude) error: %v", err)
	}
	if agent.Cmd != "claude" {
		t.Errorf("Cmd = %q, want claude", agent.Cmd)
	}
	if len(agent.Args) != 1 || agent.Args[0] != "--dangerously-skip-permissions" {
		t.Errorf("Args = %v, want [--dangerously-skip-permissions]", agent.Args)
	}
	if agent.Env["CLAUDE_CONFIG_DIR"] != "/custom" {
		t.Errorf("Env[CLAUDE_CONFIG_DIR] = %q, want /custom", agent.Env["CLAUDE_CONFIG_DIR"])
	}
}

func TestAgentByName_notFound(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[agents.claude]
cmd = "claude"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cfg.AgentByName("nonexistent")
	if err == nil {
		t.Error("AgentByName(nonexistent) should return error")
	}
}

func TestAgentByName_noAgentsDefined(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = cfg.AgentByName("claude")
	if err == nil {
		t.Error("AgentByName on empty config should return error")
	}
}

func TestAgentNames_sorted(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[agents.zed]
cmd = "zed"

[agents.aider]
cmd = "aider"

[agents.claude]
cmd = "claude"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	names := cfg.AgentNames()
	if len(names) != 3 {
		t.Fatalf("AgentNames() = %v, want 3 entries", names)
	}
	if names[0] != "aider" || names[1] != "claude" || names[2] != "zed" {
		t.Errorf("AgentNames() = %v, want [aider claude zed]", names)
	}
}

func TestAgentNames_empty(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	names := cfg.AgentNames()
	if len(names) != 0 {
		t.Errorf("AgentNames() = %v, want empty", names)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/... -run "TestAgent" -v`
Expected: FAIL — `AgentByName` and `AgentNames` methods don't exist yet.

**Step 3: Implement the changes**

In `internal/config/config.go`, change `DefaultsConfig`:

```go
type DefaultsConfig struct {
	Token            string `toml:"token"             validate:"omitempty" secret:"true"`
	SSHKeyID         string `toml:"ssh_key_id"        validate:"omitempty"`
	Region           string `toml:"region"            validate:"omitempty"`
	Size             string `toml:"size"              validate:"omitempty"`
	TailscaleAuthKey string `toml:"tailscale_auth_key" validate:"omitempty" secret:"true"`
	Image            string `toml:"image"             validate:"omitempty"`
	ProjectsDir      string `toml:"projects_dir"      validate:"omitempty"`
	GitIdentityFile  string `toml:"git_identity_file" validate:"omitempty"`
	Agent            string `toml:"agent"             validate:"omitempty"`
}
```

In `internal/config/config.go`, add `Agents` field to `Config`:

```go
type Config struct {
	Defaults DefaultsConfig           `toml:"defaults"`
	Profiles map[string]ProfileConfig `toml:"profiles"`
	Projects map[string]ProjectConfig `toml:"projects"`
	Agents   map[string]AgentConfig   `toml:"agents"`
	Notify   NotifyConfig             `toml:"notify"`

	path string // runtime only, not serialized
}
```

In `internal/config/project.go`, remove `Agent AgentConfig` field:

```go
type ProjectConfig struct {
	Repo          string `toml:"repo"           validate:"omitempty"`
	DefaultBranch string `toml:"default_branch" validate:"omitempty"`
	EnvTemplate   string `toml:"env_template"   validate:"omitempty"`
}
```

In `internal/config/agent.go`, replace `ResolveAgent` and `copyEnv` with:

```go
package config

import (
	"fmt"
	"sort"
)

// AgentConfig holds agent harness settings (command, args, env vars).
type AgentConfig struct {
	Cmd  string            `toml:"cmd"`
	Args []string          `toml:"args"`
	Env  map[string]string `toml:"env"`
}

// AgentByName returns the agent config for the given name.
// Returns an error if the name is not found.
func (c *Config) AgentByName(name string) (AgentConfig, error) {
	if c.Agents == nil {
		return AgentConfig{}, fmt.Errorf("agent %q not found (no agents configured)", name)
	}
	agent, ok := c.Agents[name]
	if !ok {
		return AgentConfig{}, fmt.Errorf("agent %q not found", name)
	}
	return agent, nil
}

// AgentNames returns a sorted list of defined agent names.
func (c *Config) AgentNames() []string {
	if len(c.Agents) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.Agents))
	for name := range c.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/... -run "TestAgent" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/config/agent.go internal/config/agent_test.go internal/config/config.go internal/config/project.go
git commit -m "refactor: replace inline agent config with named agents map"
```

---

### Task 2: Remove `DefaultAgentCmd` from semconv

**Files:**
- Modify: `internal/semconv/semconv.go:10` (remove const)
- Modify: `internal/semconv/semconv_test.go` (remove any test referencing it)

**Step 1: Check for remaining references and remove them**

After Task 1 removed `ResolveAgent`, there should be no references to `DefaultAgentCmd` left in non-test code (they'll be updated in Tasks 3 and 4). Remove the constant from `internal/semconv/semconv.go`:

```go
// Remove this line:
DefaultAgentCmd = "claude"
```

**Step 2: Run tests**

Run: `go test ./internal/semconv/... -v`
Expected: PASS (or compile errors if tests reference it — fix those too)

**Step 3: Commit**

```bash
git add internal/semconv/semconv.go internal/semconv/semconv_test.go
git commit -m "refactor: remove DefaultAgentCmd constant"
```

---

### Task 3: Update CLI `session start` to use `--agent` flag

**Files:**
- Modify: `cmd/session.go:26-45,57-108,269-271` (replace resolve helpers, add flag, update RunE)
- Modify: `cmd/session_internal_test.go` (rewrite for named agent resolution)
- Modify: `cmd/session_test.go:11-29` (update config helper)

**Step 1: Write the failing tests**

Replace `cmd/session_internal_test.go`:

```go
package cmd

import (
	"testing"

	"github.com/xico42/devenv/internal/config"
)

func TestResolveAgentName_flagTakesPrecedence(t *testing.T) {
	setTestConfig(t, &config.Config{
		Defaults: config.DefaultsConfig{Agent: "default-agent"},
		Agents: map[string]config.AgentConfig{
			"default-agent": {Cmd: "default"},
			"flag-agent":    {Cmd: "flag"},
		},
	})
	name, err := resolveAgentName("flag-agent")
	if err != nil {
		t.Fatal(err)
	}
	if name != "flag-agent" {
		t.Errorf("resolveAgentName = %q, want flag-agent", name)
	}
}

func TestResolveAgentName_fallsBackToDefault(t *testing.T) {
	setTestConfig(t, &config.Config{
		Defaults: config.DefaultsConfig{Agent: "my-default"},
		Agents: map[string]config.AgentConfig{
			"my-default": {Cmd: "claude"},
		},
	})
	name, err := resolveAgentName("")
	if err != nil {
		t.Fatal(err)
	}
	if name != "my-default" {
		t.Errorf("resolveAgentName = %q, want my-default", name)
	}
}

func TestResolveAgentName_errorWhenNoneSet(t *testing.T) {
	setTestConfig(t, &config.Config{})
	_, err := resolveAgentName("")
	if err == nil {
		t.Error("resolveAgentName should error when no agent specified and no default")
	}
}

func TestBuildAgentCmd_cmdOnly(t *testing.T) {
	agent := config.AgentConfig{Cmd: "myagent"}
	got := buildAgentCmd(agent)
	if got != "myagent" {
		t.Errorf("buildAgentCmd = %q, want myagent", got)
	}
}

func TestBuildAgentCmd_cmdWithArgs(t *testing.T) {
	agent := config.AgentConfig{Cmd: "echo", Args: []string{"hello", "world"}}
	got := buildAgentCmd(agent)
	if got != "echo hello world" {
		t.Errorf("buildAgentCmd = %q, want %q", got, "echo hello world")
	}
}
```

Update `cmd/session_test.go` — change `writeSessionConfig` to use new format:

```go
func writeSessionConfig(t *testing.T, projectsDir string) string {
	t.Helper()
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	content := `[defaults]
projects_dir = "` + projectsDir + `"
agent = "echo-agent"

[agents.echo-agent]
cmd = "echo"
args = ["hello"]

[projects.myapp]
repo = "git@github.com:user/myapp.git"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}
```

Add test for `--agent` flag recognition in `cmd/session_test.go`:

```go
func TestSessionStart_agentFlag_recognized(t *testing.T) {
	cfgPath := writeSessionConfig(t, t.TempDir())
	err := runCmd(t, "--config", cfgPath, "session", "start", "--agent", "echo-agent", "notaproject", "main")
	if err == nil {
		t.Fatal("expected error for unconfigured project, got nil")
	}
	if strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("--agent flag not recognised: %v", err)
	}
}

func TestSessionStart_noAgentConfigured_errors(t *testing.T) {
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	content := `[defaults]
projects_dir = "` + t.TempDir() + `"

[projects.myapp]
repo = "git@github.com:user/myapp.git"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runCmd(t, "--config", cfgPath, "session", "start", "myapp", "main")
	if err == nil {
		t.Fatal("expected error when no agent configured")
	}
	if !strings.Contains(err.Error(), "no agent specified") {
		t.Errorf("error = %q, want to contain 'no agent specified'", err.Error())
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./cmd/... -run "TestResolveAgent|TestBuildAgent|TestSessionStart_agent|TestSessionStart_noAgent" -v`
Expected: FAIL — `resolveAgentName` and `buildAgentCmd` don't exist yet.

**Step 3: Implement the changes**

In `cmd/session.go`, replace `resolveAgentCmd` and `resolveAgentEnv` (lines 26-45) with:

```go
// resolveAgentName returns the agent name from the flag or config default.
func resolveAgentName(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	if cfg.Defaults.Agent != "" {
		return cfg.Defaults.Agent, nil
	}
	return "", fmt.Errorf("no agent specified; use --agent or set defaults.agent in config")
}

// buildAgentCmd builds the full command string from an AgentConfig.
func buildAgentCmd(agent config.AgentConfig) string {
	cmd := agent.Cmd
	if len(agent.Args) > 0 {
		cmd = cmd + " " + strings.Join(agent.Args, " ")
	}
	return cmd
}
```

Add flag variable at the top (near other flag vars):

```go
var sessionStartAgent string
```

In `init()`, register the flag:

```go
sessionStartCmd.Flags().StringVar(&sessionStartAgent, "agent", "", "agent to use for the session")
```

Update `sessionStartCmd.RunE` to use the new resolution:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	project, branch := args[0], args[1]

	agentName, err := resolveAgentName(sessionStartAgent)
	if err != nil {
		return err
	}
	agent, err := cfg.AgentByName(agentName)
	if err != nil {
		return err
	}

	wtSvc := newWorktreeService()
	path, err := wtSvc.WorktreePath(project, branch)
	if err != nil {
		if errors.Is(err, worktree.ErrWorktreeNotFound) && !sessionStartNoCreate {
			fmt.Fprintf(cmd.OutOrStdout(), "Worktree %s/%s not found, creating...  ", project, branch)
			result, createErr := wtSvc.New(project, branch)
			if createErr != nil {
				fmt.Fprintln(cmd.OutOrStdout())
				return worktreeErr(cmd, project, branch, createErr)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "done")
			path = result.Path
		} else {
			return sessionErr(cmd, err)
		}
	}

	name := semconv.SessionName(project, branch)
	fmt.Fprintf(cmd.OutOrStdout(), "Starting session %s...  ", name)

	svc := newSessionService()
	err = svc.Start(session.StartRequest{
		Project: project,
		Branch:  branch,
		Path:    path,
		Cmd:     buildAgentCmd(agent),
		Env:     agent.Env,
		Attach:  sessionStartAttach,
	})
	if err != nil {
		fmt.Fprintln(cmd.OutOrStdout())
		return sessionErr(cmd, err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "done")
	if !sessionStartAttach {
		fmt.Fprintf(cmd.OutOrStdout(), "Attach with: devenv session attach %s\n", name)
	}

	if sessionStartAttach {
		return execTmuxAttach(name)
	}
	return nil
},
```

Remove the `"github.com/xico42/devenv/internal/semconv"` import from `cmd/session.go` (no longer needed after removing `DefaultAgentCmd` usage). Add `"github.com/xico42/devenv/internal/config"` import instead.

**Step 4: Run tests to verify they pass**

Run: `go test ./cmd/... -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/session.go cmd/session_internal_test.go cmd/session_test.go
git commit -m "feat: add --agent flag to session start command"
```

---

### Task 4: Update TUI to use agent picker

**Files:**
- Modify: `internal/tui/model.go:23-27,44-71` (add screenAgentPicker const, add agentPicker field)
- Modify: `internal/tui/actions.go` (replace resolve helpers, add picker flow)
- Modify: `internal/tui/actions_test.go` (rewrite agent resolution tests)
- Create: `internal/tui/agent_picker.go` (compact picker sub-model)

**Step 1: Write the failing tests**

Update the relevant tests in `internal/tui/actions_test.go`. Remove `TestResolveAgentCommand_*` and `TestResolveAgentEnv_*` tests. Add tests for the new picker behavior:

```go
func TestAttachAction_worktreeGroup_noAgents_returnsError(t *testing.T) {
	cfg := &config.Config{} // no agents defined
	items := []Item{{Project: "myapp", Branch: "feat", Path: "/some/path", Group: groupWorktree}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList, cfg: cfg}
	m.list = newList(listItems)

	updated, _ := m.attachAction()
	um := updated.(Model)
	if um.statusMsg == "" {
		t.Error("attachAction with no agents should set statusMsg")
	}
}

func TestAttachAction_worktreeGroup_singleAgent_skipsPickerAndReturnCmd(t *testing.T) {
	cfg := &config.Config{
		Agents: map[string]config.AgentConfig{
			"claude": {Cmd: "claude"},
		},
	}
	items := []Item{{Project: "myapp", Branch: "feat", Path: "/some/path", Group: groupWorktree}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList, cfg: cfg}
	m.list = newList(listItems)

	updated, cmd := m.attachAction()
	um := updated.(Model)
	// Single agent — should skip picker and go straight to starting.
	if um.screen == screenAgentPicker {
		t.Error("single agent should skip picker screen")
	}
	if cmd == nil {
		t.Fatal("attachAction with single agent should return non-nil cmd")
	}
}

func TestAttachAction_worktreeGroup_multipleAgents_showsPicker(t *testing.T) {
	cfg := &config.Config{
		Agents: map[string]config.AgentConfig{
			"claude": {Cmd: "claude"},
			"aider":  {Cmd: "aider"},
		},
	}
	items := []Item{{Project: "myapp", Branch: "feat", Path: "/some/path", Group: groupWorktree}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList, cfg: cfg}
	m.list = newList(listItems)

	updated, _ := m.attachAction()
	um := updated.(Model)
	if um.screen != screenAgentPicker {
		t.Errorf("screen = %d, want %d (screenAgentPicker)", um.screen, screenAgentPicker)
	}
	if um.agentPicker == nil {
		t.Fatal("agentPicker should be set")
	}
}

func TestAttachAction_worktreeGroup_multipleAgents_defaultPreselected(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.DefaultsConfig{Agent: "aider"},
		Agents: map[string]config.AgentConfig{
			"claude": {Cmd: "claude"},
			"aider":  {Cmd: "aider"},
		},
	}
	items := []Item{{Project: "myapp", Branch: "feat", Path: "/some/path", Group: groupWorktree}}
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = it
	}
	m := Model{screen: screenList, cfg: cfg}
	m.list = newList(listItems)

	updated, _ := m.attachAction()
	um := updated.(Model)
	if um.agentPicker == nil {
		t.Fatal("agentPicker should be set")
	}
	if um.agentPicker.selected() != "aider" {
		t.Errorf("selected = %q, want aider (default)", um.agentPicker.selected())
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/... -run "TestAttachAction_worktree" -v`
Expected: FAIL — `screenAgentPicker`, `agentPicker` field don't exist.

**Step 3: Create agent picker sub-model**

Create `internal/tui/agent_picker.go`:

```go
package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/session"
)

// agentSelectedMsg is sent when the user picks an agent.
type agentSelectedMsg struct {
	agent config.AgentConfig
}

// agentPickerModel shows a compact list of named agents.
type agentPickerModel struct {
	names   []string
	cursor  int
	cfg     *config.Config
	pending *agentPickerPending // context for starting the session after selection
}

// agentPickerPending holds the context needed to start a session after agent selection.
type agentPickerPending struct {
	project string
	branch  string
	path    string
	sesSvc  *session.Service
}

func newAgentPicker(cfg *config.Config, defaultAgent string, pending *agentPickerPending) *agentPickerModel {
	names := cfg.AgentNames()
	cursor := 0
	for i, n := range names {
		if n == defaultAgent {
			cursor = i
			break
		}
	}
	return &agentPickerModel{
		names:   names,
		cursor:  cursor,
		cfg:     cfg,
		pending: pending,
	}
}

func (p *agentPickerModel) selected() string {
	if len(p.names) == 0 {
		return ""
	}
	return p.names[p.cursor]
}

func (p *agentPickerModel) Update(msg tea.Msg) (*agentPickerModel, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	switch kp.String() {
	case "j", "down":
		if p.cursor < len(p.names)-1 {
			p.cursor++
		}
	case "k", "up":
		if p.cursor > 0 {
			p.cursor--
		}
	case "enter":
		return p, p.submit()
	}
	return p, nil
}

func (p *agentPickerModel) submit() tea.Cmd {
	name := p.selected()
	agent, err := p.cfg.AgentByName(name)
	if err != nil {
		return func() tea.Msg { return errMsg{err: err} }
	}

	pending := p.pending
	cmd := agent.Cmd
	if len(agent.Args) > 0 {
		cmd = cmd + " " + strings.Join(agent.Args, " ")
	}

	return func() tea.Msg {
		err := pending.sesSvc.Start(session.StartRequest{
			Project: pending.project,
			Branch:  pending.branch,
			Path:    pending.path,
			Cmd:     cmd,
			Env:     agent.Env,
		})
		if err != nil {
			return errMsg{err: err}
		}
		return attachMsg{session: pending.project + "-" + pending.branch}
	}
}

func (p *agentPickerModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true)

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Select Agent"))
	sb.WriteString("\n────\n")
	for i, name := range p.names {
		cursor := "  "
		if i == p.cursor {
			cursor = "> "
		}
		style := lipgloss.NewStyle()
		if i == p.cursor {
			style = style.Bold(true)
		} else {
			style = style.Faint(true)
		}
		sb.WriteString(fmt.Sprintf("%s%s\n", cursor, style.Render(name)))
	}
	sb.WriteString("────\nEnter: select  |  Esc: cancel  |  j/k: navigate")
	return sb.String()
}
```

**Step 4: Update `model.go`**

Add `screenAgentPicker` const:

```go
const (
	screenList = iota
	screenForm
	screenConfirmDelete
	screenAgentPicker
)
```

Add `agentPicker` field to `Model`:

```go
// Agent picker sub-model.
agentPicker *agentPickerModel
```

Add routing in `Update()` switch for `screenAgentPicker`:

```go
case screenAgentPicker:
	return m.updateAgentPicker(msg)
```

Add rendering in `View()` switch:

```go
case screenAgentPicker:
	if m.agentPicker != nil {
		content = m.agentPicker.View()
	}
```

**Step 5: Update `actions.go`**

Remove `resolveAgentCommand` and `resolveAgentEnv` functions (lines 16-35).

Change `attachAction` for `groupWorktree` and `groupProject` cases. For both cases, instead of directly starting the session:

1. If no agents defined in config, set `statusMsg` and return.
2. If exactly one agent, skip picker and start session directly.
3. If multiple agents, show picker.

Replace the method entirely:

```go
func (m Model) attachAction() (tea.Model, tea.Cmd) {
	sel := m.selectedItem()
	if sel == nil {
		return m, nil
	}

	sesSvc := m.sesSvc
	wtSvc := m.wtSvc
	projSvc := m.projSvc
	cfg := m.cfg
	project := sel.Project
	branch := sel.Branch

	switch sel.Group {
	case groupAgent:
		// Already has agent — just attach.
		sessionName := semconv.SessionName(project, branch)
		return m, func() tea.Msg { return attachMsg{session: sessionName} }

	case groupWorktree:
		agents := cfg.AgentNames()
		if len(agents) == 0 {
			m.statusMsg = "no agents configured — add [agents.<name>] to config"
			return m, nil
		}

		path := sel.Path
		pending := &agentPickerPending{
			project: project,
			branch:  branch,
			path:    path,
			sesSvc:  sesSvc,
		}

		if len(agents) == 1 {
			// Single agent — skip picker.
			agent, _ := cfg.AgentByName(agents[0])
			agentCmd := agent.Cmd
			if len(agent.Args) > 0 {
				agentCmd = agentCmd + " " + strings.Join(agent.Args, " ")
			}
			return m, func() tea.Msg {
				sessionName := semconv.SessionName(project, branch)
				err := sesSvc.Start(session.StartRequest{
					Project: project,
					Branch:  branch,
					Path:    path,
					Cmd:     agentCmd,
					Env:     agent.Env,
				})
				if err != nil {
					return errMsg{err: err}
				}
				return attachMsg{session: sessionName}
			}
		}

		// Multiple agents — show picker.
		m.agentPicker = newAgentPicker(cfg, cfg.Defaults.Agent, pending)
		m.screen = screenAgentPicker
		return m, nil

	case groupProject:
		agents := cfg.AgentNames()
		if len(agents) == 0 {
			m.statusMsg = "no agents configured — add [agents.<name>] to config"
			return m, nil
		}

		defaultBranch := "main"
		if p, ok := cfg.Projects[project]; ok && p.DefaultBranch != "" {
			defaultBranch = p.DefaultBranch
		}

		startWithAgent := func(agent config.AgentConfig) tea.Cmd {
			agentCmd := agent.Cmd
			if len(agent.Args) > 0 {
				agentCmd = agentCmd + " " + strings.Join(agent.Args, " ")
			}
			return func() tea.Msg {
				if projSvc != nil {
					_ = projSvc.Clone(project)
				}
				result, err := wtSvc.New(project, defaultBranch)
				if err != nil {
					return errMsg{err: err}
				}
				sessionName := semconv.SessionName(project, defaultBranch)
				err = sesSvc.Start(session.StartRequest{
					Project: project,
					Branch:  defaultBranch,
					Path:    result.Path,
					Cmd:     agentCmd,
					Env:     agent.Env,
				})
				if err != nil {
					return errMsg{err: err}
				}
				return attachMsg{session: sessionName}
			}
		}

		if len(agents) == 1 {
			agent, _ := cfg.AgentByName(agents[0])
			return m, startWithAgent(agent)
		}

		// Multiple agents — show picker.
		// For groupProject, pending.path is empty; the picker's submit
		// must handle clone+worktree creation. We'll handle this by
		// storing the startWithAgent closure differently.
		// Actually, for groupProject we need a different pending flow.
		// Let's use a custom approach: store the context and handle in picker submit.
		m.agentPicker = newAgentPicker(cfg, cfg.Defaults.Agent, &agentPickerPending{
			project: project,
			branch:  defaultBranch,
			sesSvc:  sesSvc,
		})
		m.screen = screenAgentPicker
		return m, nil
	}
	return m, nil
}
```

Note: the `attachAction` signature changes from `tea.Cmd` to `(tea.Model, tea.Cmd)` to support updating model state (setting picker, statusMsg). Update the call site in `updateList`:

```go
case key.Matches(msg, m.keys.Attach):
	return m.attachAction()
```

Add `updateAgentPicker` method in `actions.go`:

```go
func (m Model) updateAgentPicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if keyMsg.String() == "esc" {
			m.screen = screenList
			m.agentPicker = nil
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.agentPicker, cmd = m.agentPicker.Update(msg)

	// If submit returned a cmd, clear picker state.
	if cmd != nil {
		m.screen = screenList
		m.agentPicker = nil
	}
	return m, cmd
}
```

Also update the `groupProject` case in `agentPickerModel.submit()` to handle clone+worktree. Since `pending.path` may be empty for `groupProject`, the picker's submit needs to use `wtSvc` and `projSvc`. Update `agentPickerPending` to include these:

```go
type agentPickerPending struct {
	project string
	branch  string
	path    string
	sesSvc  *session.Service
	wtSvc   *worktree.Service
	projSvc *project.Service
}
```

And update `submit()` to handle the worktree creation case when `path` is empty:

```go
func (p *agentPickerModel) submit() tea.Cmd {
	name := p.selected()
	agent, err := p.cfg.AgentByName(name)
	if err != nil {
		return func() tea.Msg { return errMsg{err: err} }
	}

	pending := p.pending
	cmd := agent.Cmd
	if len(agent.Args) > 0 {
		cmd = cmd + " " + strings.Join(agent.Args, " ")
	}

	return func() tea.Msg {
		path := pending.path

		// If no path, need to clone + create worktree.
		if path == "" {
			if pending.projSvc != nil {
				_ = pending.projSvc.Clone(pending.project)
			}
			if pending.wtSvc != nil {
				result, err := pending.wtSvc.New(pending.project, pending.branch)
				if err != nil {
					return errMsg{err: err}
				}
				path = result.Path
			}
		}

		err := pending.sesSvc.Start(session.StartRequest{
			Project: pending.project,
			Branch:  pending.branch,
			Path:    path,
			Cmd:     cmd,
			Env:     agent.Env,
		})
		if err != nil {
			return errMsg{err: err}
		}
		return attachMsg{session: semconv.SessionName(pending.project, pending.branch)}
	}
}
```

Update the `groupProject` picker creation to pass `wtSvc` and `projSvc`:

```go
m.agentPicker = newAgentPicker(cfg, cfg.Defaults.Agent, &agentPickerPending{
	project: project,
	branch:  defaultBranch,
	sesSvc:  sesSvc,
	wtSvc:   wtSvc,
	projSvc: projSvc,
})
```

**Step 6: Run tests to verify they pass**

Run: `go test ./internal/tui/... -v`
Expected: PASS

**Step 7: Commit**

```bash
git add internal/tui/agent_picker.go internal/tui/model.go internal/tui/actions.go internal/tui/actions_test.go
git commit -m "feat: add agent picker to TUI for session start"
```

---

### Task 5: Full verification

**Step 1: Run all tests**

Run: `make test`
Expected: PASS

**Step 2: Run linter**

Run: `make lint`
Expected: PASS (no lint errors)

**Step 3: Run coverage**

Run: `make coverage`
Expected: PASS (>= 80% aggregate coverage)

**Step 4: Build**

Run: `make build`
Expected: `./devenv` binary created

**Step 5: Commit any fixes if needed**

If any step above required fixes, commit them:

```bash
git add -A
git commit -m "fix: address lint/test issues from named agents implementation"
```

---

### Dependency Graph

```
Task 1 (config) ──→ Task 2 (semconv) ──→ Task 3 (CLI) ──→ Task 5 (verify)
                                     ──→ Task 4 (TUI) ──→ Task 5 (verify)
```

Tasks 3 and 4 can run in parallel after Task 2.
