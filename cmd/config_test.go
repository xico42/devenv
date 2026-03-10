package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalconfig "github.com/xico42/devenv/internal/config"
)

// ── fieldFromStruct ───────────────────────────────────────────────────────────

func TestFieldFromStruct_NotFound(t *testing.T) {
	type sample struct {
		Name string `toml:"name"`
	}
	_, _, err := fieldFromStruct(sample{Name: "test"}, "nonexistent")
	if err == nil {
		t.Fatal("fieldFromStruct() = nil, want error for unknown field")
	}
}

// ── getConfigValue (direct calls for unreachable branches) ────────────────────

func TestGetConfigValue_UnknownTopLevel(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := internalconfig.Load(filepath.Join(dir, "config.toml"))
	_, _, err := getConfigValue(cfg, "bogus.something")
	if err == nil {
		t.Fatal("getConfigValue() = nil, want error for unknown top-level key")
	}
}

// ── applyInitValues ───────────────────────────────────────────────────────────

func TestApplyInitValues_WritesAllFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	err := applyInitValues(path, "mytoken", "12345", "nyc3", "s-2vcpu-4gb", "mytskey", "~/projects")
	if err != nil {
		t.Fatalf("applyInitValues() error = %v", err)
	}

	loaded, err := internalconfig.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Defaults.Token != "mytoken" {
		t.Errorf("Token = %q, want %q", loaded.Defaults.Token, "mytoken")
	}
	if loaded.Defaults.SSHKeyID != "12345" {
		t.Errorf("SSHKeyID = %q, want %q", loaded.Defaults.SSHKeyID, "12345")
	}
	if loaded.Defaults.Region != "nyc3" {
		t.Errorf("Region = %q, want %q", loaded.Defaults.Region, "nyc3")
	}
	if loaded.Defaults.Size != "s-2vcpu-4gb" {
		t.Errorf("Size = %q, want %q", loaded.Defaults.Size, "s-2vcpu-4gb")
	}
	if loaded.Defaults.TailscaleAuthKey != "mytskey" {
		t.Errorf("TailscaleAuthKey = %q, want %q", loaded.Defaults.TailscaleAuthKey, "mytskey")
	}
	// ProjectsDir gets ~ expanded by Load, so just check it's non-empty.
	if loaded.Defaults.ProjectsDir == "" {
		t.Error("ProjectsDir should be set")
	}
}

func TestApplyInitValues_TrimsSSHKeyIDWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	err := applyInitValues(path, "tok", "  99999  ", "nyc3", "s-2vcpu-4gb", "", "~/projects")
	if err != nil {
		t.Fatalf("applyInitValues() error = %v", err)
	}

	loaded, _ := internalconfig.Load(path)
	if loaded.Defaults.SSHKeyID != "99999" {
		t.Errorf("SSHKeyID = %q, want trimmed %q", loaded.Defaults.SSHKeyID, "99999")
	}
}

func TestApplyInitValues_LoadError(t *testing.T) {
	// Point at a directory — Load will fail trying to read it.
	dir := t.TempDir()
	// Create a directory named config.toml so Load returns a read error.
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(cfgPath, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	err := applyInitValues(cfgPath, "tok", "1", "nyc3", "s-2vcpu-4gb", "", "~/projects")
	if err == nil {
		t.Fatal("applyInitValues() = nil, want error for unreadable config path")
	}
}

func TestConfigGet_ReturnsValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[defaults]\nregion = \"nyc3\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "get", "defaults.region"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if strings.TrimSpace(buf.String()) != "nyc3" {
		t.Errorf("get defaults.region = %q, want %q", buf.String(), "nyc3")
	}
}

func TestConfigGet_RedactsSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[defaults]\ntoken = \"do_pat_v1_abcdefghijklmnopqrstuvwxyz1234\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "get", "defaults.token"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if strings.Contains(buf.String(), "do_pat_v1_abcdefghijklmnopqrstuvwxyz1234") {
		t.Error("get token returned unredacted secret")
	}
}

