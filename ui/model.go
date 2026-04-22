package ui

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/zlliang/chore/config"
	"github.com/zlliang/chore/internal/buildinfo"
	"github.com/zlliang/chore/runner"
)

const maxOutputLines = 20

type taskStatus int

const (
	statusPending taskStatus = iota
	statusRunning
	statusCompleted
	statusFailed
	statusSkipped
)

type taskState struct {
	name        string
	description string
	status      taskStatus
	output      []string
	errorMsg    string
	duration    time.Duration
}

type Model struct {
	rootTask string
	rootDesc string
	tasks    []*taskState
	events   <-chan runner.Event
	spinner  spinner.Model
	done     bool
	quitting bool

	inputWriter io.WriteCloser
	inputTask   string
	inputBuf    string

	// Summary counts.
	completed int
	failed    int
	skipped   int
	total     time.Duration
}

func NewModel(plan *config.Plan, events <-chan runner.Event) Model {
	tasks := make([]*taskState, len(plan.Tasks))
	for i := range plan.Tasks {
		tasks[i] = &taskState{
			name:        plan.Tasks[i].Name,
			description: plan.Tasks[i].Description,
		}
	}

	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"◐", "◓", "◑", "◒"},
		FPS:    time.Second / 6,
	}
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))

	return Model{
		rootTask: plan.RootName,
		rootDesc: plan.RootDesc,
		tasks:    tasks,
		events:   events,
		spinner:  s,
	}
}

type eventMsg struct{ event runner.Event }

func (m Model) waitForEvent() tea.Msg {
	event, ok := <-m.events
	if !ok {
		return nil
	}
	return eventMsg{event}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.waitForEvent)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.inputWriter != nil {
			switch msg.Type {
			case tea.KeyCtrlC:
				m.inputWriter.Close()
				m.inputWriter = nil
				m.inputTask = ""
				m.inputBuf = ""
				return m, nil
			case tea.KeyEnter:
				m.inputWriter.Write([]byte(m.inputBuf + "\n"))
				ts := m.findTask(m.inputTask)
				if ts != nil && len(ts.output) > 0 {
					ts.output[len(ts.output)-1] += m.inputBuf
				}
				m.inputBuf = ""
				return m, nil
			case tea.KeyBackspace:
				if len(m.inputBuf) > 0 {
					m.inputBuf = m.inputBuf[:len(m.inputBuf)-1]
				}
				return m, nil
			default:
				if msg.Type == tea.KeyRunes {
					m.inputBuf += string(msg.Runes)
				} else if msg.Type == tea.KeySpace {
					m.inputBuf += " "
				}
				return m, nil
			}
		}

		if msg.String() == "ctrl+c" || msg.String() == "q" {
			m.quitting = true
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case eventMsg:
		m.handleEvent(msg.event)

		if _, ok := msg.event.(runner.EventRunDone); ok {
			m.done = true
			return m, tea.Quit
		}

		return m, m.waitForEvent
	}

	return m, nil
}

func (m *Model) handleEvent(event runner.Event) {
	switch e := event.(type) {
	case runner.EventTaskStarted:
		if ts := m.findTask(e.Task); ts != nil {
			ts.status = statusRunning
		}

	case runner.EventTaskInteractive:
		if ts := m.findTask(e.Task); ts != nil {
			ts.status = statusRunning
		}
		m.inputWriter = e.Stdin
		m.inputTask = e.Task
		m.inputBuf = ""

	case runner.EventTaskOutput:
		if ts := m.findTask(e.Task); ts != nil {
			ts.output = append(ts.output, e.Text)
			if len(ts.output) > maxOutputLines {
				ts.output = ts.output[len(ts.output)-maxOutputLines:]
			}
		}

	case runner.EventTaskCompleted:
		if ts := m.findTask(e.Task); ts != nil {
			ts.status = statusCompleted
			ts.duration = e.Duration
			ts.output = nil
			m.completed++
			m.total += e.Duration
		}
		if m.inputWriter != nil && m.inputTask == e.Task {
			m.inputWriter = nil
			m.inputTask = ""
			m.inputBuf = ""
		}

	case runner.EventTaskFailed:
		if ts := m.findTask(e.Task); ts != nil {
			ts.status = statusFailed
			ts.duration = e.Duration
			if e.Err != nil {
				ts.errorMsg = e.Err.Error()
			}
			m.failed++
			m.total += e.Duration
		}
		if m.inputWriter != nil && m.inputTask == e.Task {
			m.inputWriter = nil
			m.inputTask = ""
			m.inputBuf = ""
		}

	case runner.EventTaskSkipped:
		if ts := m.findTask(e.Task); ts != nil {
			ts.status = statusSkipped
			m.skipped++
		}

	case runner.EventRunDone:
		// handled in Update
	}
}

