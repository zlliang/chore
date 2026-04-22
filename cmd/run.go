package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/zlliang/chore/config"
	"github.com/zlliang/chore/runner"
	"github.com/zlliang/chore/ui"
)

func runTask(cmd *cobra.Command, args []string) error {
	taskName := args[0]

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	plan, err := cfg.Plan(taskName)
	if err != nil {
		return err
	}

	if dryRun {
		printPlan(os.Stderr, plan)
		return nil
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	r := runner.New(plan)
	go r.Run(ctx)

	if jsonOutput {
		return runJSON(os.Stdout, r.Events())
	}

	model := ui.NewModel(plan, r.Events())
	p := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(os.Stderr))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}

var (
	dryBold = lipgloss.NewStyle().Bold(true)
	dryDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func printPlan(w io.Writer, plan *config.Plan) {
	desc := plan.RootDesc
	if desc != "" {
		desc = ": " + desc
	}
	fmt.Fprintf(w, "%s%s\n", dryBold.Render(plan.RootName), desc)

	for i, t := range plan.Tasks {
		last := i == len(plan.Tasks)-1
		connector := "├─"
		prefix := "│  "
		if last {
			connector = "└─"
			prefix = "   "
		}

		td := t.Description
		if td != "" {
			td = ": " + td
		}
		fmt.Fprintf(w, "%s %s%s\n", connector, dryBold.Render(t.Name), td)

		var details []string
		if t.Check != "" {
			details = append(details, dryDim.Render("check: "+t.Check))
		}
		if t.Interactive {
			details = append(details, dryDim.Render("interactive"))
		}
		for _, cmd := range t.Run {
			details = append(details, dryDim.Render("$ "+cmd))
		}
		for j, d := range details {
			dlast := j == len(details)-1
			dc := "├─"
			if dlast {
				dc = "└─"
			}
			fmt.Fprintf(w, "%s%s %s\n", prefix, dc, d)
		}
	}

	fmt.Fprintf(w, "\n%s\n", dryDim.Render(fmt.Sprintf("shell: %s", strings.Join(plan.Shell, " "))))
}

func runJSON(w io.Writer, events <-chan runner.Event) error {
	enc := json.NewEncoder(w)
	hasFailed := false

	for event := range events {
		var obj map[string]any

		switch e := event.(type) {
		case runner.EventTaskStarted:
			obj = map[string]any{"type": "started", "task": e.Task}
		case runner.EventTaskSkipped:
			obj = map[string]any{"type": "skipped", "task": e.Task}
		case runner.EventTaskOutput:
			obj = map[string]any{"type": "output", "task": e.Task, "text": e.Text}
		case runner.EventTaskInteractive:
			obj = map[string]any{"type": "interactive", "task": e.Task}
		case runner.EventTaskCompleted:
			obj = map[string]any{"type": "completed", "task": e.Task, "duration_ms": e.Duration.Milliseconds()}
		case runner.EventTaskFailed:
			hasFailed = true
			o := map[string]any{"type": "failed", "task": e.Task, "duration_ms": e.Duration.Milliseconds()}
			if e.Err != nil {
				o["error"] = e.Err.Error()
			}
			obj = o
		case runner.EventRunDone:
			obj = map[string]any{"type": "done"}
		}

		if obj != nil {
			if err := enc.Encode(obj); err != nil {
				return fmt.Errorf("json encode: %w", err)
			}
		}
	}

	if hasFailed {
		return fmt.Errorf("one or more tasks failed")
	}
	return nil
}
