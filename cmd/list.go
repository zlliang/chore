package cmd

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available tasks",
	Args:  cobra.NoArgs,
	RunE:  listTasks,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

var (
	listBold = lipgloss.NewStyle().Bold(true)
	listDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func listTasks(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	w := cmd.ErrOrStderr()
	for _, t := range cfg.Tasks {
		kind := "composite"
		if t.IsLeaf() {
			kind = "leaf"
		}

		desc := t.Description
		if desc != "" {
			desc = " — " + desc
		}

		fmt.Fprintf(w, "%s %s%s\n",
			listBold.Render(t.Name),
			listDim.Render("("+kind+")"),
			desc,
		)
	}

	return nil
}
