# Repository Guidelines

## Project Structure & Module Organization
- `services/gateway/` (FastAPI gateway), `services/agent/` (Go agent), `services/runtime-node/` (container runtime), `services/common/` (shared Python).
- `tools/cli/`, `tools/generator/`, `tools/provisioner/` for the ESB CLI and generators.
- `e2e/` for E2E scenarios and runner (`e2e/run_tests.py`), plus unit tests in `services/*/tests` and `tools/*/tests`.
- `docs/` for architecture/specs, `config/` for configs, `proto/` for gRPC definitions.

## Build, Test, and Development Commands
```bash
uv sync --all-extras        # install dev deps
lefthook install            # git hooks (ruff/go lint)
esb build                   # generate config + build images
esb up --build              # start services (Docker Compose)
esb watch                   # hot reload on file changes
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
