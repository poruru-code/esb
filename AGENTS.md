# Repository Guidelines

## Project Structure & Module Organization
- `services/gateway/` (FastAPI gateway), `services/agent/` (Go agent), `services/runtime-node/` (container runtime), `services/common/` (shared Python).
- `cli/` (Go CLI and the Go generator in `cli/internal/generator/`); legacy Python helpers (`tools/python_cli/`, `tools/generator/`, `tools/provisioner/`) have been removed in favor of the Go pipeline.
- `e2e/` for E2E scenarios and runner (`e2e/run_tests.py`), plus unit tests in `services/*/tests` and `tools/*/tests`.
- `docs/` for architecture/specs, `config/` for configs, `proto/` for gRPC definitions.

## Build, Test, and Development Commands
```bash
uv sync --all-extras        # install dev deps
lefthook install            # git hooks (ruff/go lint)
esb build                   # generate config + build images
esb up --build              # start services (Docker Compose)
python e2e/run_tests.py   # full E2E suite
python e2e/run_tests.py --profile e2e-containerd  # specific profile
python e2e/run_tests.py --unit-only
```
Use `uv run esb ...` if the venv is not activated.

## Coding Style & Naming Conventions
- Python: 4-space indent; `ruff format` + `ruff check` (line length 100). Go: `goimports` and `golangci-lint` in `services/agent`.
- Naming: `snake_case` for Python modules/functions, `test_*.py` for tests; Go uses `CamelCase` for exported names.
- Add a brief header comment at the top of new source files (where/what/why); comments are English.
- Keep files <= 300 LOC and single-purpose; avoid new deps unless required.
- Centralize runtime tunables in `*/config.py`; avoid magic numbers in code/tests.

## Strict Code Quality Standards
- **Linting (Ruff)**: No global ignores for standard rules (e.g., `E501` line length, `F841` unused variables) in `pyproject.toml`.
  - **Resolution Policy**: Fix long lines by refactoring (e.g., defining constants, splitting functions) rather than suppressing errors.
- **Type Checking (Ty/Pyright)**: Strict compliance required.
  - **No Global Ignores**: Do not ignore entire error categories globally.
  - **Targeted Ignores**: Use scoped `# type: ignore[error-code]` only when absolutely necessary (e.g., for generated Protobuf code or dynamic legacy imports).
- **Generated Code**: Generated files (e.g., `pb/*`) may have file-level exemptions, but consumer code must handle them explicitly.

## Testing Guidelines
- Unit tests live in `services/*/tests` and `tools/*/tests`. E2E scenarios are in `tests/scenarios/*`.
- Prefer running unit tests during development; run E2E with a running ESB stack (`esb up`).
- New features require at least one test and relevant doc updates.

## Commit & Pull Request Guidelines
- Commit history shows short, single-line summaries (Japanese and English). Use concise Japanese summaries and include the component when helpful (e.g., "Gateway: ...").
- PRs should include: purpose, scope, tests run, docs updated, and any risk/rollback notes. Link issues and add logs/screenshots for CLI or UX changes.

## Agent-Specific Workflow
- Motto: small, clear, safe steps grounded in real docs.
- Follow: Plan -> Read -> Verify (use `context7` with `resolve-library-id` then `get-library-docs`) -> Implement -> Test + Docs -> Reflect.
- Escalate if requirements are ambiguous or if security/API contracts change.