func (m *Model) findTask(name string) *taskState {
	for _, t := range m.tasks {
		if t.name == name {
			return t
		}
	}
	return nil
}

func (m Model) View() string {
	var b strings.Builder

	// Header.
	fmt.Fprintf(&b, "%s %s\n", styleHeader.Render("Chore"), styleVersion.Render("v"+buildinfo.Version))
	fmt.Fprintf(&b, "└ %s: %s\n\n", m.rootTask, m.rootDesc)

	// Task list.
	for _, t := range m.tasks {
		b.WriteString(m.renderTask(t))
	}

	// Summary (only when done).
	if m.done {
		b.WriteString(m.renderSummary())
	}

	return b.String()
}

func (m Model) renderTask(t *taskState) string {
	var b strings.Builder

	desc := t.description
	if desc != "" {
		desc = ": " + desc
	}

	switch t.status {
	case statusCompleted:
		dur := formatDuration(t.duration)
		fmt.Fprintf(&b, "%s %s%s %s\n",
			iconCompleted,
			styleCompleted.Render(t.name),
			desc,
			styleDim.Render(dur),
		)

	case statusFailed:
		dur := formatDuration(t.duration)
		fmt.Fprintf(&b, "%s %s%s %s\n",
			iconFailed,
			styleFailed.Render(t.name),
			desc,
			styleDim.Render(dur),
		)
		if t.errorMsg != "" {
			fmt.Fprintf(&b, "  %s\n", styleErrorLine.Render("└ "+t.errorMsg))
		}
		for i, line := range t.output {
			if i == 0 && t.errorMsg == "" {
				fmt.Fprintf(&b, "  └ %s\n", line)
			} else {
				fmt.Fprintf(&b, "    %s\n", line)
			}
		}

	case statusRunning:
		fmt.Fprintf(&b, "%s %s%s\n",
			m.spinner.View(),
			styleRunning.Render(t.name),
			desc,
		)
		for i, line := range t.output {
			display := line
			if i == len(t.output)-1 && t.name == m.inputTask {
				display = line + m.inputBuf + m.cursor()
			}
			if i == 0 {
				fmt.Fprintf(&b, "  └ %s\n", display)
			} else {
				fmt.Fprintf(&b, "    %s\n", display)
			}
		}

	case statusSkipped:
		fmt.Fprintf(&b, "%s %s%s %s\n",
			iconSkipped,
			styleSkipped.Render(t.name),
			desc,
			styleSkipped.Render("(skipped)"),
		)

	case statusPending:
		fmt.Fprintf(&b, "%s %s%s\n",
			iconPending,
			stylePending.Render(t.name),
			desc,
		)
	}

	return b.String()
}

func (m Model) renderSummary() string {
	var parts []string
	if m.completed > 0 {
		parts = append(parts, styleCompleted.Render(fmt.Sprintf("%d passed", m.completed)))
	}
	if m.failed > 0 {
		parts = append(parts, styleFailed.Render(fmt.Sprintf("%d failed", m.failed)))
	}
	if m.skipped > 0 {
		parts = append(parts, styleSkipped.Render(fmt.Sprintf("%d skipped", m.skipped)))
	}
	summary := strings.Join(parts, ", ")
	dur := formatDuration(m.total)

	return styleSummary.Render(fmt.Sprintf("%s in %s", summary, dur)) + "\n"
}

func (m Model) cursor() string {
	return "█"
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
