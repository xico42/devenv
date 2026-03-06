# `devenv config` Command Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the full `devenv config` command with subcommands `init`, `show`, `set`, `get`, and `profile create/list/delete/show`.

**Architecture:** `internal/config` gains comment-preserving `SetKey`/`DeleteSection` (via `pelletier/go-toml` v1 Tree API), struct validation (`go-playground/validator/v10`), and a `Redact` helper. `cmd/config.go` expands from a stub to 8 subcommands; interactive wizards use `charmbracelet/huh`. The `internal/do` package gains SSH key and region listing for `config init`.

**Tech Stack:** Go, Cobra, `pelletier/go-toml` v1, `go-playground/validator/v10`, `charmbracelet/huh`, `digitalocean/godo`.

**Worktree:** `~/.config/superpowers/worktrees/remote-dev/feat/config-command/`

All commands below assume you are **inside the worktree directory**.

---

### Task 1: Swap `BurntSushi/toml` → `pelletier/go-toml` v1

The APIs are identical for struct ops — only the import path changes. This is a pure library swap; no logic changes.

**Files:**
- Modify: `internal/config/config.go` (import only)
- Modify: `go.mod`, `go.sum`

**Step 1: Add new dep, remove old one**

```bash
go get github.com/pelletier/go-toml@latest
go get github.com/BurntSushi/toml@none
go mod tidy
```

**Step 2: Update import in `internal/config/config.go`**

Replace:
```go
"github.com/BurntSushi/toml"
```
With:
```go
"github.com/pelletier/go-toml"
```

No other code changes — `toml.Unmarshal(data, cfg)` and `toml.NewEncoder(f).Encode(c)` have identical signatures in both libraries.

**Step 3: Run tests to verify nothing broke**

```bash
go test ./...
```

Expected: all packages pass, same as before.

**Step 4: Commit**

```bash
git add go.mod go.sum internal/config/config.go
git commit -m "chore: swap BurntSushi/toml for pelletier/go-toml v1"
```

---

### Task 2: Add `go-playground/validator/v10` + `Validate()` method

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/project.go`
- Modify: `internal/config/notify.go`
- Modify: `internal/config/config_test.go`

**Step 1: Add dep**

```bash
go get github.com/go-playground/validator/v10
go mod tidy
```

**Step 2: Add validation tags to structs**

In `internal/config/config.go`, update `DefaultsConfig` and `ProfileConfig`:

```go
// DefaultsConfig holds default values applied to every droplet.
type DefaultsConfig struct {
	Token            string `toml:"token"             validate:"omitempty"          secret:"true"`
	SSHKeyID         string `toml:"ssh_key_id"         validate:"omitempty"`
	Region           string `toml:"region"             validate:"omitempty"`
	Size             string `toml:"size"               validate:"omitempty"`
	TailscaleAuthKey string `toml:"tailscale_auth_key" validate:"omitempty"          secret:"true"`
	Image            string `toml:"image"              validate:"omitempty"`
	ProjectsDir      string `toml:"projects_dir"       validate:"omitempty"`
	GitIdentityFile  string `toml:"git_identity_file"  validate:"omitempty"`
}

// ProfileConfig holds per-profile overrides.
type ProfileConfig struct {
	Size   string `toml:"size"   validate:"omitempty"`
	Region string `toml:"region" validate:"omitempty"`
	Image  string `toml:"image"  validate:"omitempty"`
}
```

In `internal/config/project.go`, update `ProjectConfig`:

```go
type ProjectConfig struct {
	Repo          string `toml:"repo"           validate:"omitempty"`
	DefaultBranch string `toml:"default_branch" validate:"omitempty"`
	EnvTemplate   string `toml:"env_template"   validate:"omitempty"`
}
```

In `internal/config/notify.go`, add `secret:"true"` to credential fields:

```go
type TelegramNotifyConfig struct {
	BotToken string `toml:"bot_token" secret:"true"`
	ChatID   string `toml:"chat_id"   secret:"true"`
}

type SlackNotifyConfig struct {
	WebhookURL string `toml:"webhook_url" secret:"true"`
}

type DiscordNotifyConfig struct {
	WebhookURL string `toml:"webhook_url" secret:"true"`
}

type WebhookNotifyConfig struct {
	URL          string            `toml:"url"           secret:"true"`
	Method       string            `toml:"method"`
	Headers      map[string]string `toml:"headers"`
	BodyTemplate string            `toml:"body_template"`
}
```

**Step 3: Add `Validate()` to `Config` in `internal/config/config.go`**

Add at the package level (below imports):

```go
import "github.com/go-playground/validator/v10"

