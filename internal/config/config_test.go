package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xico42/devenv/internal/config"
)

func TestLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if cfg.Defaults.Image != "ubuntu-24-04-x64" {
		t.Errorf("Defaults.Image = %q, want %q", cfg.Defaults.Image, "ubuntu-24-04-x64")
	}
}

func TestLoad_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	const content = `
[defaults]
token = "mytoken"
region = "nyc3"
size = "s-2vcpu-4gb"

[profiles.large]
size = "s-4vcpu-8gb"
region = "sfo3"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Defaults.Token != "mytoken" {
		t.Errorf("Token = %q, want %q", cfg.Defaults.Token, "mytoken")
	}
	if cfg.Defaults.Region != "nyc3" {
		t.Errorf("Region = %q, want %q", cfg.Defaults.Region, "nyc3")
	}
}

func TestLoad_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("{{invalid"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load() error = nil, want error for corrupt file")
	}
}

func TestConfig_Profile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	const content = `
[profiles.large]
size = "s-4vcpu-8gb"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tests := []struct {
		name    string
		profile string
		wantErr bool
	}{
		{"existing", "large", false},
		{"missing", "nonexistent", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := cfg.Profile(tt.profile)
			if (err != nil) != tt.wantErr {
				t.Errorf("Profile(%q) error = %v, wantErr %v", tt.profile, err, tt.wantErr)
			}
		})
	}
}

func TestConfig_Save(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.toml") // Save must create subdirectory

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.Defaults.Token = "saved-token"
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	cfg2, err := config.Load(path)
	if err != nil {
		t.Fatalf("second Load() error = %v", err)
	}
	if cfg2.Defaults.Token != "saved-token" {
		t.Errorf("Token = %q, want %q", cfg2.Defaults.Token, "saved-token")
	}
}

func TestConfig_ApplyEnv(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	t.Setenv("DIGITALOCEAN_TOKEN", "env-token")
	t.Setenv("DEVENV_REGION", "ams3")
	t.Setenv("DEVENV_SIZE", "s-4vcpu-8gb")
	t.Setenv("DEVENV_IMAGE", "ubuntu-22-04-x64")
	t.Setenv("TAILSCALE_AUTH_KEY", "tskey-env")
	cfg.ApplyEnv()
	if cfg.Defaults.Token != "env-token" {
		t.Errorf("Token = %q, want env-token", cfg.Defaults.Token)
	}
	if cfg.Defaults.Region != "ams3" {
		t.Errorf("Region = %q, want ams3", cfg.Defaults.Region)
	}
	if cfg.Defaults.Size != "s-4vcpu-8gb" {
		t.Errorf("Size = %q, want s-4vcpu-8gb", cfg.Defaults.Size)
	}
	if cfg.Defaults.Image != "ubuntu-22-04-x64" {
		t.Errorf("Image = %q, want ubuntu-22-04-x64", cfg.Defaults.Image)
	}
	if cfg.Defaults.TailscaleAuthKey != "tskey-env" {
		t.Errorf("TailscaleAuthKey = %q, want tskey-env", cfg.Defaults.TailscaleAuthKey)
	}
}

func TestConfig_ApplyFlags(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	cfg.ApplyFlags("flag-token")
	if cfg.Defaults.Token != "flag-token" {
		t.Errorf("Token = %q, want flag-token", cfg.Defaults.Token)
	}
}
