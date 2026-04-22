package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/zlliang/chore/config"
	"github.com/zlliang/chore/internal/buildinfo"
)

var (
	cfgPath    string
	dryRun     bool
	jsonOutput bool
)

var (
	styleError = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	styleInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

var errNoConfig = errors.New("no config")

var rootCmd = &cobra.Command{
	Use:   "chore [task]",
	Short: "A task runner for repetitive daily chores",
	Args:  cobra.ExactArgs(1),
	RunE:  runTask,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgPath, "config", "c", "", "config file (default: ~/.config/chore/config.toml)")
	rootCmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "print the resolved task plan without executing")
	rootCmd.Flags().BoolVar(&jsonOutput, "json", false, "output events as newline-delimited JSON")

	styleBold := lipgloss.NewStyle().Bold(true)
	styleGray := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	versionLine := styleBold.Render("Chore") + " " + styleGray.Render("v"+buildinfo.Version)

	helpTmpl := versionLine + `{{with .Short}} - {{.}}{{end}}

Usage:
  {{.UseLine}}{{if .HasAvailableSubCommands}}

Commands:
{{range .Commands}}{{if .IsAvailableCommand}}  {{rpad .Name .NamePadding}}  {{.Short}}
{{end}}{{end}}{{end}}{{if .HasAvailableFlags}}
Flags:
{{.Flags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`
	rootCmd.SetHelpTemplate(helpTmpl)
	rootCmd.CompletionOptions.HiddenDefaultCmd = true
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if errors.Is(err, errNoConfig) {
			fmt.Fprintln(os.Stderr, styleInfo.Render("No config file found."))
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Create one at ~/.config/chore/config.toml or specify with --config.")
			fmt.Fprintln(os.Stderr, "See https://github.com/zlliang/chore for the config format.")
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, styleError.Render("Error: "+err.Error()))
		fmt.Fprintln(os.Stderr)
		rootCmd.Usage()
		os.Exit(1)
	}
}

func loadConfig() (*config.Config, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		if isNotExist(err) {
			return nil, errNoConfig
		}
		return nil, fmt.Errorf("failed to load config: %w", err)
	}
	return cfg, nil
}

func isNotExist(err error) bool {
	var pathErr *os.PathError
	return errors.As(err, &pathErr) && errors.Is(pathErr.Err, os.ErrNotExist)
}