var configValidator = validator.New()
```

Add method:

```go
// Validate runs struct validation on the config.
func (c *Config) Validate() error {
	return configValidator.Struct(c)
}
```

**Step 4: Write the failing test**

In `internal/config/config_test.go`, add:

```go
func TestConfig_Validate_EmptyConfig(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	// Empty config is valid — nothing is required at file level.
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() on empty config = %v, want nil", err)
	}
}
```

**Step 5: Run test**

```bash
go test ./internal/config/... -run TestConfig_Validate_EmptyConfig -v
```

Expected: PASS.

**Step 6: Run full test suite**

```bash
go test ./...
```

Expected: all pass.

**Step 7: Commit**

```bash
git add go.mod go.sum internal/config/config.go internal/config/project.go internal/config/notify.go internal/config/config_test.go
git commit -m "feat(config): add validator/v10 and Validate() method"
```

---

### Task 3: Add `Redact()` helper

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write the failing test first**

In `internal/config/config_test.go`, add:

```go
func TestRedact(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"do_pat_v1_abcdefghijklmnopqrstuvwxyz1234", "do_pat_v1****1234"},
		{"short", "****"},
		{"exactly12ch!", "****"},
		{"", "****"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := config.Redact(tt.input)
			if got != tt.want {
				t.Errorf("Redact(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test ./internal/config/... -run TestRedact -v
```

Expected: FAIL — `config.Redact` undefined.

**Step 3: Add `Redact()` to `internal/config/config.go`**

```go
// Redact masks a secret string, showing the first 8 and last 4 characters.
// Strings of 12 characters or fewer are fully masked.
func Redact(s string) string {
	if len(s) <= 12 {
		return "****"
	}
	return s[:8] + "****" + s[len(s)-4:]
}
```

**Step 4: Run test to verify it passes**

```bash
go test ./internal/config/... -run TestRedact -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add Redact() helper and secret struct tags"
```

---

### Task 4: Add `Path()` getter and key-path validation

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing tests**

In `internal/config/config_test.go`, add:

```go
func TestConfig_Path(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg, _ := config.Load(path)
	if cfg.Path() != path {
		t.Errorf("Path() = %q, want %q", cfg.Path(), path)
	}
}

func TestIsValidKeyPath(t *testing.T) {
	tests := []struct {
		path  string
		valid bool
	}{
		{"defaults.token", true},
		{"defaults.region", true},
		{"defaults.ssh_key_id", true},
		{"defaults.foo", false},
		{"profiles.heavy.size", true},
		{"profiles.heavy.region", true},
		{"profiles.heavy.unknown", false},
		{"projects.myapp.repo", true},
		{"projects.myapp.default_branch", true},
		{"projects.myapp.env_template", true},
		{"projects.myapp.nope", false},
		{"notify.provider", true},
		{"notify.telegram.bot_token", true},
		{"unknown.key", false},
		{"defaults", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := config.IsValidKeyPath(tt.path)
			if got != tt.valid {
				t.Errorf("IsValidKeyPath(%q) = %v, want %v", tt.path, got, tt.valid)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/... -run "TestConfig_Path|TestIsValidKeyPath" -v
```

Expected: FAIL.

**Step 3: Add `Path()` and `IsValidKeyPath()` to `internal/config/config.go`**

```go
import (
	"reflect"
	"strings"
	"sync"
)

// Path returns the config file path.
func (c *Config) Path() string { return c.path }

var (
	keyInitOnce    sync.Once
	defaultsKeys   map[string]bool
	profileKeys    map[string]bool
	projectKeys    map[string]bool
)

func initKeyMaps() {
	keyInitOnce.Do(func() {
		defaultsKeys = tomlFields(reflect.TypeOf(DefaultsConfig{}))
		profileKeys  = tomlFields(reflect.TypeOf(ProfileConfig{}))
		projectKeys  = tomlFields(reflect.TypeOf(ProjectConfig{}))
	})
}

func tomlFields(t reflect.Type) map[string]bool {
	fields := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		tag := t.Field(i).Tag.Get("toml")
		if tag != "" && tag != "-" {
			fields[tag] = true
		}
	}
	return fields
}

// IsValidKeyPath reports whether a dot-notation key path is settable via SetKey.
func IsValidKeyPath(dotPath string) bool {
	if dotPath == "" {
		return false
	}
	initKeyMaps()
	parts := strings.SplitN(dotPath, ".", 3)
	switch parts[0] {
	case "defaults":
		return len(parts) == 2 && defaultsKeys[parts[1]]
	case "profiles":
		return len(parts) == 3 && profileKeys[parts[2]]
	case "projects":
		return len(parts) == 3 && projectKeys[parts[2]]
	case "notify":
		return len(parts) >= 2
	default:
		return false
	}
}
```

**Step 4: Run tests**

```bash
go test ./internal/config/... -run "TestConfig_Path|TestIsValidKeyPath" -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add Path() and IsValidKeyPath()"
```

---

### Task 5: Add `SetKey()` (comment-preserving TOML edit)

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing tests**

```go
func TestConfig_SetKey_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	// Write initial config with a comment that must survive the edit.
	content := "# my important comment\n[defaults]\nregion = \"nyc3\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(path)

	if err := cfg.SetKey("defaults.region", "sfo3"); err != nil {
		t.Fatalf("SetKey() error = %v", err)
	}

	// Value updated
	cfg2, _ := config.Load(path)
	if cfg2.Defaults.Region != "sfo3" {
		t.Errorf("Region = %q, want %q", cfg2.Defaults.Region, "sfo3")
	}

	// Comment preserved
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "# my important comment") {
		t.Error("SetKey() clobbered the comment in the file")
	}
}

func TestConfig_SetKey_UnknownKey(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "config.toml"))
	err := cfg.SetKey("defaults.nonexistent", "val")
	if err == nil {
		t.Fatal("SetKey() with unknown key = nil, want error")
	}
}

func TestConfig_SetKey_CreatesNewProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	cfg, _ := config.Load(path)

	if err := cfg.SetKey("profiles.heavy.size", "s-8vcpu-16gb"); err != nil {
		t.Fatalf("SetKey() error = %v", err)
	}

	cfg2, _ := config.Load(path)
	p, err := cfg2.Profile("heavy")
	if err != nil {
		t.Fatalf("Profile() error = %v", err)
	}
	if p.Size != "s-8vcpu-16gb" {
		t.Errorf("Size = %q, want %q", p.Size, "s-8vcpu-16gb")
	}
}
```

**Step 2: Run tests to verify they fail**

```bash
go test ./internal/config/... -run "TestConfig_SetKey" -v
```

Expected: FAIL.

**Step 3: Add `SetKey()` to `internal/config/config.go`**

```go
// SetKey sets a single config value by dot-notation key, preserving file comments.
// It re-reads the file before writing to avoid clobbering concurrent changes.
func (c *Config) SetKey(dotPath, value string) error {
	if !IsValidKeyPath(dotPath) {
		return fmt.Errorf("unknown config key %q. Run 'devenv config show' to see valid keys", dotPath)
	}

	data, err := os.ReadFile(c.path)
	var tree *toml.Tree
	if errors.Is(err, fs.ErrNotExist) || len(data) == 0 {
		tree, err = toml.Load("")
		if err != nil {
			return fmt.Errorf("creating empty config tree: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("reading config: %w", err)
	} else {
		tree, err = toml.LoadBytes(data)
		if err != nil {
			return fmt.Errorf("parsing config: %w", err)
		}
	}

	tree.Set(dotPath, value)

	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	f, err := os.Create(c.path)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	defer f.Close() //nolint:errcheck
	if _, err := tree.WriteTo(f); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	updated, err := Load(c.path)
	if err != nil {
		return err
	}
	return updated.Validate()
}
```

**Step 4: Run tests**

```bash
go test ./internal/config/... -run "TestConfig_SetKey" -v
```

Expected: PASS.

**Step 5: Run full suite**

```bash
go test ./...
```

**Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add SetKey() with comment preservation"
```

---

### Task 6: Add `DeleteSection()`

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Step 1: Write failing test**

```go
func TestConfig_DeleteSection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "[defaults]\nregion = \"nyc3\"\n\n[profiles.heavy]\nsize = \"s-8vcpu-16gb\"\n"
	os.WriteFile(path, []byte(content), 0o600)
	cfg, _ := config.Load(path)

	if err := cfg.DeleteSection("profiles.heavy"); err != nil {
		t.Fatalf("DeleteSection() error = %v", err)
	}

	cfg2, _ := config.Load(path)
	if _, err := cfg2.Profile("heavy"); err == nil {
		t.Error("Profile heavy still exists after DeleteSection")
	}
	// defaults must be untouched
	if cfg2.Defaults.Region != "nyc3" {
		t.Errorf("Region = %q after delete, want nyc3", cfg2.Defaults.Region)
	}
}

func TestConfig_DeleteSection_NonExistent(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "config.toml"))
	// Must not error on missing section.
	if err := cfg.DeleteSection("profiles.ghost"); err != nil {
		t.Errorf("DeleteSection non-existent = %v, want nil", err)
	}
}
```

**Step 2: Run to verify failure**

```bash
go test ./internal/config/... -run "TestConfig_DeleteSection" -v
```

**Step 3: Add `DeleteSection()` to `internal/config/config.go`**

```go
// DeleteSection removes a dot-notation section from the config file.
func (c *Config) DeleteSection(dotPath string) error {
	data, err := os.ReadFile(c.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil // nothing to delete
	}
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	tree, err := toml.LoadBytes(data)
	if err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	if err := tree.Delete(dotPath); err != nil {
		// go-toml returns error for missing keys; treat as no-op.
		return nil
	}

	f, err := os.Create(c.path)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	defer f.Close() //nolint:errcheck
	if _, err := tree.WriteTo(f); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return nil
}
```

**Step 4: Run tests**

```bash
go test ./internal/config/... -run "TestConfig_DeleteSection" -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): add DeleteSection()"
```

---

### Task 7: Implement `config show`

**Files:**
- Modify: `cmd/config.go`

**Step 1: Write failing test in `cmd/root_test.go` (or create `cmd/config_test.go`)**

Create `cmd/config_test.go`:

```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigShow_RedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
[defaults]
token = "do_pat_v1_abcdefghijklmnopqrstuvwxyz1234"
region = "nyc3"
`
	os.WriteFile(path, []byte(content), 0o600)

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
```

**Step 2: Run to verify failure**

```bash
go test ./cmd/... -run TestConfigShow_RedactsSecrets -v
```

Expected: FAIL (prints "not implemented").

**Step 3: Replace `cmd/config.go` with full subcommand implementation**

Replace the current stub with:

```go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xico42/devenv/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage devenv configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the current config (secrets redacted)",
	RunE:  runConfigShow,
}

func runConfigShow(cmd *cobra.Command, _ []string) error {
	c := cmd.Root().Annotations["cfg"].(*config.Config) // see note below
	// ... print each section
	printDefaults(cmd, c)
	printProfiles(cmd, c)
	printProjects(cmd, c)
	printNotify(cmd, c)
	return nil
}
```

**Note on config access:** `PersistentPreRunE` in `root.go` stores the loaded config in the package-level `cfg` variable. The `show` command accesses it via the `cfg` package variable directly (already the pattern in the codebase). No `Annotations` map needed.

Here is the actual `cmd/config.go` implementation for `show`:

```go
package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/xico42/devenv/internal/config"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configProfileCmd)
	configProfileCmd.AddCommand(configProfileCreateCmd)
	configProfileCmd.AddCommand(configProfileListCmd)
	configProfileCmd.AddCommand(configProfileDeleteCmd)
	configProfileCmd.AddCommand(configProfileShowCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage devenv configuration",
}

// ── show ─────────────────────────────────────────────────────────────────────

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the current config (secrets redacted)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		d := cfg.Defaults
		fmt.Fprintln(w, "[defaults]")
		fmt.Fprintf(w, "  token\t= %q\n", config.Redact(d.Token))
		fmt.Fprintf(w, "  ssh_key_id\t= %q\n", d.SSHKeyID)
		fmt.Fprintf(w, "  region\t= %q\n", d.Region)
		fmt.Fprintf(w, "  size\t= %q\n", d.Size)
		fmt.Fprintf(w, "  image\t= %q\n", d.Image)
		fmt.Fprintf(w, "  tailscale_auth_key\t= %q\n", config.Redact(d.TailscaleAuthKey))
		fmt.Fprintf(w, "  git_identity_file\t= %q\n", d.GitIdentityFile)
		fmt.Fprintf(w, "  projects_dir\t= %q\n", d.ProjectsDir)

		if len(cfg.Profiles) > 0 {
			fmt.Fprintln(w, "\n[profiles]")
			for name, p := range cfg.Profiles {
				fmt.Fprintf(w, "  %s:\n", name)
				if p.Size != "" {
					fmt.Fprintf(w, "    size\t= %q\n", p.Size)
				}
				if p.Region != "" {
					fmt.Fprintf(w, "    region\t= %q\n", p.Region)
				}
			}
		}

		if len(cfg.Projects) > 0 {
			fmt.Fprintln(w, "\n[projects]")
			for name, p := range cfg.Projects {
				fmt.Fprintf(w, "  %s:\n", name)
				fmt.Fprintf(w, "    repo\t= %q\n", p.Repo)
				fmt.Fprintf(w, "    default_branch\t= %q\n", p.DefaultBranch)
			}
		}

		if cfg.Notify.Provider != "" {
			fmt.Fprintln(w, "\n[notify]")
			fmt.Fprintf(w, "  provider\t= %q\n", cfg.Notify.Provider)
		}
		return w.Flush()
	},
}
```

**Step 4: Add placeholder stubs for the other subcommands** (so the file compiles):

```go
var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		return nil
	},
}

var configProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage named profiles",
}

var configProfileCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a named profile interactively",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		return nil
	},
}

var configProfileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		return nil
	},
}

var configProfileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		return nil
	},
}

var configProfileShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a profile's settings",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "not implemented")
		return nil
	},
}
```

**Step 5: Run tests**

```bash
go test ./cmd/... -run TestConfigShow_RedactsSecrets -v
```

Expected: PASS.

**Step 6: Commit**

```bash
git add cmd/config.go cmd/config_test.go
git commit -m "feat(config): implement config show subcommand"
```

---

### Task 8: Implement `config get <key>`

**Files:**
- Modify: `cmd/config.go`
- Modify: `cmd/config_test.go`

**Step 1: Write failing tests**

Add to `cmd/config_test.go`:

```go
func TestConfigGet_ReturnsValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte("[defaults]\nregion = \"nyc3\"\n"), 0o600)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "get", "defaults.region"})
	rootCmd.Execute()

	if strings.TrimSpace(buf.String()) != "nyc3" {
		t.Errorf("get defaults.region = %q, want %q", buf.String(), "nyc3")
	}
}

func TestConfigGet_RedactsSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte("[defaults]\ntoken = \"do_pat_v1_abcdefghijklmnopqrstuvwxyz1234\"\n"), 0o600)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "get", "defaults.token"})
	rootCmd.Execute()

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
```

**Step 2: Run to verify failure**

```bash
go test ./cmd/... -run "TestConfigGet" -v
```

**Step 3: Implement `configGetCmd` in `cmd/config.go`**

The `get` command reads from the already-loaded `cfg` struct using a field lookup map. Build the map from the struct using `reflect`:

```go
var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value (secrets redacted)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		if !config.IsValidKeyPath(key) {
			return fmt.Errorf("unknown config key %q. Run 'devenv config show' to see valid keys", key)
		}
		val, isSecret, err := getConfigValue(cfg, key)
		if err != nil {
			return err
		}
		if isSecret {
			val = config.Redact(val)
		}
		fmt.Fprintln(cmd.OutOrStdout(), val)
		return nil
	},
}

