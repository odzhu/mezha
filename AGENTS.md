# Repository Guidelines

## Project Structure & Module Organization
- `cmd/app/` contains the CLI entrypoint (`main.go`).
- `internal/app/` contains application logic: CLI wiring, Microsandbox SDK integration, repo sync, and runtime helpers.
- `internal/execx/` contains small command-execution utilities used for local Git operations.
- Root files: `Makefile`, `README.md`, `go.mod`, `go.sum`.
- Local build artifacts are written to `bin/`; temporary local build cache may appear under `.cache/`.

## Build, Test, and Development Commands
- `make build` — build the CLI to `./bin/mezha`.
- `make run` — run the app locally with `go run`.
- `make test` — run all Go tests.
- `make fmt` — format all Go packages with `go fmt`.
- `make tidy` — clean and sync module dependencies.
- `make clean` — remove `./bin` artifacts.

Example:
```bash
make build
./bin/mezha run -- git status
```

## Coding Style & Naming Conventions
- Use standard Go formatting; always run `make fmt` before submitting changes.
- Keep packages focused and small; prefer `internal/app` for domain logic and `cmd/app` only for startup wiring.
- Use Go naming conventions:
  - exported names: `CamelCase`
  - unexported names: `camelCase`
  - filenames: lowercase, short, descriptive (`config.go`, `sdk.go`)
- Prefer explicit error wrapping with context, e.g. `fmt.Errorf("load configuration: %w", err)`.
- Keep comments minimalistic and no longer than one line.

## Testing Guidelines
- Write or update unit tests only when the user explicitly asks for tests or when preparing a change for commit.
- Use Go’s built-in `testing` package.
- Place tests next to implementation files as `*_test.go`.
- Name tests clearly, e.g. `TestResolveRepoContext` or `TestBuildConfig`.
- Run all tests with:
```bash
make test
```
- Add tests for parsing, path handling, configuration conversion, and non-network helper logic.

## Commit & Pull Request Guidelines
- This repository currently has no commit history yet; use short, imperative commit messages such as:
  - `add microsandbox network support`
  - `refactor sandbox sync logic`
- Keep commits focused and logically grouped.
- Pull requests should include:
  - a short summary of the change
  - testing notes (`make test`, manual CLI checks)
  - any Microsandbox runtime assumptions

## Security & Configuration Tips
- Do not commit local credentials or Microsandbox configuration.
- Avoid running the app against its own working directory if upload/download behavior could overwrite local changes.
