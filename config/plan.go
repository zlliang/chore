package config

import "fmt"

// Plan is a resolved, flat execution plan for a named root task.
type Plan struct {
	RootName string
	RootDesc string
	Shell    []string
	Tasks    []Task // flattened leaf tasks in execution order
}

// Plan resolves the named task into a flat execution plan.
func (c *Config) Plan(name string) (*Plan, error) {
	root, ok := c.Lookup(name)
	if !ok {
		return nil, fmt.Errorf("unknown task: %q", name)
	}

	leaves, err := c.Resolve(name)
	if err != nil {
		return nil, err
	}

	tasks := make([]Task, len(leaves))
	for i, t := range leaves {
		tasks[i] = *t
	}

	shell := make([]string, len(c.Shell))
	copy(shell, c.Shell)

	return &Plan{
		RootName: root.Name,
		RootDesc: root.Description,
		Shell:    shell,
		Tasks:    tasks,
	}, nil
}
