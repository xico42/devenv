package cmd

import (
	"testing"

	"github.com/xico42/devenv/internal/config"
)

// setTestConfig sets the package-level cfg for the duration of the test.
func setTestConfig(t *testing.T, c *config.Config) {
	t.Helper()
	orig := cfg
	cfg = c
	t.Cleanup(func() { cfg = orig })
}

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
