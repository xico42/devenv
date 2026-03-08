package cmd

import (
	"strings"
	"testing"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/semconv"
)

// setTestConfig sets the package-level cfg for the duration of the test.
func setTestConfig(t *testing.T, c *config.Config) {
	t.Helper()
	orig := cfg
	cfg = c
	t.Cleanup(func() { cfg = orig })
}

func TestResolveAgentCmd_defaultWhenEmpty(t *testing.T) {
	setTestConfig(t, &config.Config{})
	cmd := resolveAgentCmd("anyproject")
	if cmd != semconv.DefaultAgentCmd {
		t.Errorf("resolveAgentCmd with empty config = %q, want %q", cmd, semconv.DefaultAgentCmd)
	}
}

func TestResolveAgentCmd_withCmdAndArgs(t *testing.T) {
	setTestConfig(t, &config.Config{
		Defaults: config.DefaultsConfig{
			Agent: config.AgentConfig{
				Cmd:  "echo",
				Args: []string{"hello", "world"},
			},
		},
	})
	got := resolveAgentCmd("anyproject")
	want := "echo hello world"
	if got != want {
		t.Errorf("resolveAgentCmd = %q, want %q", got, want)
	}
}

func TestResolveAgentCmd_cmdOnlyNoArgs(t *testing.T) {
	setTestConfig(t, &config.Config{
		Defaults: config.DefaultsConfig{
			Agent: config.AgentConfig{
				Cmd: "myagent",
			},
		},
	})
	got := resolveAgentCmd("anyproject")
	if got != "myagent" {
		t.Errorf("resolveAgentCmd = %q, want myagent", got)
	}
}

func TestResolveAgentEnv_empty(t *testing.T) {
	setTestConfig(t, &config.Config{})
	env := resolveAgentEnv("anyproject")
	if len(env) != 0 {
		t.Errorf("resolveAgentEnv with empty config = %v, want empty map", env)
	}
}

func TestResolveAgentEnv_withVars(t *testing.T) {
	setTestConfig(t, &config.Config{
		Defaults: config.DefaultsConfig{
			Agent: config.AgentConfig{
				Env: map[string]string{
					"FOO": "bar",
					"BAZ": "qux",
				},
			},
		},
	})
	env := resolveAgentEnv("anyproject")
	if env["FOO"] != "bar" {
		t.Errorf("env[FOO] = %q, want bar", env["FOO"])
	}
	if env["BAZ"] != "qux" {
		t.Errorf("env[BAZ] = %q, want qux", env["BAZ"])
	}
}

func TestResolveAgentCmd_noArgs_usesDefault(t *testing.T) {
	setTestConfig(t, &config.Config{
		Defaults: config.DefaultsConfig{
			Agent: config.AgentConfig{
				Cmd:  "",
				Args: []string{},
			},
		},
	})
	got := resolveAgentCmd("anyproject")
	if !strings.HasPrefix(got, semconv.DefaultAgentCmd) {
		t.Errorf("resolveAgentCmd = %q, want prefix %q", got, semconv.DefaultAgentCmd)
	}
}
