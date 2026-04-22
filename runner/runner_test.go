package runner

import (
	"context"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/zlliang/chore/config"
)

func drainEvents(ch <-chan Event) []Event {
	var events []Event
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func makePlan(tasks ...config.Task) *config.Plan {
	return &config.Plan{
		RootName: "test",
		Shell:    []string{"sh", "-c"},
		Tasks:    tasks,
	}
}

func TestSimpleTaskExecution(t *testing.T) {
	plan := makePlan(config.Task{
		Name: "greet",
		Run:  []string{"echo hello"},
	})
	r := New(plan)

	go r.Run(context.Background())
	events := drainEvents(r.Events())

	want := []string{"started:greet", "output:greet:hello", "completed:greet", "done"}
	got := summarize(events)
	if len(got) != len(want) {
		t.Fatalf("expected %d events, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestMultipleTasks(t *testing.T) {
	plan := makePlan(
		config.Task{Name: "first", Run: []string{"echo one"}},
		config.Task{Name: "second", Run: []string{"echo two"}},
	)
	r := New(plan)

	go r.Run(context.Background())
	events := drainEvents(r.Events())

	want := []string{
		"started:first", "output:first:one", "completed:first",
		"started:second", "output:second:two", "completed:second",
		"done",
	}
	got := summarize(events)
	if len(got) != len(want) {
		t.Fatalf("expected %d events, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestFailedTask(t *testing.T) {
	plan := makePlan(
		config.Task{Name: "fail", Run: []string{"exit 1"}},
		config.Task{Name: "after", Run: []string{"echo after"}},
	)
	r := New(plan)

	go r.Run(context.Background())
	events := drainEvents(r.Events())

	got := summarize(events)
	want := []string{
		"started:fail", "failed:fail",
		"started:after", "output:after:after", "completed:after",
		"done",
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d events, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestSkipLogic(t *testing.T) {
	plan := makePlan(config.Task{
		Name:  "skippable",
		Run:   []string{"echo should not run"},
		Check: "nonexistent_binary_xyz",
	})
	r := New(plan)
	r.lookPath = func(name string) (string, error) {
		return "", fmt.Errorf("not found: %s", name)
	}

	go r.Run(context.Background())
	events := drainEvents(r.Events())

	got := summarize(events)
	want := []string{"skipped:skippable", "done"}
	if len(got) != len(want) {
		t.Fatalf("expected %d events, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}

func TestContextCancellation(t *testing.T) {
	plan := makePlan(
		config.Task{Name: "fast", Run: []string{"echo fast"}},
		config.Task{Name: "never", Run: []string{"echo never"}},
	)
	r := New(plan)

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel as soon as the first task completes.
	go func() {
		for e := range r.Events() {
			if _, ok := e.(EventTaskCompleted); ok {
				cancel()
				// Keep draining so the channel doesn't block.
			}
		}
	}()

	r.Run(ctx)

	// If we reach here, Run returned — context cancellation worked.
	// The second task should either not start or fail due to context.
	cancel() // no-op if already cancelled
}

func TestShellArgs(t *testing.T) {
	plan := &config.Plan{
		Shell: []string{"sh", "-c"},
	}
	r := New(plan)

	args := r.shellArgs("echo hello")
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "-c" {
		t.Errorf("args[0]: want %q, got %q", "-c", args[0])
	}
	if args[1] != "echo hello" {
		t.Errorf("args[1]: want %q, got %q", "echo hello", args[1])
	}

	// Verify no aliasing: mutating the result must not affect the plan.
	args[0] = "MUTATED"
	if plan.Shell[1] == "MUTATED" {
		t.Error("shellArgs result aliases plan.Shell slice")
	}
}

func TestNewCommand(t *testing.T) {
	plan := makePlan(config.Task{
		Name: "test",
		Run:  []string{"echo test"},
	})
	r := New(plan)

	cmd := r.newCommand(context.Background(), "echo test")

	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Error("Setpgid: want true, got false")
	}
	if cmd.WaitDelay != 3*time.Second {
		t.Errorf("WaitDelay: want %v, got %v", 3*time.Second, cmd.WaitDelay)
	}
	if cmd.Cancel == nil {
		t.Error("Cancel function is nil")
	}
	if cmd.Path == "" {
		t.Fatal("Path is empty")
	}
	// cmd.Args[0] is the command name, followed by shell flags and the command string.
	wantArgs := []string{"sh", "-c", "echo test"}
	if len(cmd.Args) != len(wantArgs) {
		t.Fatalf("Args: want %v, got %v", wantArgs, cmd.Args)
	}
	for i, w := range wantArgs {
		if cmd.Args[i] != w {
			t.Errorf("Args[%d]: want %q, got %q", i, w, cmd.Args[i])
		}
	}

	_ = syscall.SIGTERM // ensure syscall import is used
}

func TestCommandPreservesANSIColors(t *testing.T) {
	// printf outputs ANSI escape codes; the PTY should preserve them.
	plan := makePlan(config.Task{
		Name: "colors",
		Run:  []string{`printf '\033[31mred\033[0m\n'`},
	})
	r := New(plan)

	go r.Run(context.Background())
	events := drainEvents(r.Events())

	var output string
	for _, e := range events {
		if o, ok := e.(EventTaskOutput); ok {
			output = o.Text
		}
	}
	if !strings.Contains(output, "\033[31m") {
		t.Errorf("expected ANSI escape codes in output, got %q", output)
	}
}

func TestCommandOutputNoTrailingCR(t *testing.T) {
	// PTY produces \r\n line endings; trailing \r should be stripped.
	plan := makePlan(config.Task{
		Name: "crlf",
		Run:  []string{"echo hello"},
	})
	r := New(plan)

	go r.Run(context.Background())
	events := drainEvents(r.Events())

	for _, e := range events {
		if o, ok := e.(EventTaskOutput); ok {
			if strings.HasSuffix(o.Text, "\r") {
				t.Errorf("output has trailing \\r: %q", o.Text)
			}
		}
	}
}

func TestInteractivePreservesANSIColors(t *testing.T) {
	plan := makePlan(config.Task{
		Name:        "icolors",
		Interactive: true,
		Run:         []string{`printf '\033[32mgreen\033[0m\n'`},
	})
	r := New(plan)

	go r.Run(context.Background())
	events := drainEvents(r.Events())

	var output string
	for _, e := range events {
		if o, ok := e.(EventTaskOutput); ok {
			output = o.Text
		}
	}
	if !strings.Contains(output, "\033[32m") {
		t.Errorf("expected ANSI escape codes in interactive output, got %q", output)
	}
}

func TestInteractiveOutputNoTrailingCR(t *testing.T) {
	plan := makePlan(config.Task{
		Name:        "icrlf",
		Interactive: true,
		Run:         []string{"echo hello"},
	})
	r := New(plan)

	go r.Run(context.Background())
	events := drainEvents(r.Events())

	for _, e := range events {
		if o, ok := e.(EventTaskOutput); ok {
			if strings.HasSuffix(o.Text, "\r") {
				t.Errorf("interactive output has trailing \\r: %q", o.Text)
			}
		}
	}
}

// summarize converts events into compact strings for easy comparison.
func summarize(events []Event) []string {
	var out []string
	for _, e := range events {
		switch v := e.(type) {
		case EventTaskStarted:
			out = append(out, "started:"+v.Task)
		case EventTaskSkipped:
			out = append(out, "skipped:"+v.Task)
		case EventTaskOutput:
			out = append(out, "output:"+v.Task+":"+v.Text)
		case EventTaskCompleted:
			out = append(out, "completed:"+v.Task)
		case EventTaskFailed:
			out = append(out, "failed:"+v.Task)
		case EventRunDone:
			out = append(out, "done")
		}
	}
	return out
}
