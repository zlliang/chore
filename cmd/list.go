package cmd

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/zlliang/chore/config"
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

	// Collect names that appear as steps of a composite task.
	children := make(map[string]bool)
	for _, t := range cfg.Tasks {
		for _, step := range t.Steps {
			children[step] = true
		}
	}

	w := cmd.ErrOrStderr()
	for _, t := range cfg.Tasks {
		// Skip tasks that are children of a composite; they'll be printed inline.
		if children[t.Name] {
			continue
		}
		printTask(w, cfg, &t, "", "")
	}

	return nil
}

func printTask(w io.Writer, cfg *config.Config, t *config.Task, connector, prefix string) {
	kind := "composite"
	if t.IsLeaf() {
		kind = "leaf"
	}

	desc := t.Description
	if desc != "" {
		desc = " — " + desc
	}

	fmt.Fprintf(w, "%s%s %s%s\n",
		connector,
		listBold.Render(t.Name),
		listDim.Render("("+kind+")"),
		desc,
	)

	for i, step := range t.Steps {
		child, ok := cfg.Lookup(step)
		if !ok {
			continue
		}
		last := i == len(t.Steps)-1
		ch := listDim.Render("├ ")
		cp := prefix + listDim.Render("│ ")
		if last {
			ch = listDim.Render("└ ")
			cp = prefix + "  "
		}
		printTask(w, cfg, child, prefix+ch, cp)
	}
}
