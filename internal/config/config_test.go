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

func TestLoad_Projects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[projects.myapp]
repo = "git@github.com:user/myapp.git"
default_branch = "main"

[projects.api]
repo = "git@github.com:user/api.git"
default_branch = "develop"
env_template = "~/.config/devenv/templates/api.env.template"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Projects) != 2 {
		t.Fatalf("len(Projects) = %d, want 2", len(cfg.Projects))
	}
	myapp := cfg.Projects["myapp"]
	if myapp.Repo != "git@github.com:user/myapp.git" {
		t.Errorf("myapp.Repo = %q, want expected", myapp.Repo)
	}
	if myapp.DefaultBranch != "main" {
		t.Errorf("myapp.DefaultBranch = %q, want %q", myapp.DefaultBranch, "main")
	}
	api := cfg.Projects["api"]
	if api.DefaultBranch != "develop" {
		t.Errorf("api.DefaultBranch = %q, want %q", api.DefaultBranch, "develop")
	}
	// EnvTemplate should have ~ expanded
	home, _ := os.UserHomeDir()
	wantTpl := home + "/.config/devenv/templates/api.env.template"
	if api.EnvTemplate != wantTpl {
		t.Errorf("api.EnvTemplate = %q, want %q", api.EnvTemplate, wantTpl)
	}
}

func TestLoad_ProjectsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Projects != nil {
		t.Errorf("Projects = %v, want nil for missing file", cfg.Projects)
	}
}

func TestLoad_ProjectsDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[defaults]
projects_dir = "~/projects"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	home, _ := os.UserHomeDir()
	want := home + "/projects"
	if cfg.Defaults.ProjectsDir != want {
		t.Errorf("ProjectsDir = %q, want %q", cfg.Defaults.ProjectsDir, want)
	}
}

func TestLoad_ProjectsDirDefault(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	home, _ := os.UserHomeDir()
	want := home + "/projects"
	if cfg.Defaults.ProjectsDir != want {
		t.Errorf("ProjectsDir = %q, want %q (default)", cfg.Defaults.ProjectsDir, want)
	}
}

// TestLoad_ReadError exercises the non-ErrNotExist read error branch by
// making the config path a directory.
func TestLoad_ReadError(t *testing.T) {
	dir := t.TempDir()
	// Make a directory where the config file would be.
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(cfgPath, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("Load() on directory = nil, want error")
	}
}

// TestLoad_DefaultImage exercises the branch where a loaded config has an
// empty image (so the default must be applied after parsing).
func TestLoad_DefaultImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	// Write a valid config with no image field.
	const content = `
[defaults]
token = "tok"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Defaults.Image != "ubuntu-24-04-x64" {
		t.Errorf("Image = %q, want default %q", cfg.Defaults.Image, "ubuntu-24-04-x64")
	}
}

// TestLoad_NoHostURL exercises the RepoPath "no host" branch via a URL-style
// string that parses but has no host.
func TestLoad_ProjectsWithAbsEnvTemplate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[projects.api]
repo = "git@github.com:user/api.git"
env_template = "/absolute/path/api.env.template"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	api := cfg.Projects["api"]
	// Absolute paths must not be altered by expandTilde.
	if api.EnvTemplate != "/absolute/path/api.env.template" {
		t.Errorf("EnvTemplate = %q, want unchanged absolute path", api.EnvTemplate)
	}
}

func TestLoad_GitIdentityFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[defaults]
git_identity_file = "~/.ssh/id_ed25519"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	home, _ := os.UserHomeDir()
	want := home + "/.ssh/id_ed25519"
	if cfg.Defaults.GitIdentityFile != want {
		t.Errorf("GitIdentityFile = %q, want %q", cfg.Defaults.GitIdentityFile, want)
	}
}
