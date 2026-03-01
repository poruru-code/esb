# Replace `esb-ctl` Go Runtime Path With Python While Keeping Go As Reference

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document follows `.agent/PLANS.md` from the repository root. Any change in implementation must be reflected in this plan so a new contributor can restart from this file alone.

## Purpose / Big Picture

After this change, users can run `esb-ctl` without compiling Go code. The command remains `esb-ctl`, and the E2E runner contract (`deploy`, `provision`, and `internal` subcommands with JSON schema checks) continues to work. The Go implementation remains in the repository as a reference, but daily execution shifts to Python/shell orchestration.

The visible behavior to prove is:
1. `esb-ctl --help` and required subcommand helps are available.
2. `esb-ctl internal capabilities --output json` returns schema/contracts expected by `e2e/run_tests.py`.
3. E2E runner unit contracts for ctl discovery and deploy command assembly pass.

## Progress

- [x] (2026-03-01 01:51Z) Reviewed planning requirements in `.agent/PLANS.md` and created this ExecPlan.
- [x] (2026-03-01 01:51Z) Confirmed branding single-source requirement and default ctl name source at `e2e/runner/branding_constants_gen.py`.
- [x] (2026-03-01 02:02Z) Implemented Python `esb-ctl` command surface in `tools/cli` with `deploy`, `provision`, and required internal subcommands.
- [x] (2026-03-01 02:02Z) Implemented deploy runtime logic (image preparation, artifact merge apply, runtime-config sync), fixture image ensure, and maven shim ensure using Go behavior as reference.
- [x] (2026-03-01 02:02Z) Switched setup/build path in `.mise.toml` `build-ctl` task to Python wrapper installer (`tools/cli/install.py`).
- [x] (2026-03-01 02:02Z) Updated docs/CI references (`README.md`, `e2e/runner/README.md`, `.github/workflows/quality-gates.yml`) to align runtime path with Python ctl.
- [x] (2026-03-01 02:02Z) Ran focused validations: ruff on new Python ctl code, E2E runner contract tests, command help probes, and capabilities JSON probe.
- [x] (2026-03-01 02:02Z) Recorded completion outcomes and residual risks.
- [x] (2026-03-01 09:46Z) Removed last Python runtime dependency on `pkg/*` by moving maven-shim assets into `tools/cli/assets/mavenshim`.
- [x] (2026-03-01 09:46Z) Updated `go.work` and CI boundary guards so `pkg` directory removal no longer hard-breaks ctl runtime path and checks.

## Surprises & Discoveries

- Observation: The E2E runner does not require Go specifically; it requires command surface and JSON contract compatibility.
  Evidence: `e2e/run_tests.py` probes subcommand help and `internal capabilities` schema/contracts only.
- Observation: Branding default for ctl command name is already generated and centralized in Python.
  Evidence: `e2e/runner/branding_constants_gen.py` defines `DEFAULT_CTL_BIN = "esb-ctl"`.
- Observation: Direct `pytest` invocation on `e2e/runner/tests/*` fails early unless E2E auth env vars are present because `e2e/conftest.py` validates them at import time.
  Evidence: test run failed with `RuntimeError: X_API_KEY is required...`; rerun with `X_API_KEY/AUTH_USER/AUTH_PASS=dummy` succeeded.
- Observation: Existing E2E runner contract tests do not require a real Docker daemon path when command execution is monkeypatched, so they are suitable as migration regression guards.
  Evidence: `test_run_tests_cli_requirement.py` and `test_deploy_command.py` passed after migration.

## Decision Log

- Decision: Implement Python ctl in `tools/cli` and keep Go code as reference.
  Rationale: User requested migration under pkg retirement assumption while explicitly keeping Go reference for verification.
  Date/Author: 2026-03-01 / Codex

- Decision: Import `DEFAULT_CTL_BIN` from `e2e/runner/branding_constants_gen.py` instead of redefining command name in new code.
  Rationale: Preserve branding single source and keep existing check scripts valid.
  Date/Author: 2026-03-01 / Codex

- Decision: Keep Go implementation files in-place and only switch runtime install path to Python.
  Rationale: User requested Go references remain available for self-verification while migration proceeds.
  Date/Author: 2026-03-01 / Codex

- Decision: Use `uv run python -m tools.cli.cli` wrapper in `~/.local/bin/esb-ctl`.
  Rationale: Avoid Python environment drift and ensure command runs with repo-locked dependencies.
  Date/Author: 2026-03-01 / Codex

- Decision: Duplicate maven-shim Docker assets under `tools/cli/assets/mavenshim` and make Python ctl read those assets.
  Rationale: Runtime ownership must stay fully in Python path; asset co-location removes the final hard dependency on `pkg`.
  Date/Author: 2026-03-01 / Codex

## Outcomes & Retrospective

Completed in this iteration:

1. Added Python ctl implementation under `tools/cli/`:
   - `cli.py`: command parser/dispatch, error hints, capabilities output.
   - `deploy_ops.py`: deploy/provision behavior including image preparation, Dockerfile rewrite, maven shim integration, runtime config target resolution and sync.
   - `artifact.py`: artifact manifest read/validate and runtime config merge behavior.
   - `fixture_image.py`: local fixture source discovery and build/push contract.
   - `maven_shim.py`: deterministic shim image derivation and build/push contract.
   - `install.py`: local wrapper installer.

2. Switched developer setup task:
   - `.mise.toml` `build-ctl` now installs Python `esb-ctl` wrapper, no Go build required for runtime use.