func TestConfigGet_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	rootCmd.SetArgs([]string{"--config", path, "config", "get", "defaults.nope"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("get unknown key = nil, want error")
	}
}

func TestConfigSet_SetsValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[defaults]\nregion = \"nyc3\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "set", "defaults.region", "sfo3"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Reload and verify via internal/config package
	internalCfg, _ := internalconfig.Load(path)
	if internalCfg.Defaults.Region != "sfo3" {
		t.Errorf("Region after set = %q, want %q", internalCfg.Defaults.Region, "sfo3")
	}
}

func TestConfigSet_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	rootCmd.SetArgs([]string{"--config", path, "config", "set", "defaults.nope", "val"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("set unknown key = nil, want error")
	}
}

func TestConfigShow_RedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[defaults]
token = "do_pat_v1_abcdefghijklmnopqrstuvwxyz1234"
region = "nyc3"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "show"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "do_pat_v1_abcdefghijklmnopqrstuvwxyz1234") {
		t.Error("show output contains unredacted token")
	}
	if !strings.Contains(out, "nyc3") {
		t.Error("show output missing region")
	}
}

func TestConfigShow_WithProfilesAndProjects(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[defaults]
region = "nyc3"

[profiles.large]
size = "s-4vcpu-8gb"

[projects.myapp]
repo = "git@github.com:user/myapp.git"
default_branch = "main"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "show"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "large") {
		t.Error("show output missing profile 'large'")
	}
	if !strings.Contains(out, "myapp") {
		t.Error("show output missing project 'myapp'")
	}
}

// ── getConfigValue ────────────────────────────────────────────────────────────

func TestGetConfigValue_ProjectsKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[projects.myapp]
repo = "git@github.com:user/myapp.git"
default_branch = "main"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "get", "projects.myapp.repo"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(buf.String(), "git@github.com:user/myapp.git") {
		t.Errorf("get projects.myapp.repo = %q, want repo URL", buf.String())
	}
}

func TestGetConfigValue_ProjectsKey_UnknownProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	rootCmd.SetArgs([]string{"--config", path, "config", "get", "projects.nope.repo"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("get unknown project = nil, want error")
	}
}

func TestGetConfigValue_ProfilesKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[profiles.large]\nsize = \"s-4vcpu-8gb\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "get", "profiles.large.size"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if strings.TrimSpace(buf.String()) != "s-4vcpu-8gb" {
		t.Errorf("get profiles.large.size = %q, want %q", buf.String(), "s-4vcpu-8gb")
	}
}

func TestGetConfigValue_ProfilesKey_UnknownProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	rootCmd.SetArgs([]string{"--config", path, "config", "get", "profiles.nope.size"})
	err := rootCmd.Execute()
	rootCmd.SetArgs(nil)
	if err == nil {
		t.Error("get unknown profile = nil, want error")
	}
}

func TestConfigProfileList_ShowsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[defaults]\nsize = \"s-2vcpu-4gb\"\nregion = \"nyc3\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "profile", "list"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	rootCmd.SetArgs(nil)

	out := buf.String()
	if !strings.Contains(out, "default") {
		t.Error("profile list output missing 'default' row")
	}
	if !strings.Contains(out, "nyc3") {
		t.Error("profile list output missing region 'nyc3'")
	}
}

func TestConfigProfileDelete_CannotDeleteDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	rootCmd.SetArgs([]string{"--config", path, "config", "profile", "delete", "default"})
	err := rootCmd.Execute()
	rootCmd.SetArgs(nil)
	if err == nil {
		t.Error("deleting 'default' profile should return error")
	}
}

func TestConfigProfileShow_UnknownProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	rootCmd.SetArgs([]string{"--config", path, "config", "profile", "show", "ghost"})
	err := rootCmd.Execute()
	rootCmd.SetArgs(nil)
	if err == nil {
		t.Error("show unknown profile should return error")
	}
}
