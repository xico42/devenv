package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/xico42/devenv/internal/config"
)

var (
	cfgFile string
	token   string
	noColor bool
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "devenv",
	Short: "Manage ephemeral Digital Ocean dev droplets",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		cfg.ApplyEnv()
		cfg.ApplyFlags(token)
		return nil
	},
}

func init() {
	rootCmd.SilenceErrors = true
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.config/devenv/config.toml)")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "Digital Ocean API token (overrides config and DIGITALOCEAN_TOKEN)")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
}

// Execute runs the root command and returns any error.
func Execute() error {
	resetAllFlags(rootCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return fmt.Errorf("%w", err)
	}
	return nil
}

// resetAllFlags resets the Changed state of all flags in the command tree so
// that Execute() can be called multiple times (e.g. in tests) without flags
// from a previous call leaking into the next.
func resetAllFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	cmd.PersistentFlags().VisitAll(func(f *pflag.Flag) { f.Changed = false })
	for _, sub := range cmd.Commands() {
		resetAllFlags(sub)
	}
}
