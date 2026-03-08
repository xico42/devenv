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

// AgentByName returns the named agent config or an error if not found.
func (c *Config) AgentByName(name string) (AgentConfig, error) {
	a, ok := c.Agents[name]
	if !ok {
		return AgentConfig{}, fmt.Errorf("agent %q not found", name)
	}
	return a, nil
}

// AgentNames returns a sorted list of all configured agent names.
func (c *Config) AgentNames() []string {
	names := make([]string, 0, len(c.Agents))
	for name := range c.Agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
