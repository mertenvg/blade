# blade
A developer-friendly process runner for monorepos and multi-service projects. It reads a `blade.yaml` file and:
- starts one or more services concurrently
- watches files and restarts services on changes
- forwards output to your terminal
- handles graceful shutdown
- provides a quick status glance via a signal


## Overview
Blade is a small CLI you install with Go. In each repository, you define services in `blade.yaml` (command to run, what to watch, env vars, output preferences, etc.). Then run `blade run` to boot them all, or specify a subset by name.

- Language/Stack: Go (Go modules)
- Frameworks/Libraries: `gopkg.in/yaml.v3` for config parsing, simple internal file-watcher, colored logging via `pkg/colorterm`
- Package manager: Go modules (`go.mod`)
- Entry point: `main.go` (binary name `blade` when installed)


## Requirements
- Go 1.24+ (module declares `go 1.24.1`)
- macOS or Linux are expected to work
  - TODO: Confirm Windows support and document any limitations


## Installation
```bash
go install github.com/mertenvg/blade@latest
``` 
Make sure `$GOPATH/bin` (or your Go bin dir, typically `$HOME/go/bin`) is on your `$PATH`.

Alternatively, build locally from source:
```bash
git clone https://github.com/mertenvg/blade.git
cd blade
go build -o blade .
```


## Usage
```bash
# run all services (except those marked skip: true)
blade run

# run only selected services by name
blade run service-one service-two
```
Blade reads configuration from `./blade.yaml` in the current working directory. If arguments are provided, only the named services are run; otherwise, all non-skipped services are started.

Signals and status:
- Send SIGINT or SIGTERM (e.g., Ctrl+C) to gracefully stop all services.
- Send SIGINFO to print a live status snapshot (active/inactive, pid, uptime).
  - Note: On macOS SIGINFO can be triggered with Ctrl+T. On Linux this varies.
  - TODO: Document platform-specific key combos and behavior for SIGINFO.


## Configuration (blade.yaml)
Blade uses YAML to define services. Minimum per-service fields: `name` and `run`.

Example (see full example in `example/blade.yaml`):
```yaml
- name: service-one
  watch:
    fs:
      path: cmd/service-one/
      ignore:
        - node_modules
        - .*
        - "**/*_test.go"
  env:
    - name: VAR_1
      value: VAL_1
  before: echo "do something at first run"
  run: go run cmd/service-one/main.go
  output:
    stdout: os
    stderr: os

- name: service-two
  inheritEnv: true
  env:
    - name: VAR_3
      value: VAL_3
    - name: VAR_EMPTY_QUOTE
      value: ""
  watch:
    fs:
      paths:
        - cmd/service-two/main.go
  run: go run cmd/service-two/main.go
  output:
    stdout: os
    stderr: os
```

Schema (inferred from code):
- Service fields (`internal/service/service.go`):
  - `name` (string) — required
  - `run` (string) — required; shell command to start the service
  - `before` (string) — optional; one-time command executed prior to first start
  - `watch` (object) — optional; file watching config
    - `fs.path` (string) — single path to watch
    - `fs.paths` (array<string>) — multiple paths to watch
    - `fs.ignore` (array<string>) — glob-like patterns to ignore (`*`, `**` supported)
  - `inheritEnv` (bool) — if true, inherit current process env for the child; if false, start with an empty env
  - `env` (array<object>) — environment variable entries:
    - `name` (string) — variable name
    - `value` (string, optional) — explicit value; if omitted, the current environment value is used (may be empty)
  - `dir` (string) — working directory for the command (defaults to `.`)
  - `output` (object) — where to pipe stdio:
    - `stdout` (string) — when set to `os`, stdout is passed through to the terminal
    - `stderr` (string) — when set to `os`, stderr is passed through to the terminal
    - `stdin`  (string) — when NOT set to `os`, stdin is passed through to the terminal (current behavior in code)
  - `sleep` (int, milliseconds) — delay before restarting after a service exits
  - `skip` (bool) — do not start this service when no explicit list is provided
  - `dnr` (bool) — do-not-restart flag used on exit/shutdown

Behavioral notes:
- Blade auto-sets `BLADE_SERVICE_NAME` for each child process.
- A small PID helper in `pkg/blade` writes `.<service>.pid` on start and deletes it on exit if your service imports `github.com/mertenvg/blade/pkg/blade` and calls `blade.Done()` on shutdown (see `example/cmd/service-one`).
- Exponential backoff is applied when a service fails to start; backoff resets after a successful run.


## Environment Variables
- Reserved/Injected by Blade:
  - `BLADE_SERVICE_NAME` — set for child processes to the current service name. Used by `pkg/blade` to manage PID files.
- From config (`env`):
  - If `value` is provided, that value is used.
  - If `value` is omitted, the current environment value is captured and forwarded (may be empty).
- `inheritEnv: true` starts the child with the full current environment; otherwise, the child starts with an empty environment and only variables defined in `env` are present.


## Development and Examples
Example services are under `example/cmd/*` with a sample `example/blade.yaml`.

Run the example from the repo root:
```bash
cp example/blade.yaml ./blade.yaml
./blade run           # if you built locally
# or, if installed in PATH:
blade run
```


## Testing
- The repository currently includes a placeholder test in `example/cmd/service-one/main_test.go` that is intentionally ignored by the default ignore patterns.
- TODO: Add unit tests for the watcher, service lifecycle, and YAML parsing.
- TODO: Add integration tests that spin up short-lived example services and verify restart behavior and signal handling.

Run tests (once added):
```bash
go test ./...
```


## Project Structure
```
.
├── main.go                       # CLI entry point
├── internal/
│   └── service/
│       ├── service.go            # service lifecycle (start/restart/exit/status, env, output)
│       └── watcher/
│           └── watcher.go        # simple FS watcher with ignore patterns
├── pkg/
│   ├── blade/blade.go            # PID helper using BLADE_SERVICE_NAME
│   └── colorterm/colorterm.go    # colored console output
├── example/
│   ├── blade.yaml                # sample configuration
│   └── cmd/                      # toy services for demonstration
├── go.mod / go.sum               # Go modules
├── README.md                     # this file
└── LICENSE
```


## Scripts
There are no external script runners (e.g., Makefile) in this repo. Useful Go commands:
- Build: `go build -o blade .`
- Install: `go install github.com/mertenvg/blade@latest`
- Run locally without installing: `go run . run`


## License
This project is licensed under the terms of the license in `LICENSE`.


## Roadmap / TODO
- [x] Allow tags per service in `blade.yaml` to filter by tag when running (from original TODO)
- [ ] Document Windows support and SIGINFO behavior across platforms
- [x] Add unit and integration tests
