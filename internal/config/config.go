package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const defaultImage = "ubuntu-24-04-x64"

// Config holds all devenv configuration.
type Config struct {
	Defaults DefaultsConfig           `toml:"defaults"`
	Profiles map[string]ProfileConfig `toml:"profiles"`

	path string // runtime only, not serialized
}

// DefaultsConfig holds default values applied to every droplet.
type DefaultsConfig struct {
	Token            string `toml:"token"`
	SSHKeyID         string `toml:"ssh_key_id"`
	Region           string `toml:"region"`
	Size             string `toml:"size"`
	TailscaleAuthKey string `toml:"tailscale_auth_key"`
	Image            string `toml:"image"`
}

// ProfileConfig holds per-profile overrides.
type ProfileConfig struct {
	Size   string `toml:"size"`
	Region string `toml:"region"`
	Image  string `toml:"image"`
}

// Load reads config from path. If path is empty, uses ~/.config/devenv/config.toml.
// A missing file returns an empty Config with defaults and nil error.
func Load(path string) (*Config, error) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home dir: %w", err)
		}
		path = filepath.Join(home, ".config", "devenv", "config.toml")
	}

	cfg := &Config{path: path}
	cfg.Defaults.Image = defaultImage

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	if cfg.Defaults.Image == "" {
		cfg.Defaults.Image = defaultImage
	}
	return cfg, nil
}

// Profile returns the named profile or an error if not found.
func (c *Config) Profile(name string) (ProfileConfig, error) {
	p, ok := c.Profiles[name]
	if !ok {
		return ProfileConfig{}, fmt.Errorf("profile %q not found", name)
	}
	return p, nil
}

// Save writes the config back to its file, creating directories as needed.
func (c *Config) Save() (err error) {
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	f, err := os.Create(c.path)
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("closing config file: %w", cerr)
		}
	}()
	if err := toml.NewEncoder(f).Encode(c); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return nil
}

// ApplyEnv overlays DIGITALOCEAN_TOKEN, DEVENV_REGION, DEVENV_SIZE, DEVENV_IMAGE,
// and TAILSCALE_AUTH_KEY environment variables onto the config.
func (c *Config) ApplyEnv() {
	if v := os.Getenv("DIGITALOCEAN_TOKEN"); v != "" {
		c.Defaults.Token = v
	}
	if v := os.Getenv("DEVENV_REGION"); v != "" {
		c.Defaults.Region = v
	}
	if v := os.Getenv("DEVENV_SIZE"); v != "" {
		c.Defaults.Size = v
	}
	if v := os.Getenv("DEVENV_IMAGE"); v != "" {
		c.Defaults.Image = v
	}
	if v := os.Getenv("TAILSCALE_AUTH_KEY"); v != "" {
		c.Defaults.TailscaleAuthKey = v
	}
}

// ApplyFlags overlays CLI flag values; empty string means "not set".
func (c *Config) ApplyFlags(token string) {
	if token != "" {
		c.Defaults.Token = token
	}
}