// getConfigValue retrieves a dot-notation key from the loaded Config struct.
// Returns the string value and whether the field is marked secret.
func getConfigValue(c *config.Config, dotPath string) (string, bool, error) {
	parts := strings.SplitN(dotPath, ".", 3)
	switch parts[0] {
	case "defaults":
		return fieldFromStruct(c.Defaults, parts[1])
	case "profiles":
		p, err := c.Profile(parts[1])
		if err != nil {
			return "", false, err
		}
		return fieldFromStruct(p, parts[2])
	case "projects":
		proj, ok := c.Projects[parts[1]]
		if !ok {
			return "", false, fmt.Errorf("project %q not found", parts[1])
		}
		return fieldFromStruct(proj, parts[2])
	case "notify":
		// notify is handled manually since it has nested sub-structs
		if parts[1] == "provider" {
			return c.Notify.Provider, false, nil
		}
		return "", false, fmt.Errorf("use 'config show' for nested notify values")
	}
	return "", false, fmt.Errorf("unknown key %q", dotPath)
}

// fieldFromStruct returns the string value of the toml-tagged field in v.
func fieldFromStruct(v any, tomlKey string) (string, bool, error) {
	rv := reflect.ValueOf(v)
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		if f.Tag.Get("toml") == tomlKey {
			secret := f.Tag.Get("secret") == "true"
			return rv.Field(i).String(), secret, nil
		}
	}
	return "", false, fmt.Errorf("field %q not found", tomlKey)
}
```

Add `"reflect"` and `"strings"` to imports.

**Step 4: Run tests**

```bash
go test ./cmd/... -run "TestConfigGet" -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/config.go cmd/config_test.go
git commit -m "feat(config): implement config get subcommand"
```

---

### Task 9: Implement `config set <key> <value>`

**Files:**
- Modify: `cmd/config.go`
- Modify: `cmd/config_test.go`

**Step 1: Write failing test**

```go
func TestConfigSet_SetsValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte("[defaults]\nregion = \"nyc3\"\n"), 0o600)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "set", "defaults.region", "sfo3"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	// Reload and verify
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
```

Add `internalconfig "github.com/xico42/devenv/internal/config"` to test imports.

**Step 2: Run to verify failure**

```bash
go test ./cmd/... -run "TestConfigSet" -v
```

**Step 3: Implement `configSetCmd` in `cmd/config.go`**

```go
var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, val := args[0], args[1]
		if err := cfg.SetKey(key, val); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %q\n", key, val)
		return nil
	},
}
```

**Step 4: Run tests**

```bash
go test ./cmd/... -run "TestConfigSet" -v
```

Expected: PASS.

**Step 5: Commit**

```bash
git add cmd/config.go cmd/config_test.go
git commit -m "feat(config): implement config set subcommand"
```

---

### Task 10: Add SSH key and region listing to `internal/do`

Needed by `config init` to populate the wizard dropdowns.

**Files:**
- Modify: `internal/do/client.go`
- Create: `internal/do/init.go`
- Create: `internal/do/init_test.go`

**Step 1: Write failing test**

Create `internal/do/init_test.go`:

```go
package do_test

