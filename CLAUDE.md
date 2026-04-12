# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Refer to the AI engineering guidelines at https://github.com/mertenvg/my-ai-guidelines/guidelines/ for the full set of rules. They apply to all work in this repository.

## Overview
`blade` is a small Go CLI that runs and supervises local services defined in `blade.yaml` (or files under `.blade/`). It handles env var interpolation, service inheritance, file-watch-triggered restarts, tag/name filtering, and graceful shutdown on SIGINT/SIGTERM. SIGINFO prints a status snapshot.

## Common commands
- Build: `go build -o blade .`
- Run from source: `go run . run [service-or-tag ...]`
- Test all: `go test ./...`
- Test a single package: `go test ./internal/service/...`
- Run a single test: `go test ./internal/service -run TestName`
- Format / vet: `go fmt ./...` / `go vet ./...`

## Architecture
Entry point `main.go` first handles config-independent commands (`version`, `check-for-updates`, `update`), then loads YAML config from `./blade.yaml` or `./.blade/`, resolves service inheritance (`from:` field), interpolates env vars (`{$VAR}` syntax), then starts services and installs signal handlers.

- `internal/service/` — core lifecycle. `service.S` exposes `Start`, `Wait`, `Restart`, `Exit`, `Status`, `InheritFrom`. Restart flow waits for full process exit before respawning and applies a backoff delay.
- `internal/service/watcher/` — filesystem watcher with glob-style ignore patterns (`*`, `**`); triggers `Restart` on change. Transient errors (e.g. missing paths) are suppressed.
- `pkg/colorterm/` — ANSI color helpers (`Info`, `Success`, `Error`, ...). Reuse these instead of hand-writing escape codes.
- `pkg/blade/` — small helper library for child processes; uses `BLADE_SERVICE_NAME` env var and PID files to track lifecycle (`Done()`).
- `pkg/coalesce/`, `pkg/dedupe/` — small utilities used by config inheritance resolution.
- `version.go` — version resolution (ldflags or `debug.ReadBuildInfo()`), `check-for-updates` (queries Go module proxy), and `update` (`go install @latest`).
- `example/` — runnable examples; keep in sync when changing user-facing behavior.

Only third-party dependency is `gopkg.in/yaml.v3`. The project deliberately favors stdlib and minimal abstractions.

## Cached summary: my-ai-guidelines
Authoritative source: https://github.com/mertenvg/my-ai-guidelines/tree/main/guidelines (raw: `https://raw.githubusercontent.com/mertenvg/my-ai-guidelines/main/guidelines/<file>.md`). Fetch the individual file when a task needs full detail. The summary below was cached on 2026-04-09.

**principles** — Correctness first, optimize only when measured. Inspect nearby code and reuse existing utilities before introducing new patterns. Make minimal, localized changes; no drive-by refactors. Favor simplicity; preserve existing architecture and conventions.

**ai-behavior** — Deliver complete working code (no stubs/pseudo-code) with tests and verification. Stay in scope; don't invent requirements or add unrequested features. Ask when ambiguous; flag but don't fix unrelated issues. No placeholder TODOs or docstrings on code you didn't change.

**code-quality** — One responsibility per function; early returns over deep nesting. Validate inputs at system boundaries, trust internal code. Descriptive names, no type repetition. Never silently ignore errors. Error messages follow `module: operation: %w`.

**go** — `gofmt`, `go vet`, linters must pass. Imports grouped stdlib / third-party / local. Wrap errors with `fmt.Errorf("ctx: %w", err)`; check with `errors.Is/As`. Panics only for programmer errors. Define interfaces at the consumer, keep them 1–3 methods. Inject deps via constructors, never package globals. Propagate `context.Context`; pass `go test -race`.

**architecture** — Organize by feature domain (not `models/`, `handlers/`). Inject dependencies via interface-accepting constructors. Small consumer-side interfaces. Services own their schemas; cross-service contracts via gRPC/REST. Independently testable and deployable.

**dependencies** — Prefer stdlib; check existing deps before adding new ones. Pin exact versions via lock files. Evaluate maintenance, transitive deps, and license before adding.

**api-compatibility** — Don't remove/rename public API fields, endpoints, or parameters without explicit instruction. Additive changes safe; removals/renames/type changes require version bump and migration notes. Use versioned API paths; validate contracts at boundaries.

**testing** — Add tests when behavior changes, bugs are fixed, or security logic is modified (regression test for bugs). Test behavior not implementation; prefer fakes/stubs over mocks. Deterministic — no real network/time/external deps in unit tests. `require` for preconditions/errors, `assert` for values. Run `go test ./...` and `go vet ./...` before committing.

**security** — Validate/sanitize input at boundaries; encrypt sensitive data at rest and in transit. Never log or commit secrets. Parameterized SQL only. Enforce authz/token checks on every protected endpoint. Least privilege; vetted crypto libs only.

**observability** — Structured logging with key-value context (service, operation, IDs). Never log secrets, PII, or full request bodies. Wrap errors with operation context. Don't remove or disable metrics, tracing, or health checks.

**performance** — Bound concurrency (limit goroutines) and retries (backoff, no infinite loops). Every network call needs a timeout or context deadline. Batch DB ops and paginate — no unbounded result sets. Memory/CPU should scale with input, not grow unbounded.

**git-workflow** — `main` stays releasable. Short-lived `feat/`, `fix/`, `chore/` branches; rebase before review. Conventional Commits, imperative, first line ≤72 chars. One logical change per PR, <~300 lines. Issue-first when scope is uncertain. SemVer tags `vMAJOR.MINOR.PATCH`; document breaking changes.

**Not summarized (fetch on demand):** `protobuf-grpc.md`, `sql-data.md`, `typescript-react.md` — not currently used by this repo.
