package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"

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
	cmd := r.newCommand(ctx, cmdStr)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	r.emit(EventTaskInteractive{
		Task:  taskName,
		Stdin: stdin,
	})

	r.streamOutputInteractive(taskName, stdout)

	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func (r *Runner) streamOutputInteractive(taskName string, reader io.Reader) {
	buf := make([]byte, 256)
	var partial string
	var partialEmitted bool
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			data := string(buf[:n])
			if !partialEmitted {
				data = partial + data
			}
			partial = ""
			partialEmitted = false

			lines := strings.Split(data, "\n")
			partial = lines[len(lines)-1]
			for _, line := range lines[:len(lines)-1] {
				r.emit(EventTaskOutput{Task: taskName, Text: line})
			}
			if partial != "" {
				r.emit(EventTaskOutput{Task: taskName, Text: partial})
				partialEmitted = true
			}
		}
		if err != nil {
			if partial != "" && !partialEmitted {
				r.emit(EventTaskOutput{Task: taskName, Text: partial})
			}
			break
		}
	}
}

func (r *Runner) execCommand(ctx context.Context, taskName, cmdStr string) error {
	cmd := r.newCommand(ctx, cmdStr)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	r.streamOutput(taskName, stdout)

	if err := cmd.Wait(); err != nil {
		return err
	}
	return nil
}

func (r *Runner) streamOutput(taskName string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		r.emit(EventTaskOutput{Task: taskName, Text: scanner.Text()})
	}
}
