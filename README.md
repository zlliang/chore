# Chore

Task runner for repetitive daily chores — software updates, auth renewals, environment setup, and other routines you run often enough to automate but not often enough to remember.

Chore runs tasks defined in a TOML config file and displays progress in a live-updating terminal UI with spinners, color-coded status, and folded output.

[![Chore demo](./assets/chore-demo.gif)](https://asciinema.org/a/960336)

## Installation

```sh
go install github.com/zlliang/chore@latest
```

Or build from source:

```sh
git clone https://github.com/zlliang/chore.git
cd chore
go build -o chore .
```

## Usage

```sh
chore [task]            # run a task
chore [task] --dry-run  # preview the resolved plan without executing
chore [task] --json     # output events as newline-delimited JSON
chore list              # list all available tasks
```

Flags:

- `-c, --config <path>` — config file (default: `~/.config/chore/config.toml`)
- `-n, --dry-run` — print the resolved task plan without executing
- `--json` — output events as newline-delimited JSON (for CI/CD integration)

## Configuration

Chore reads from `~/.config/chore/config.toml` by default (respects `$XDG_CONFIG_HOME`).

### Basic example

```toml
[[tasks]]
name = "greet"
description = "Print greetings"
steps = ["hello", "goodbye"]

[[tasks]]
name = "hello"
description = "Say hello"
run = ["echo 'Hello, world!'"]

[[tasks]]
name = "goodbye"
description = "Say goodbye"
run = ["echo 'Goodbye!'"]
```

```sh
chore greet
```

### Task types

**Leaf tasks** execute shell commands:

```toml
[[tasks]]
name = "build"
description = "Compile the project"
run = [
  "echo 'Compiling...'",
  "go build ./...",
]
```

**Composite tasks** reference other tasks by name:

```toml
[[tasks]]
name = "deploy"
description = "Build, test, and deploy"
steps = ["build", "test", "upload"]
```

Composite tasks are expanded recursively and can be nested.

### Conditional execution

Use `check` to skip a task when a binary is not found in `$PATH`:

```toml
[[tasks]]
name = "brew-update"
description = "Update Homebrew packages"
check = "brew"
run = ["brew update && brew upgrade"]
```

If `brew` is not installed, this task is skipped silently.

### Interactive tasks

Tasks that require user input (e.g., `read`, password prompts) should be marked `interactive`:

```toml
[[tasks]]
name = "login"
description = "Authenticate"
interactive = true
run = ["printf 'Token: ' && read token && echo \"Saved: $token\""]
```

### Custom shell

By default, commands run via `sh -c`. Override with:

```toml
shell = ["bash", "-c"]
```
