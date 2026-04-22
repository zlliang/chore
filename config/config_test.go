package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskIsComposite(t *testing.T) {
	tests := []struct {
		name string
		task Task
		want bool
	}{
		{"with steps", Task{Steps: []string{"a"}}, true},
		{"without steps", Task{Run: []string{"echo hi"}}, false},
		{"empty task", Task{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.task.IsComposite(); got != tt.want {
				t.Errorf("IsComposite() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskIsLeaf(t *testing.T) {
	tests := []struct {
		name string
		task Task
		want bool
	}{
		{"with run", Task{Run: []string{"echo hi"}}, true},
		{"without run", Task{Steps: []string{"a"}}, false},
		{"empty task", Task{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.task.IsLeaf(); got != tt.want {
				t.Errorf("IsLeaf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "valid leaf task",
			cfg: Config{
				Shell: []string{"sh", "-c"},
				Tasks: []Task{{Name: "build", Run: []string{"go build"}}},
			},
		},
		{
			name: "valid composite task",
			cfg: Config{
				Shell: []string{"sh", "-c"},
				Tasks: []Task{
					{Name: "lint", Run: []string{"golint"}},
					{Name: "all", Steps: []string{"lint"}},
				},
			},
		},
		{
			name: "empty name",
			cfg: Config{
				Tasks: []Task{{Run: []string{"echo"}}},
			},
			wantErr: "task at index 0 has no name",
		},
		{
			name: "duplicate name",
			cfg: Config{
				Tasks: []Task{
					{Name: "build", Run: []string{"go build"}},
					{Name: "build", Run: []string{"go build ./..."}},
				},
			},
			wantErr: "duplicate task name",
		},
		{
			name: "both steps and run",
			cfg: Config{
				Tasks: []Task{
					{Name: "bad", Steps: []string{"x"}, Run: []string{"echo"}},
				},
			},
			wantErr: "has both steps and run",
		},
		{
			name: "neither steps nor run",
			cfg: Config{
				Tasks: []Task{
					{Name: "empty", Description: "does nothing"},
				},
			},
			wantErr: "has neither steps nor run",
		},
		{
			name: "composite with check",
			cfg: Config{
				Tasks: []Task{
					{Name: "lint", Run: []string{"golint"}},
					{Name: "all", Steps: []string{"lint"}, Check: "true"},
				},
			},
			wantErr: "is composite but has check",
		},
		{
			name: "unknown step reference",
			cfg: Config{
				Tasks: []Task{
					{Name: "all", Steps: []string{"missing"}},
				},
			},
			wantErr: "references unknown step",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLookup(t *testing.T) {
	cfg := Config{
		Tasks: []Task{
			{Name: "build", Run: []string{"go build"}},
			{Name: "test", Run: []string{"go test"}},
		},
	}

	t.Run("found", func(t *testing.T) {
		task, ok := cfg.Lookup("build")
		if !ok {
			t.Fatal("expected to find task")
		}
		if task.Name != "build" {
			t.Errorf("got name %q, want %q", task.Name, "build")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, ok := cfg.Lookup("missing")
		if ok {
			t.Fatal("expected not found")
		}
	})
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		taskName  string
		wantNames []string
		wantErr   string
	}{
		{
			name: "leaf task",
			cfg: Config{
				Tasks: []Task{
					{Name: "build", Run: []string{"go build"}},
				},
			},
			taskName:  "build",
			wantNames: []string{"build"},
		},
		{
			name: "single-level composite",
			cfg: Config{
				Tasks: []Task{
					{Name: "lint", Run: []string{"golint"}},
					{Name: "build", Run: []string{"go build"}},
					{Name: "all", Steps: []string{"lint", "build"}},
				},
			},
			taskName:  "all",
			wantNames: []string{"lint", "build"},
		},
		{
			name: "nested composite",
			cfg: Config{
				Tasks: []Task{
					{Name: "fmt", Run: []string{"gofmt"}},
					{Name: "lint", Run: []string{"golint"}},
					{Name: "check", Steps: []string{"fmt", "lint"}},
					{Name: "build", Run: []string{"go build"}},
					{Name: "all", Steps: []string{"check", "build"}},
				},
			},
			taskName:  "all",
			wantNames: []string{"fmt", "lint", "build"},
		},
		{
			name: "cycle detection",
			cfg: Config{
				Tasks: []Task{
					{Name: "a", Steps: []string{"b"}},
					{Name: "b", Steps: []string{"a"}},
				},
			},
			taskName: "a",
			wantErr:  "cycle detected",
		},
		{
			name: "unknown task",
			cfg: Config{
				Tasks: []Task{
					{Name: "build", Run: []string{"go build"}},
				},
			},
			taskName: "missing",
			wantErr:  "unknown task",
		},
		{
			name: "step references unknown task",
			cfg: Config{
				Tasks: []Task{
					{Name: "all", Steps: []string{"missing"}},
				},
			},
			taskName: "all",
			wantErr:  "unknown task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tasks, err := tt.cfg.Resolve(tt.taskName)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(tasks) != len(tt.wantNames) {
				t.Fatalf("got %d tasks, want %d", len(tasks), len(tt.wantNames))
			}
			for i, name := range tt.wantNames {
				if tasks[i].Name != name {
					t.Errorf("task[%d] = %q, want %q", i, tasks[i].Name, name)
				}
			}
		})
	}
}

func TestPlan(t *testing.T) {
	t.Run("flattens composite task", func(t *testing.T) {
		cfg := Config{
			Shell: []string{"bash", "-c"},
			Tasks: []Task{
				{Name: "lint", Run: []string{"golint"}, Description: "run linter"},
				{Name: "build", Run: []string{"go build"}, Check: "test -f binary"},
				{Name: "all", Steps: []string{"lint", "build"}, Description: "run everything"},
			},
		}

		plan, err := cfg.Plan("all")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if plan.RootName != "all" {
			t.Errorf("RootName = %q, want %q", plan.RootName, "all")
		}
		if plan.RootDesc != "run everything" {
			t.Errorf("RootDesc = %q, want %q", plan.RootDesc, "run everything")
		}
		if len(plan.Shell) != 2 || plan.Shell[0] != "bash" || plan.Shell[1] != "-c" {
			t.Errorf("Shell = %v, want [bash -c]", plan.Shell)
		}
		if len(plan.Tasks) != 2 {
			t.Fatalf("got %d tasks, want 2", len(plan.Tasks))
		}
		if plan.Tasks[0].Name != "lint" {
			t.Errorf("Tasks[0].Name = %q, want %q", plan.Tasks[0].Name, "lint")
		}
		if plan.Tasks[1].Name != "build" {
			t.Errorf("Tasks[1].Name = %q, want %q", plan.Tasks[1].Name, "build")
		}
		if plan.Tasks[1].Check != "test -f binary" {
			t.Errorf("Tasks[1].Check = %q, want %q", plan.Tasks[1].Check, "test -f binary")
		}
	})

	t.Run("leaf task plan", func(t *testing.T) {
		cfg := Config{
			Shell: []string{"sh", "-c"},
			Tasks: []Task{
				{Name: "build", Run: []string{"go build"}, Description: "build it"},
			},
		}

		plan, err := cfg.Plan("build")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if plan.RootName != "build" {
			t.Errorf("RootName = %q, want %q", plan.RootName, "build")
		}
		if len(plan.Tasks) != 1 {
			t.Fatalf("got %d tasks, want 1", len(plan.Tasks))
		}
	})

	t.Run("shell is copied not shared", func(t *testing.T) {
		cfg := Config{
			Shell: []string{"bash", "-c"},
			Tasks: []Task{
				{Name: "build", Run: []string{"go build"}},
			},
		}

		plan, err := cfg.Plan("build")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		plan.Shell[0] = "zsh"
		if cfg.Shell[0] != "bash" {
			t.Error("Plan shell mutation affected Config shell")
		}
	})

	t.Run("unknown task", func(t *testing.T) {
		cfg := Config{
			Shell: []string{"sh", "-c"},
			Tasks: []Task{
				{Name: "build", Run: []string{"go build"}},
			},
		}

		_, err := cfg.Plan("missing")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "unknown task") {
			t.Errorf("error %q does not contain %q", err, "unknown task")
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		content := `
shell = ["bash", "-c"]

[[tasks]]
name = "build"
run = ["go build ./..."]
check = "test -f binary"

[[tasks]]
name = "all"
steps = ["build"]
description = "run all"
`
		cfg := loadFromString(t, content)
		if len(cfg.Shell) != 2 || cfg.Shell[0] != "bash" {
			t.Errorf("Shell = %v, want [bash -c]", cfg.Shell)
		}
		if len(cfg.Tasks) != 2 {
			t.Fatalf("got %d tasks, want 2", len(cfg.Tasks))
		}
	})

	t.Run("default shell", func(t *testing.T) {
		content := `
[[tasks]]
name = "build"
run = ["go build"]
`
		cfg := loadFromString(t, content)
		if len(cfg.Shell) != 2 || cfg.Shell[0] != "sh" || cfg.Shell[1] != "-c" {
			t.Errorf("Shell = %v, want [sh -c]", cfg.Shell)
		}
	})

	t.Run("invalid TOML", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(path, []byte("not valid [[[ toml"), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error for invalid TOML")
		}
		if !strings.Contains(err.Error(), "loading config") {
			t.Errorf("error %q does not contain %q", err, "loading config")
		}
	})

	t.Run("validation error propagates", func(t *testing.T) {
		content := `
[[tasks]]
name = "bad"
`
		dir := t.TempDir()
		path := filepath.Join(dir, "config.toml")
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		_, err := Load(path)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "has neither steps nor run") {
			t.Errorf("error %q does not contain expected validation message", err)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		_, err := Load("/nonexistent/path/config.toml")
		if err == nil {
			t.Fatal("expected error for nonexistent file")
		}
	})

	t.Run("interactive field", func(t *testing.T) {
		content := `
[[tasks]]
name = "deploy"
run = ["./deploy.sh"]
interactive = true
`
		cfg := loadFromString(t, content)
		if !cfg.Tasks[0].Interactive {
			t.Error("expected Interactive to be true")
		}
	})
}

func loadFromString(t *testing.T, content string) *Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	return cfg
}
