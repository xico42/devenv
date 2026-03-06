package config

// errors_test.go exercises error branches in Save, SetKey, and DeleteSection
// that require direct access to the unexported Config.path field.
// This file is in package config (not config_test) so it can construct
// a Config with a controlled path.

import (
	"os"
	"path/filepath"
	"testing"
)

// newConfigWithPath returns a Config whose internal path is set to p
// without reading or creating any file.
func newConfigWithPath(p string) *Config {
	return &Config{path: p}
}

// TestSave_MkdirAllError exercises the MkdirAll error branch in Save
// by pointing the config at a path whose parent directory cannot be created
// (parent exists as a file, not a dir).
func TestSave_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// Create a regular file at "parent" so MkdirAll("parent/sub") fails.
	parentFile := filepath.Join(dir, "parent")
	if err := os.WriteFile(parentFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := newConfigWithPath(filepath.Join(parentFile, "sub", "config.toml"))
	err := cfg.Save()
	if err == nil {
		t.Fatal("Save() = nil, want error when MkdirAll fails")
	}
}

// TestSetKey_ReadError exercises the non-ErrNotExist read-error branch in
// SetKey by pointing the config path at a directory (ReadFile returns an
// error that is not ErrNotExist).
func TestSetKey_ReadError(t *testing.T) {
	dir := t.TempDir()
	// Make the config path itself a directory so ReadFile returns a read error.
	cfgDir := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(cfgDir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	cfg := newConfigWithPath(cfgDir)
	err := cfg.SetKey("defaults.region", "nyc3")
	if err == nil {
		t.Fatal("SetKey() = nil, want error when ReadFile fails")
	}
}

// TestSetKey_WriteError exercises the os.Create error branch in SetKey
// by making the config directory read-only after MkdirAll would otherwise
// succeed (use a path inside an existing dir but make that dir read-only).
func TestSetKey_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions; skipping")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := newConfigWithPath(cfgPath)

	// Chmod dir read-only so os.Create fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := cfg.SetKey("defaults.region", "nyc3")
	if err == nil {
		t.Fatal("SetKey() = nil, want error when os.Create fails")
	}
}

// TestDeleteSection_ParseError exercises the TOML parse error branch in
// DeleteSection by writing an invalid TOML file.
func TestDeleteSection_ParseError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("{{invalid toml"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := newConfigWithPath(cfgPath)
	err := cfg.DeleteSection("defaults")
	if err == nil {
		t.Fatal("DeleteSection() = nil, want error when TOML is invalid")
	}
}

// TestDeleteSection_ReadError exercises the non-ErrNotExist read-error branch
// in DeleteSection by pointing the config path at a directory.
func TestDeleteSection_ReadError(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(cfgDir, 0o700); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	cfg := newConfigWithPath(cfgDir)
	err := cfg.DeleteSection("defaults")
	if err == nil {
		t.Fatal("DeleteSection() = nil, want error when ReadFile fails")
	}
}

// TestSetKey_ParseError exercises the TOML parse error branch in SetKey
// by writing a corrupt TOML file and then calling SetKey.
func TestSetKey_ParseError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	// Write invalid TOML so toml.LoadBytes fails.
	if err := os.WriteFile(cfgPath, []byte("{{invalid toml"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := newConfigWithPath(cfgPath)
	err := cfg.SetKey("defaults.region", "nyc3")
	if err == nil {
		t.Fatal("SetKey() = nil, want error when TOML is invalid")
	}
}

// TestSave_CreateError exercises the os.Create error branch in Save
// by making the config directory read-only after MkdirAll would otherwise succeed.
func TestSave_CreateError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions; skipping")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	cfg := newConfigWithPath(cfgPath)

	// Make dir read-only so os.Create fails.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	err := cfg.Save()
	if err == nil {
		t.Fatal("Save() = nil, want error when os.Create fails")
	}
}

// TestDeleteSection_WriteError exercises the os.Create error branch in
// DeleteSection by making the config file read-only (0o400) so os.Create fails.
func TestDeleteSection_WriteError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses file permissions; skipping")
	}
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")

	// Write a valid file first so DeleteSection reads it successfully.
	if err := os.WriteFile(cfgPath, []byte("[defaults]\nregion = \"nyc3\"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Make the file read-only so os.Create (O_WRONLY|O_CREATE|O_TRUNC) fails.
	if err := os.Chmod(cfgPath, 0o400); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(cfgPath, 0o600) })

	cfg := newConfigWithPath(cfgPath)
	err := cfg.DeleteSection("defaults")
	if err == nil {
		t.Fatal("DeleteSection() = nil, want error when os.Create fails")
	}
}