import (
	"context"
	"testing"

	"github.com/digitalocean/godo"

	"github.com/xico42/devenv/internal/do"
)

type mockKeysService struct {
	keys []godo.Key
}

func (m *mockKeysService) List(_ context.Context, _ *godo.ListOptions) ([]godo.Key, *godo.Response, error) {
	return m.keys, &godo.Response{}, nil
}

type mockRegionsService struct {
	regions []godo.Region
}

func (m *mockRegionsService) List(_ context.Context, _ *godo.ListOptions) ([]godo.Region, *godo.Response, error) {
	return m.regions, &godo.Response{}, nil
}

func TestClient_ListSSHKeys(t *testing.T) {
	c := &do.Client{
		Keys: &mockKeysService{
			keys: []godo.Key{{ID: 1, Name: "MyKey", Fingerprint: "aa:bb"}},
		},
	}
	keys, err := c.ListSSHKeys(context.Background())
	if err != nil {
		t.Fatalf("ListSSHKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0].Name != "MyKey" {
		t.Errorf("ListSSHKeys() = %v, want 1 key named MyKey", keys)
	}
}

func TestClient_ListRegions(t *testing.T) {
	c := &do.Client{
		Regions: &mockRegionsService{
			regions: []godo.Region{{Slug: "nyc3", Name: "New York 3", Available: true}},
		},
	}
	regions, err := c.ListRegions(context.Background())
	if err != nil {
		t.Fatalf("ListRegions() error = %v", err)
	}
	if len(regions) != 1 || regions[0].Slug != "nyc3" {
		t.Errorf("ListRegions() = %v, want 1 region nyc3", regions)
	}
}
```

**Step 2: Run to verify failure**

```bash
go test ./internal/do/... -run "TestClient_List" -v
```

**Step 3: Add interfaces and methods to `internal/do/client.go`**

Add to `client.go`:

```go
// SSHKeysService is the subset of godo.KeysService used by devenv.
type SSHKeysService interface {
	List(ctx context.Context, opts *godo.ListOptions) ([]godo.Key, *godo.Response, error)
}

// RegionsService is the subset of godo.RegionsService used by devenv.
type RegionsService interface {
	List(ctx context.Context, opts *godo.ListOptions) ([]godo.Region, *godo.Response, error)
}

// Client wraps the DO API with only the methods devenv needs.
type Client struct {
	Droplets DropletsService
	Keys     SSHKeysService
	Regions  RegionsService
}

// New returns an authenticated Client for the given token.
func New(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(context.Background(), ts)
	g := godo.NewClient(tc)
	return &Client{
		Droplets: g.Droplets,
		Keys:     g.Keys,
		Regions:  g.Regions,
	}, nil
}
```

Create `internal/do/init.go`:

```go
package do

import (
	"context"
	"fmt"

	"github.com/digitalocean/godo"
)

// ListSSHKeys returns all SSH keys on the account.
func (c *Client) ListSSHKeys(ctx context.Context) ([]godo.Key, error) {
	keys, _, err := c.Keys.List(ctx, &godo.ListOptions{PerPage: 200})
	if err != nil {
		return nil, fmt.Errorf("listing SSH keys: %w", err)
	}
	return keys, nil
}

// ListRegions returns all available regions.
func (c *Client) ListRegions(ctx context.Context) ([]godo.Region, error) {
	regions, _, err := c.Regions.List(ctx, &godo.ListOptions{PerPage: 200})
	if err != nil {
		return nil, fmt.Errorf("listing regions: %w", err)
	}
	return regions, nil
}
```

**Step 4: Run tests**

```bash
go test ./internal/do/... -run "TestClient_List" -v
```

Expected: PASS.

**Step 5: Run full suite to check nothing broke**

```bash
go test ./...
```

**Step 6: Commit**

```bash
git add internal/do/client.go internal/do/init.go internal/do/init_test.go
git commit -m "feat(do): add SSH key and region listing for config init"
```

---

### Task 11: Implement `config init` wizard

**Files:**
- Modify: `cmd/config.go`

Note: `config init` uses `charmbracelet/huh` for interactive prompts. The huh forms **cannot be unit tested without a TTY**. Tests cover the supporting DO client mock path but not the interactive flow.

**Step 1: Add huh dependency**

```bash
go get github.com/charmbracelet/huh@latest
go mod tidy
```

**Step 2: Define init API client interface and add `configInitCmd` to `cmd/config.go`**

```go
import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/digitalocean/godo"
	"github.com/spf13/cobra"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/do"
)

