package runner

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/zlliang/chore/config"
)

// LookPathFunc is the signature of exec.LookPath, injectable for testing.
type LookPathFunc func(string) (string, error)

// Runner executes a resolved task plan and emits events.
type Runner struct {
	plan     *config.Plan
	lookPath LookPathFunc
	events   chan Event
}

// New creates a runner for the given plan.
func New(plan *config.Plan) *Runner {
	return &Runner{
		plan:     plan,
		lookPath: exec.LookPath,
		events:   make(chan Event, 64),
	}
}

// Events returns the channel that receives execution events.
func (r *Runner) Events() <-chan Event {
	return r.events
}

func (r *Runner) emit(e Event) {
	r.events <- e
}

// Run executes all tasks in the plan sequentially.
func (r *Runner) Run(ctx context.Context) {
	defer func() {
		r.emit(EventRunDone{})
		close(r.events)
	}()

	for i := range r.plan.Tasks {
		if ctx.Err() != nil {
			return
		}
		r.runTask(ctx, &r.plan.Tasks[i])
	}
}

func (r *Runner) runTask(ctx context.Context, task *config.Task) {
	if r.shouldSkip(task) {
		r.emit(EventTaskSkipped{Task: task.Name})
		return
	}

	r.emit(EventTaskStarted{Task: task.Name})
	start := time.Now()

	err := r.runCommands(ctx, task)

	if err != nil {
		r.emit(EventTaskFailed{
			Task:     task.Name,
			Err:      err,
			Duration: time.Since(start),
		})
		return
	}

	r.emit(EventTaskCompleted{
		Task:     task.Name,
		Duration: time.Since(start),
	})
}

func (r *Runner) shouldSkip(task *config.Task) bool {
	if task.Check == "" {
		return false
	}
	_, err := r.lookPath(task.Check)
	return err != nil
}

func (r *Runner) runCommands(ctx context.Context, task *config.Task) error {
	for _, cmdStr := range task.Run {
		if err := ctx.Err(); err != nil {
			return err
		}
		var execErr error
		if task.Interactive {
			execErr = r.execInteractive(ctx, task.Name, cmdStr)
		} else {
			execErr = r.execCommand(ctx, task.Name, cmdStr)
		}
		if execErr != nil {
			return execErr
		}
	}
	return nil
}

func (r *Runner) shellArgs(cmdStr string) []string {
	args := make([]string, len(r.plan.Shell)-1, len(r.plan.Shell))
	copy(args, r.plan.Shell[1:])
	return append(args, cmdStr)
}

// newCommand creates an exec.Cmd with its own process group so signals
// can be forwarded to the entire group (child + its descendants).
func (r *Runner) newCommand(ctx context.Context, cmdStr string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, r.plan.Shell[0], r.shellArgs(cmdStr)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 3 * time.Second
	return cmd
}

func (r *Runner) execInteractive(ctx context.Context, taskName, cmdStr string) error {
	cmd := exec.CommandContext(ctx, r.plan.Shell[0], r.shellArgs(cmdStr)...)
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 3 * time.Second

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	defer ptmx.Close()

	r.emit(EventTaskInteractive{
		Task: taskName,
		PTY:  ptmx,
	})

	r.streamOutput(taskName, ptmx)

	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func (r *Runner) streamOutput(taskName string, reader io.Reader) {
	buf := make([]byte, 256)
	var partial string
	var partialEmitted bool
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data := string(buf[:n])
			partial, partialEmitted = r.streamOutputChunk(taskName, partial+data, partialEmitted)
		}
		if err != nil {
			if partial != "" && !partialEmitted {
				r.emitOutput(taskName, partial, false)
			}
			break
		}
	}
}

func (r *Runner) streamOutputChunk(taskName, data string, partialEmitted bool) (string, bool) {
	partial := data
	for {
		i := strings.IndexAny(partial, "\r\n")
		if i < 0 {
			break
		}

		line := partial[:i]
		sep := partial[i]
		partial = partial[i+1:]

		if line != "" {
			r.emitOutput(taskName, line, partialEmitted)
		}
		partialEmitted = sep == '\r' && line != ""

		if sep == '\n' {
			partialEmitted = false
		}
	}

	if partial != "" {
		r.emitOutput(taskName, partial, partialEmitted)
		partialEmitted = true
	}

	return partial, partialEmitted
}

func (r *Runner) emitOutput(taskName, text string, replace bool) {
	text = stripTerminalControls(text)
	if text == "" {
		return
	}
	r.emit(EventTaskOutput{Task: taskName, Text: text, Replace: replace})
}

// stripTerminalControls removes terminal control sequences that would interfere
// with Chore's own TUI while preserving SGR color/style sequences.
func stripTerminalControls(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) {
			switch s[i+1] {
			case '[':
				// CSI sequence.
				j := i + 2
				for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
					j++ // parameter bytes
				}
				for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
					j++ // intermediate bytes
				}
				if j < len(s) {
					final := s[j]
					j++
					if final == 'm' {
						b.WriteString(s[i:j])
					}
					i = j
					continue
				}
				return b.String()

			case ']':
				// OSC sequence, terminated by BEL or ST.
				j := i + 2
				for j < len(s) {
					if s[j] == '\a' {
						j++
						break
					}
					if s[j] == '\x1b' && j+1 < len(s) && s[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				i = j
				continue

			default:
				// Other escape sequences such as ESC 7, ESC 8, ESC >, ESC <.
				i += 2
				continue
			}
		}

		if s[i] < 0x20 && s[i] != '\t' {
			i++
			continue
		}

		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// stripCursorSequences is kept as a small compatibility wrapper for tests and
// callers inside this package that need the old name.
func stripCursorSequences(s string) string {
	return stripTerminalControls(s)
}

func (r *Runner) execCommand(ctx context.Context, taskName, cmdStr string) error {
	cmd := exec.CommandContext(ctx, r.plan.Shell[0], r.shellArgs(cmdStr)...)
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 3 * time.Second

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	defer ptmx.Close()

	r.streamOutput(taskName, ptmx)

	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}
