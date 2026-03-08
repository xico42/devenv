package config

// AgentConfig holds agent harness settings (command, args, env vars).
type AgentConfig struct {
	Cmd  string            `toml:"cmd"`
	Args []string          `toml:"args"`
	Env  map[string]string `toml:"env"`
}

// ResolveAgent returns the merged agent config for the given project.
// Per-project cmd and args replace global if set. Env is merged with
// project winning on key conflict. If project is empty or not found,
// returns the global defaults.
func (c *Config) ResolveAgent(project string) AgentConfig {
	global := c.Defaults.Agent
	result := AgentConfig{
		Cmd:  global.Cmd,
		Args: global.Args,
		Env:  copyEnv(global.Env),
	}

	p, ok := c.Projects[project]
	if !ok {
		return result
	}

	if p.Agent.Cmd != "" {
		result.Cmd = p.Agent.Cmd
	}
	if p.Agent.Args != nil {
		result.Args = p.Agent.Args
	}
	for k, v := range p.Agent.Env {
		if result.Env == nil {
			result.Env = make(map[string]string)
		}
		result.Env[k] = v
	}
	return result
}

func copyEnv(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}