// initAPIClient is the interface config init uses to fetch SSH keys and regions.
// *do.Client satisfies this interface.
type initAPIClient interface {
	ListSSHKeys(ctx context.Context) ([]godo.Key, error)
	ListRegions(ctx context.Context) ([]godo.Region, error)
}

// configInitAPIClientFunc allows injecting a mock in tests.
var configInitAPIClientFunc = func(token string) (initAPIClient, error) {
	return do.New(token)
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Interactive first-run setup wizard",
	RunE:  runConfigInit,
}

func runConfigInit(cmd *cobra.Command, _ []string) error {
	// If config already exists, confirm overwrite.
	if cfg.Defaults.Token != "" || cfg.Defaults.Region != "" {
		var overwrite bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Config already exists. Overwrite?").
				Value(&overwrite),
		)).Run(); err != nil {
			return err
		}
		if !overwrite {
			return nil
		}
	}

	// Phase 1: get token.
	var token string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Digital Ocean API token").
			EchoMode(huh.EchoModePassword).
			Value(&token),
	)).Run(); err != nil {
		return err
	}

	// Phase 2: fetch SSH keys and regions from DO API.
	ctx := context.Background()
	var sshKeyOpts []huh.Option[string]
	var regionOpts []huh.Option[string]

	client, err := configInitAPIClientFunc(token)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not connect to DO API (%v). Enter values manually.\n", err)
	} else {
		keys, kerr := client.ListSSHKeys(ctx)
		if kerr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not fetch SSH keys (%v).\n", kerr)
		} else {
			for _, k := range keys {
				sshKeyOpts = append(sshKeyOpts, huh.NewOption(
					fmt.Sprintf("%s (%d)", k.Name, k.ID),
					strconv.Itoa(k.ID),
				))
			}
		}

		regions, rerr := client.ListRegions(ctx)
		if rerr != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not fetch regions (%v).\n", rerr)
		} else {
			for _, r := range regions {
				if r.Available {
					regionOpts = append(regionOpts, huh.NewOption(
						fmt.Sprintf("%s - %s", r.Slug, r.Name),
						r.Slug,
					))
				}
			}
		}
	}

	// Phase 3: collect remaining values.
	var sshKeyID, region, size, tsKey, projectsDir string
	projectsDir = "~/projects"

	sizeOpts := []huh.Option[string]{
		huh.NewOption("s-2vcpu-4gb  ($18/mo, $0.027/hr)  -- recommended", "s-2vcpu-4gb"),
		huh.NewOption("s-4vcpu-8gb  ($36/mo, $0.054/hr)", "s-4vcpu-8gb"),
		huh.NewOption("s-8vcpu-16gb ($72/mo, $0.107/hr)", "s-8vcpu-16gb"),
	}

	var group []huh.Field

	if len(sshKeyOpts) > 0 {
		group = append(group, huh.NewSelect[string]().Title("SSH key to use").Options(sshKeyOpts...).Value(&sshKeyID))
	} else {
		group = append(group, huh.NewInput().Title("SSH key ID").Value(&sshKeyID))
	}

	if len(regionOpts) > 0 {
		group = append(group, huh.NewSelect[string]().Title("Default region").Options(regionOpts...).Value(&region))
	} else {
		group = append(group, huh.NewInput().Title("Default region").Value(&region))
	}

	group = append(group,
		huh.NewSelect[string]().Title("Default droplet size").Options(sizeOpts...).Value(&size),
		huh.NewInput().Title("Tailscale auth key (optional, press Enter to skip)").Value(&tsKey),
		huh.NewInput().Title("Projects directory").Value(&projectsDir),
	)

	if err := huh.NewForm(huh.NewGroup(group...)).Run(); err != nil {
		return err
	}

	// Build and save config.
	newCfg := &config.Config{} // use exported constructor below
	// We need a fresh Config at the correct path — use Load() on the path.
	freshCfg, err := config.Load(cfg.Path())
	if err != nil {
		return err
	}
	_ = newCfg
	freshCfg.Defaults.Token = token
	freshCfg.Defaults.SSHKeyID = strings.TrimSpace(sshKeyID)
	freshCfg.Defaults.Region = region
	freshCfg.Defaults.Size = size
	freshCfg.Defaults.TailscaleAuthKey = tsKey
	freshCfg.Defaults.ProjectsDir = projectsDir

	if err := freshCfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nConfig written to %s\n", cfg.Path())
	return nil
}
```

**Important:** `config.Config` has an unexported `path` field. `Load()` sets it. So calling `config.Load(cfg.Path())` on an existing (or new) path gives a fresh `Config` with `path` set correctly.

Add `configInitCmd` to the `init()` function:
```go
configCmd.AddCommand(configInitCmd)
```

**Step 3: Build and verify**

```bash
go build ./...
go test ./...
```

Expected: build succeeds, all tests pass.

**Step 4: Commit**

```bash
git add cmd/config.go go.mod go.sum
git commit -m "feat(config): implement config init wizard"
```

---

### Task 12: Implement profile subcommands

**Files:**
- Modify: `cmd/config.go`
- Modify: `cmd/config_test.go`

**Step 1: Write failing tests**

Add to `cmd/config_test.go`:

```go
func TestConfigProfileList_ShowsDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	os.WriteFile(path, []byte("[defaults]\nsize = \"s-2vcpu-4gb\"\nregion = \"nyc3\"\n"), 0o600)

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetArgs([]string{"--config", path, "config", "profile", "list"})
	rootCmd.Execute()

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
	if err == nil {
		t.Error("deleting 'default' profile should return error")
	}
}

