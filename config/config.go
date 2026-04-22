package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Shell []string `toml:"shell"`
	Tasks []Task   `toml:"tasks"`
}

type Task struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Steps       []string `toml:"steps"`
	Run         []string `toml:"run"`
	Check       string   `toml:"check"`
	Interactive bool     `toml:"interactive"`
}

// IsComposite returns true if the task references other tasks via steps.
func (t *Task) IsComposite() bool {
	return len(t.Steps) > 0
}

// IsLeaf returns true if the task runs shell commands directly.
func (t *Task) IsLeaf() bool {
	return len(t.Run) > 0
}

// Load reads and parses the config file. If path is empty, it uses the default
// location at $XDG_CONFIG_HOME/chore/config.toml.
func Load(path string) (*Config, error) {
	if path == "" {
		var err error
		path, err = defaultPath()
		if err != nil {
			return nil, err
		}
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if len(cfg.Shell) == 0 {
		cfg.Shell = []string{"sh", "-c"}
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// Lookup returns a task by name.
func (c *Config) Lookup(name string) (*Task, bool) {
	for i := range c.Tasks {
		if c.Tasks[i].Name == name {
			return &c.Tasks[i], true
		}
	}
	return nil, false
}

// Resolve returns the flat ordered list of leaf tasks for a given root task,
// expanding composite tasks recursively.
func (c *Config) Resolve(name string) ([]*Task, error) {
	return c.resolve(name, nil)
}

func (c *Config) resolve(name string, seen []string) ([]*Task, error) {
	for _, s := range seen {
		if s == name {
			return nil, fmt.Errorf("cycle detected: %v -> %s", seen, name)
		}
	}

	task, ok := c.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown task: %q", name)
	}

	if task.IsLeaf() {
		return []*Task{task}, nil
	}

	seen = append(seen, name)
	var result []*Task
	for _, step := range task.Steps {
		children, err := c.resolve(step, seen)
		if err != nil {
			return nil, err
		}
		result = append(result, children...)
	}
	return result, nil
}

func (c *Config) validate() error {
	names := make(map[string]int)
	for i, t := range c.Tasks {
		if t.Name == "" {
			return fmt.Errorf("task at index %d has no name", i)
		}
		if prev, ok := names[t.Name]; ok {
			return fmt.Errorf("duplicate task name %q (indices %d and %d)", t.Name, prev, i)
		}
		names[t.Name] = i

		hasSteps := len(t.Steps) > 0
		hasRun := len(t.Run) > 0
		if hasSteps && hasRun {
			return fmt.Errorf("task %q has both steps and run", t.Name)
		}
		if !hasSteps && !hasRun {
			return fmt.Errorf("task %q has neither steps nor run", t.Name)
		}
		if hasSteps && t.Check != "" {
			return fmt.Errorf("task %q is composite but has check", t.Name)
		}
	}

	// Validate references.
	for _, t := range c.Tasks {
		for _, step := range t.Steps {
			if _, ok := names[step]; !ok {
				return fmt.Errorf("task %q references unknown step %q", t.Name, step)
			}
		}
	}

	return nil
}

func defaultPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "chore", "config.toml"), nil
}
