# Project Guidelines

These guidelines describe how to propose changes, write code, and collaborate effectively in this repository.

If anything here is unclear or you think an improvement is needed, please open an issue or a PR to update this document.


## Table of Contents
- Goals and Principles
- Getting Started
- Branching and Workflow
- Commit Messages
- Pull Requests
- Code Style (Go)
- Error Handling and Logging
- Concurrency
- Configuration and Environment
- Testing
- Tooling
- Versioning and Releases
- Documentation
- Security and Responsible Disclosure
- Communication and Behavior

---

## Goals and Principles
- Favor clarity over cleverness; the codebase should be easy to understand.
- Keep the surface area small; avoid unnecessary abstractions.
- Prefer simple, explicit solutions before introducing new dependencies.
- Be kind to the next person reading your code (it might be you).

## Getting Started
1. Ensure you have a recent Go toolchain installed (see `go.mod` for the required version).
2. Clone the repository and run:
   ```sh
   go mod download
   ```
3. Explore examples under `example/` to understand typical usage.

## Branching and Workflow
- Main branch: `main` is protected and should always be releasable.
- Feature branches: use short-lived branches named like `feat/<short-name>`, `fix/<short-name>`, or `chore/<short-name>`.
- Keep PRs small and focused; aim for under ~300 lines of diff when possible.
- Rebase your branch on top of the latest `main` before requesting review to keep history clean.

### Issue-first changes
- Open an issue describing the problem or proposal when in doubt.
- Reference the issue in your PR description and commit messages.

## Commit Messages
Follow Conventional Commits where reasonable:
- `feat: add blade YAML tag filtering`
- `fix: handle watcher errors on missing path`
- `docs: update README with installation note`
- `chore: bump dependencies`
- `refactor: simplify service runner lifecycle`

Guidelines:
- Use the imperative mood: "add", "fix", "update".
- The first line should be ≤ 72 characters when possible.
- Include context in the body if the change is non-trivial (what/why, not how).

## Pull Requests
- One logical change per PR. Split large changes into multiple PRs.
- Provide a clear description: problem, approach, alternatives considered, trade-offs.
- Include before/after examples or screenshots/logs when relevant.
- Ensure `go build` succeeds and tests pass locally.
- Request review from a relevant maintainer.

## Code Style (Go)
This project follows idiomatic Go conventions.

- Formatting: always run `gofmt` (most editors do this automatically). `go fmt ./...` before commits.
- Imports: group standard library, third-party, then local packages; keep imports sorted.
- Naming: use descriptive, short names; exported identifiers require clear doc comments.
- Package layout: prefer smaller, cohesive packages. Avoid circular dependencies.
- Dependencies: keep them minimal; prefer stdlib when possible.
- Public API: maintain backward compatibility where feasible; document breaking changes.
- Panics: avoid panicking in libraries; return errors instead.

### Project-specific notes
- Colorized terminal output lives in `pkg/colorterm`. Reuse helpers there instead of re-implementing ANSI codes.
- Core orchestration code is under `pkg/blade` and `internal/service`. Keep responsibilities narrow:
  - `internal/service`: service definitions, lifecycle, and watching logic.
  - `pkg/blade`: user-facing library surface and CLI glue shared with `main.go`.
- Examples under `example/` should remain runnable and small; keep them in sync with feature changes.

## Error Handling and Logging
- Return `error` values; wrap with context using `%w` and `fmt.Errorf`.
- Prefer sentinel errors or small typed errors for common conditions; check with `errors.Is/As`.
- Logging:
  - Be concise; include actionable context (service name, path, command).
  - Do not log sensitive information (secrets, tokens, private file paths if sensitive).
  - For user-facing CLI output, use consistent formatting and color helpers where appropriate.

## Concurrency
- Use contexts to control goroutines where applicable; cancel on shutdown.
- Guard shared state with channels or mutexes; avoid data races.
- Ensure goroutines exit on program termination and watchers are cleaned up.
- When using file watchers, debounce events appropriately to avoid flapping.

## Configuration and Environment
- `blade.yaml` is the primary config surface. Keep it human-friendly.
- Document new config fields in `README.md` with examples. Keep defaults sensible.
- Environment handling:
  - Never assume env vars exist—validate and provide clear errors.
  - Support `inheritEnv` patterns consistently across services.

## Testing
- Unit tests for non-trivial logic. Place tests in the same package with `_test.go` suffix.
- Keep tests deterministic; avoid sleeping unless necessary. Use fakes where possible.
- Use table-driven tests for permutations.
- Name tests clearly: `TestThing_Scenario_Expectation`.
- Run all tests locally before submitting:
  ```sh
  go test ./...
  ```

## Tooling
- Building: `go build ./...`
- Formatting: `go fmt ./...`
- Vetting (optional but recommended): `go vet ./...`
- Linting: If you use `golangci-lint`, prefer the default rules unless a false-positive requires tuning.

## Versioning and Releases
- Follow semantic versioning (SemVer) for tags: `vMAJOR.MINOR.PATCH`.
- Document noteworthy changes in `README.md` or a future `CHANGELOG.md`.
- Avoid breaking changes in minor/patch releases. If unavoidable, document migration steps.

## Documentation
- Update `README.md` when adding features or altering behavior.
- Include small runnable examples under `example/` when introducing new capabilities.
- Keep comments concise and useful; exported symbols should have GoDoc-friendly comments.

## Security and Responsible Disclosure
- Do not include secrets in the repository or logs.
- If you discover a vulnerability, please report it privately to the maintainers. Avoid filing public issues with exploit details. We will acknowledge receipt and work on a fix before disclosure.

## Communication and Behavior
- Be respectful and constructive.
- Assume positive intent. Disagree and commit when consensus is reached.
- Prefer async communication via issues and PRs to keep discussions discoverable.