func TestConfigProfileShow_UnknownProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	rootCmd.SetArgs([]string{"--config", path, "config", "profile", "show", "ghost"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("show unknown profile should return error")
	}
}
```

**Step 2: Run to verify failure**

```bash
go test ./cmd/... -run "TestConfigProfile" -v
```

**Step 3: Implement profile subcommands in `cmd/config.go`**

Replace the profile stubs with:

```go
var configProfileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all profiles",
	RunE: func(cmd *cobra.Command, _ []string) error {
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "  PROFILE\tSIZE\tREGION")
		fmt.Fprintf(w, "  default\t%s\t%s\t(from [defaults])\n",
			cfg.Defaults.Size, cfg.Defaults.Region)
		for name, p := range cfg.Profiles {
			fmt.Fprintf(w, "  %s\t%s\t%s\n", name, p.Size, p.Region)
		}
		return w.Flush()
	},
}

var configProfileShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a profile's settings",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := cfg.Profile(args[0])
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "  size\t= %q\n", p.Size)
		fmt.Fprintf(w, "  region\t= %q\n", p.Region)
		return w.Flush()
	},
}

var configProfileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a named profile",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if name == "default" {
			return fmt.Errorf("%q is not a profile — edit [defaults] directly", name)
		}
		if _, err := cfg.Profile(name); err != nil {
			return fmt.Errorf("profile %q does not exist", name)
		}
		var confirm bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("Delete profile %q?", name)).
				Value(&confirm),
		)).Run(); err != nil {
			return err
		}
		if !confirm {
			return nil
		}
		if err := cfg.DeleteSection("profiles." + name); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Profile %q deleted\n", name)
		return nil
	},
}

var configProfileCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a named profile interactively",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sizeOpts := []huh.Option[string]{
			huh.NewOption("s-2vcpu-4gb  ($18/mo, $0.027/hr)", "s-2vcpu-4gb"),
			huh.NewOption("s-4vcpu-8gb  ($36/mo, $0.054/hr)", "s-4vcpu-8gb"),
			huh.NewOption("s-8vcpu-16gb ($72/mo, $0.107/hr)", "s-8vcpu-16gb"),
		}

		var size, region string
		defaultRegion := cfg.Defaults.Region
		region = defaultRegion

		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Size").Options(sizeOpts...).Value(&size),
			huh.NewInput().
				Title(fmt.Sprintf("Region (Enter to use default %s)", defaultRegion)).
				Value(&region),
		)).Run(); err != nil {
			return err
		}

		if err := cfg.SetKey("profiles."+name+".size", size); err != nil {
			return err
		}
		if region != "" && region != defaultRegion {
			if err := cfg.SetKey("profiles."+name+".region", region); err != nil {
				return err
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Profile %q created\nUse it with: devenv up --profile %s\n", name, name)
		return nil
	},
}
```

**Step 4: Run tests**

```bash
go test ./cmd/... -run "TestConfigProfile" -v
```

Expected: PASS.

**Step 5: Run full suite**

```bash
go test ./...
```

**Step 6: Commit**

```bash
git add cmd/config.go cmd/config_test.go
git commit -m "feat(config): implement profile subcommands"
```

---

### Task 13: Final wiring, lint, and coverage

**Step 1: Ensure all subcommands are wired in `init()`**

In `cmd/config.go`, verify the `init()` function includes all commands:

```go
func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configProfileCmd)
	configProfileCmd.AddCommand(configProfileCreateCmd)
	configProfileCmd.AddCommand(configProfileListCmd)
	configProfileCmd.AddCommand(configProfileDeleteCmd)
	configProfileCmd.AddCommand(configProfileShowCmd)
}
```

**Step 2: Run lint**

```bash
make lint
```

Fix any issues before proceeding.

**Step 3: Run coverage**

```bash
make coverage
```

Expected: ≥80% aggregate. If below, add tests to cover missing branches in `internal/config` (particularly `SetKey` and `DeleteSection` edge cases).

**Step 4: Run full test suite one final time**

```bash
make test
```

**Step 5: Commit if any lint/coverage fixes were made**

```bash
git add -p
git commit -m "chore(config): fix lint and coverage gaps"
```

**Step 6: Build**

```bash
make build
```

Verify `./devenv config --help` shows all subcommands.

---

## Summary of Commits

By end of this plan, the branch should have these commits (on top of the design doc commits):

1. `chore: swap BurntSushi/toml for pelletier/go-toml v1`
2. `feat(config): add validator/v10 and Validate() method`
3. `feat(config): add Redact() helper and secret struct tags`
4. `feat(config): add Path() and IsValidKeyPath()`
5. `feat(config): add SetKey() with comment preservation`
6. `feat(config): add DeleteSection()`
7. `feat(config): implement config show subcommand`
8. `feat(config): implement config get subcommand`
9. `feat(config): implement config set subcommand`
10. `feat(do): add SSH key and region listing for config init`
11. `feat(config): implement config init wizard`
12. `feat(config): implement profile subcommands`
13. `chore(config): fix lint and coverage gaps` *(if needed)*
