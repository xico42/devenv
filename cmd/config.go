package cmd

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"text/tabwriter"

	"charm.land/huh/v2"
	"github.com/digitalocean/godo"
	"github.com/spf13/cobra"

	"github.com/xico42/devenv/internal/config"
	"github.com/xico42/devenv/internal/do"
)

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

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Manage devenv configuration",
	GroupID: "config",
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

		return w.Flush()
	},
}

// ── set ──────────────────────────────────────────────────────────────────────

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, val := args[0], args[1]
		if err := cfg.SetKey(key, val); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Set %s = %q\n", key, val)
		return nil
	},
}

// ── get ──────────────────────────────────────────────────────────────────────

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
			return "", false, fmt.Errorf("get profile %q: %w", parts[1], err)
		}
		return fieldFromStruct(p, parts[2])
	case "projects":
		proj, ok := c.Projects[parts[1]]
		if !ok {
			return "", false, fmt.Errorf("project %q not found", parts[1])
		}
		return fieldFromStruct(proj, parts[2])
	}
	return "", false, fmt.Errorf("unknown key %q", dotPath)
}

// fieldFromStruct returns the string value and secret status of the toml-tagged field in v.
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

// ── profile ───────────────────────────────────────────────────────────────────

var configProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage named profiles",
}

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
			return fmt.Errorf("profile %q: %w", args[0], err)
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
			return fmt.Errorf("confirm delete: %w", err)
		}
		if !confirm {
			return nil
		}
		if err := cfg.DeleteSection("profiles." + name); err != nil {
			return fmt.Errorf("delete profile %q: %w", name, err)
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
			return fmt.Errorf("profile create form: %w", err)
		}

		if err := cfg.SetKey("profiles."+name+".size", size); err != nil {
			return fmt.Errorf("set profile size: %w", err)
		}
		if region != "" && region != defaultRegion {
			if err := cfg.SetKey("profiles."+name+".region", region); err != nil {
				return fmt.Errorf("set profile region: %w", err)
			}
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Profile %q created\nUse it with: devenv up --profile %s\n", name, name)
		return nil
	},
}

// ── init ─────────────────────────────────────────────────────────────────────

// initAPIClient is the interface config init uses to fetch SSH keys and regions.
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
	// If config already has values, confirm overwrite.
	if cfg.Defaults.Token != "" || cfg.Defaults.Region != "" {
		var overwrite bool
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("Config already exists. Overwrite?").
				Value(&overwrite),
		)).Run(); err != nil {
			return fmt.Errorf("confirm overwrite: %w", err)
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
		return fmt.Errorf("get token: %w", err)
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
		return fmt.Errorf("config init form: %w", err)
	}

	// Build and save config.
	if err := applyInitValues(cfg.Path(), token, sshKeyID, region, size, tsKey, projectsDir); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nConfig written to %s\n", cfg.Path())
	return nil
}

// applyInitValues writes the collected wizard values to a fresh Config and saves it.
// Extracted from runConfigInit to enable testing without a TTY.
func applyInitValues(path, token, sshKeyID, region, size, tsKey, projectsDir string) error {
	freshCfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	freshCfg.Defaults.Token = token
	freshCfg.Defaults.SSHKeyID = strings.TrimSpace(sshKeyID)
	freshCfg.Defaults.Region = region
	freshCfg.Defaults.Size = size
	freshCfg.Defaults.TailscaleAuthKey = tsKey
	freshCfg.Defaults.ProjectsDir = projectsDir
	if err := freshCfg.Save(); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}