3. Updated docs/CI references:
   - Runtime wording in `README.md` and `e2e/runner/README.md`.

Validation results:

- `uv run ruff check tools/cli` passed.
- `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run python -m pytest e2e/runner/tests/test_run_tests_cli_requirement.py e2e/runner/tests/test_deploy_command.py` passed (`28 passed`).
- `~/.local/bin/esb-ctl --help` and all required subcommand `--help` probes returned success.
- `~/.local/bin/esb-ctl internal capabilities --output json` returned schema/contracts:
  `{"schema_version": 1, "contracts": {"maven_shim_ensure_schema_version": 1, "fixture_image_ensure_schema_version": 1}}`

Residual risks:

- Full live deploy against all environments is not exercised in this iteration; only contract/unit path was validated.
- Go reference code remains and may diverge from Python if future changes update only one side.

Pkg-retire readiness update:

- `tools/cli/maven_shim.py` now resolves assets from `tools/cli/assets/mavenshim`.
- `go.work` no longer references `./pkg/*` modules.
- `.github/checks/check_tooling_boundaries.sh` and `.github/checks/check_branding_single_source.sh` tolerate `pkg` absence and keep only relevant checks active.

## Context and Orientation

Current executable path:
- `e2e/run_tests.py` resolves ctl binary and validates command support and capabilities schema.
- `e2e/runner/deploy.py` calls:
  - `esb-ctl internal fixture-image ensure --artifact ... --output json`
  - `esb-ctl deploy --artifact ...`
  - `esb-ctl provision --project ... --compose-file ...`

Go reference behavior lives in:

Branding constants for ctl name live in:
- `e2e/runner/branding_constants_gen.py`

Important contract files:
- `e2e/runner/ctl_contract.py`
- `e2e/runner/tests/test_run_tests_cli_requirement.py`
- `e2e/runner/tests/test_deploy_command.py`

## Plan of Work

Implement a Python package `tools/cli` that exposes a `main()` command-line entrypoint and reproduces required ctl behavior.

First, build command parsing and help behavior with required subcommands and JSON output contracts. Then port operational logic in focused modules:
- Manifest read/validate and runtime config merge (artifact apply behavior).
- Deploy image preparation and runtime config sync.
- Provision command orchestration using `docker compose`.
- Internal maven shim ensure and internal fixture image ensure.

Next, switch local install path so `mise run build-ctl` installs an `esb-ctl` wrapper that executes the Python implementation.

Finally, update CI/docs references that currently imply mandatory Go build for ctl runtime usage, run focused validations, and document outcomes.

## Concrete Steps

All commands are run from repository root `/home/akira/esb`.

1. Add Python files under `tools/cli/` for CLI and operation modules.
2. Add installer module to place `~/.local/bin/esb-ctl` wrapper.
3. Update `.mise.toml` `build-ctl` task to install the Python wrapper.
4. Update docs and CI references that currently assume Go binary build for runtime usage.
5. Run validation commands:
   `uv run python -m pytest e2e/runner/tests/test_run_tests_cli_requirement.py`
   `uv run python -m pytest e2e/runner/tests/test_deploy_command.py`
   `~/.local/bin/esb-ctl --help`
   `~/.local/bin/esb-ctl internal capabilities --output json`

Expected key outputs:
- Pytest shows passing tests for the two targeted files.
- Help output includes `deploy`, `provision`, `internal`.
- Capabilities JSON includes:
  `{"schema_version":1,"contracts":{"maven_shim_ensure_schema_version":1,"fixture_image_ensure_schema_version":1}}`

## Validation and Acceptance

Acceptance criteria are behavior-focused:

2. `e2e/run_tests.py` compatibility checks pass for command help and capabilities schema/contracts.
3. Deploy command assembly tests continue to pass, proving E2E runner and ctl interface remain aligned.

## Idempotence and Recovery

Steps are idempotent:
- Re-running installer overwrites the same `~/.local/bin/esb-ctl` wrapper safely.
- Re-running tests does not mutate repository state beyond caches.

If migration causes runtime issues:
- Keep Go code untouched as reference.
- Temporarily set `CTL_BIN` to any known-good binary path while debugging Python behavior.

## Artifacts and Notes

Initial evidence collected before implementation:

  - `e2e/run_tests.py` probes required subcommands and capabilities JSON contract.
  - `e2e/runner/branding_constants_gen.py` provides canonical default ctl command name.

## Interfaces and Dependencies

New Python interface:
- Module entrypoint: `tools.cli.cli:main`.
- Installed command name: value from `e2e.runner.branding_constants_gen.DEFAULT_CTL_BIN`.

Required command interfaces:
- `deploy --artifact <path> [--no-cache]`
- `provision --project <name> --compose-file <files> [--env-file <path>] [--project-dir <dir>] [--with-deps] [-v]`
- `internal maven-shim ensure --base-image <ref> [--host-registry <host>] [--no-cache] [--output json]`
- `internal fixture-image ensure --artifact <path> [--no-cache] [--output json]`
- `internal capabilities [--output json]`

Dependencies used by Python implementation:
- Standard library (`argparse`, `json`, `pathlib`, `subprocess`, `tempfile`, `hashlib`, `zipfile`, etc.)
- `PyYAML` already present in project dependencies.

Revision note:
- 2026-03-01: Initial plan authored from current repository state and aligned to `.agent/PLANS.md`.
- 2026-03-01: Updated with completed migration work, validation evidence, and residual risk summary.
- 2026-03-01: Updated for pkg-retirement readiness (maven assets relocation, workspace cleanup, and CI guard adjustments).
