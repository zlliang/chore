package ui

import (
	"fmt"
	"os"
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

	inputPTY  *os.File
	inputTask string

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
		if m.inputPTY != nil {
			if b, ok := encodeKey(msg); ok {
				m.inputPTY.Write(b)
			}
			return m, nil
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
		m.inputPTY = e.PTY
		m.inputTask = e.Task

	case runner.EventTaskOutput:
		if ts := m.findTask(e.Task); ts != nil {
			if e.Replace && len(ts.output) > 0 {
				ts.output[len(ts.output)-1] = e.Text
			} else {
				ts.output = append(ts.output, e.Text)
			}
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
		if m.inputPTY != nil && m.inputTask == e.Task {
			m.inputPTY = nil
			m.inputTask = ""
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
		if m.inputPTY != nil && m.inputTask == e.Task {
			m.inputPTY = nil
			m.inputTask = ""
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
				display = line + m.cursor()
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

// encodeKey translates a bubbletea key message into terminal bytes.
func encodeKey(msg tea.KeyMsg) ([]byte, bool) {
	switch msg.Type {
	case tea.KeyRunes:
		return []byte(string(msg.Runes)), true
	case tea.KeySpace:
		return []byte(" "), true
	case tea.KeyEnter:
		return []byte{'\r'}, true
	case tea.KeyTab:
		return []byte{'\t'}, true
	case tea.KeyEscape:
		return []byte{0x1b}, true
	case tea.KeyBackspace:
		return []byte{0x7f}, true
	case tea.KeyCtrlC:
		return []byte{0x03}, true
	case tea.KeyCtrlD:
		return []byte{0x04}, true
	case tea.KeyCtrlZ:
		return []byte{0x1a}, true
	case tea.KeyUp:
		return []byte("\x1b[A"), true
	case tea.KeyDown:
		return []byte("\x1b[B"), true
	case tea.KeyRight:
		return []byte("\x1b[C"), true
	case tea.KeyLeft:
		return []byte("\x1b[D"), true
	case tea.KeyHome:
		return []byte("\x1b[H"), true
	case tea.KeyEnd:
		return []byte("\x1b[F"), true
	case tea.KeyDelete:
		return []byte("\x1b[3~"), true
	default:
		return nil, false
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
