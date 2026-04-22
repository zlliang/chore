package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zlliang/chore/config"
	"github.com/zlliang/chore/runner"
)

func TestRunJSON(t *testing.T) {
	t.Run("all event types", func(t *testing.T) {
		ch := make(chan runner.Event, 7)
		ch <- runner.EventTaskStarted{Task: "build"}
		ch <- runner.EventTaskSkipped{Task: "lint"}
		ch <- runner.EventTaskOutput{Task: "build", Text: "compiling..."}
		ch <- runner.EventTaskInteractive{Task: "deploy"}
		ch <- runner.EventTaskCompleted{Task: "build", Duration: 1500 * time.Millisecond}
		ch <- runner.EventTaskFailed{Task: "test", Err: errors.New("exit 1"), Duration: 3200 * time.Millisecond}
		ch <- runner.EventRunDone{}
		close(ch)

		var buf bytes.Buffer
		err := runJSON(&buf, ch)
		if err == nil {
			t.Fatal("expected error for failed task, got nil")
		}
		if !strings.Contains(err.Error(), "failed") {
			t.Fatalf("expected failure error, got: %v", err)
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 7 {
			t.Fatalf("expected 7 lines, got %d: %v", len(lines), lines)
		}

		type obj = map[string]any

		cases := []struct {
			index  int
			checks func(t *testing.T, m obj)
		}{
			{0, func(t *testing.T, m obj) {
				expectField(t, m, "type", "started")
				expectField(t, m, "task", "build")
			}},
			{1, func(t *testing.T, m obj) {
				expectField(t, m, "type", "skipped")
				expectField(t, m, "task", "lint")
			}},
			{2, func(t *testing.T, m obj) {
				expectField(t, m, "type", "output")
				expectField(t, m, "task", "build")
				expectField(t, m, "text", "compiling...")
			}},
			{3, func(t *testing.T, m obj) {
				expectField(t, m, "type", "interactive")
				expectField(t, m, "task", "deploy")
			}},
			{4, func(t *testing.T, m obj) {
				expectField(t, m, "type", "completed")
				expectField(t, m, "task", "build")
				expectFieldFloat(t, m, "duration_ms", 1500)
			}},
			{5, func(t *testing.T, m obj) {
				expectField(t, m, "type", "failed")
				expectField(t, m, "task", "test")
				expectFieldFloat(t, m, "duration_ms", 3200)
				expectField(t, m, "error", "exit 1")
			}},
			{6, func(t *testing.T, m obj) {
				expectField(t, m, "type", "done")
			}},
		}

		for _, tc := range cases {
			var m obj
			if err := json.Unmarshal([]byte(lines[tc.index]), &m); err != nil {
				t.Fatalf("line %d: invalid JSON: %v", tc.index, err)
			}
			tc.checks(t, m)
		}
	})

	t.Run("no failures returns nil", func(t *testing.T) {
		ch := make(chan runner.Event, 3)
		ch <- runner.EventTaskStarted{Task: "build"}
		ch <- runner.EventTaskCompleted{Task: "build", Duration: 100 * time.Millisecond}
		ch <- runner.EventRunDone{}
		close(ch)

		var buf bytes.Buffer
		if err := runJSON(&buf, ch); err != nil {
			t.Fatalf("expected nil error, got: %v", err)
		}
	})

	t.Run("failed task without error field", func(t *testing.T) {
		ch := make(chan runner.Event, 2)
		ch <- runner.EventTaskFailed{Task: "x", Err: nil, Duration: 50 * time.Millisecond}
		ch <- runner.EventRunDone{}
		close(ch)

		var buf bytes.Buffer
		err := runJSON(&buf, ch)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		var m map[string]any
		if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if _, ok := m["error"]; ok {
			t.Fatal("expected no 'error' field when Err is nil")
		}
		expectFieldFloat(t, m, "duration_ms", 50)
	})
}

func TestPrintPlan(t *testing.T) {
	plan := &config.Plan{
		RootName: "deploy",
		RootDesc: "Deploy all services",
		Shell:    []string{"bash", "-c"},
		Tasks: []config.Task{
			{
				Name:        "lint",
				Description: "Run linter",
				Run:         []string{"eslint ."},
				Check:       "which eslint",
			},
			{
				Name:        "setup",
				Description: "",
				Run:         []string{"npm install", "npm run build"},
				Interactive: true,
			},
			{
				Name: "push",
				Run:  []string{"git push"},
			},
		},
	}

	var buf bytes.Buffer
	printPlan(&buf, plan)
	out := buf.String()

	expected := []string{
		"deploy",
		"Deploy all services",
		"lint",
		"Run linter",
		"check: which eslint",
		"$ eslint .",
		"setup",
		"interactive",
		"$ npm install",
		"$ npm run build",
		"push",
		"$ git push",
		"├─",
		"└─",
		"bash -c",
	}

	for _, s := range expected {
		if !strings.Contains(out, s) {
			t.Errorf("output missing expected substring %q\noutput:\n%s", s, out)
		}
	}
}

func TestIsNotExist(t *testing.T) {
	t.Run("real PathError with ErrNotExist", func(t *testing.T) {
		_, err := os.Open("/nonexistent/path/that/does/not/exist")
		if !isNotExist(err) {
			t.Errorf("expected isNotExist to return true for os.Open error, got false")
		}
	})

	t.Run("wrapped PathError", func(t *testing.T) {
		_, err := os.Open("/nonexistent/path/that/does/not/exist")
		wrapped := fmt.Errorf("loading config: %w", err)
		if !isNotExist(wrapped) {
			t.Errorf("expected isNotExist to return true for wrapped PathError, got false")
		}
	})

	t.Run("non-PathError", func(t *testing.T) {
		err := errors.New("something else")
		if isNotExist(err) {
			t.Errorf("expected isNotExist to return false for non-PathError, got true")
		}
	})

	t.Run("nil error", func(t *testing.T) {
		if isNotExist(nil) {
			t.Errorf("expected isNotExist to return false for nil, got true")
		}
	})
}

func expectField(t *testing.T, m map[string]any, key, want string) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("missing field %q", key)
		return
	}
	if got != want {
		t.Errorf("field %q = %v, want %v", key, got, want)
	}
}

func expectFieldFloat(t *testing.T, m map[string]any, key string, want float64) {
	t.Helper()
	got, ok := m[key]
	if !ok {
		t.Errorf("missing field %q", key)
		return
	}
	f, ok := got.(float64)
	if !ok {
		t.Errorf("field %q is not a number: %v (%T)", key, got, got)
		return
	}
	if f != want {
		t.Errorf("field %q = %v, want %v", key, f, want)
	}
}
