package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xico42/devenv/internal/config"
)

func TestAgentConfig_Defaults(t *testing.T) {
	cfg, err := config.Load(filepath.Join(t.TempDir(), "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	agent := cfg.ResolveAgent("")
	if agent.Cmd != "" {
		t.Errorf("default Cmd = %q, want empty (caller applies default)", agent.Cmd)
	}
}

func TestAgentConfig_GlobalOnly(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[defaults.agent]
cmd = "claude"
args = ["--dangerously-skip-permissions"]

[defaults.agent.env]
CLAUDE_CONFIG_DIR = "/custom"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	agent := cfg.ResolveAgent("nonexistent")
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

func TestAgentConfig_ProjectOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[defaults.agent]
cmd = "claude"
args = ["--global-flag"]

[defaults.agent.env]
GLOBAL_VAR = "global"
SHARED_VAR = "from-global"

[projects.myapp]
repo = "git@github.com:user/myapp.git"

[projects.myapp.agent]
cmd = "aider"
args = ["--model", "opus"]

[projects.myapp.agent.env]
PROJECT_VAR = "project"
SHARED_VAR = "from-project"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	agent := cfg.ResolveAgent("myapp")

	// cmd and args: project replaces global
	if agent.Cmd != "aider" {
		t.Errorf("Cmd = %q, want aider", agent.Cmd)
	}
	if len(agent.Args) != 2 || agent.Args[0] != "--model" {
		t.Errorf("Args = %v, want [--model opus]", agent.Args)
	}

	// env: merged, project wins on conflict
	if agent.Env["GLOBAL_VAR"] != "global" {
		t.Errorf("Env[GLOBAL_VAR] = %q, want global", agent.Env["GLOBAL_VAR"])
	}
	if agent.Env["PROJECT_VAR"] != "project" {
		t.Errorf("Env[PROJECT_VAR] = %q, want project", agent.Env["PROJECT_VAR"])
	}
	if agent.Env["SHARED_VAR"] != "from-project" {
		t.Errorf("Env[SHARED_VAR] = %q, want from-project", agent.Env["SHARED_VAR"])
	}
}

func TestAgentConfig_ProjectPartialOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	content := `
[defaults.agent]
cmd = "claude"
args = ["--global-flag"]

[projects.myapp]
repo = "git@github.com:user/myapp.git"

[projects.myapp.agent.env]
EXTRA = "val"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	agent := cfg.ResolveAgent("myapp")

	// cmd and args: fall back to global (project didn't set them)
	if agent.Cmd != "claude" {
		t.Errorf("Cmd = %q, want claude (global fallback)", agent.Cmd)
	}
	if len(agent.Args) != 1 || agent.Args[0] != "--global-flag" {
		t.Errorf("Args = %v, want [--global-flag] (global fallback)", agent.Args)
	}
	if agent.Env["EXTRA"] != "val" {
		t.Errorf("Env[EXTRA] = %q, want val", agent.Env["EXTRA"])
	}
}
