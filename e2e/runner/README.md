<!--
Where: e2e/runner/README.md
What: Architecture and execution contracts for the E2E runner pipeline.
Why: Keep WS1 runner responsibilities and behavior explicit for maintenance.
-->
# E2E Runner Architecture

## Scope
This document describes the **execution pipeline** behind `e2e/run_tests.py`.
It focuses on orchestration behavior, module boundaries, and failure contracts.

Smoke/runtime scenario design is documented separately in `docs/e2e-runtime-smoke.md`.

## Canonical Execution Path
`e2e/run_tests.py` is the single entrypoint.

```text
e2e/run_tests.py
  -> e2e.runner.cli.parse_args
  -> e2e.runner.config.load_test_matrix
  -> e2e.runner.planner.build_plan
  -> e2e.runner.runner.run_parallel
```

`e2e/runner/executor.py` is no longer part of the execution path.

## Phase Model
Each environment is executed with the same phase order.

1. `reset`
2. `compose`
3. `deploy`
4. `test`

If `--test-only` is given, `reset/compose/deploy` are skipped.
If `--build-only` is given, `test` is skipped after successful deploy.

## Module Responsibilities (WS1 Split)
| Module | Responsibility |
| --- | --- |
| `e2e/runner/runner.py` | Suite orchestration, parallel scheduling, phase sequencing, result aggregation |
| `e2e/runner/context.py` | Per-environment runtime/deploy/pytest context assembly and env merge |
| `e2e/runner/ports.py` | Stable per-environment host port block allocation for parallel runs |
| `e2e/runner/warmup.py` | Template scan and Java fixture warmup (`maven` in Docker) |
| `e2e/runner/lifecycle.py` | `compose up/down`, reset, gateway health wait |
| `e2e/runner/cleanup.py` | Aggressive cleanup for containers/networks/volumes/images |
| `e2e/runner/buildx.py` | Buildx builder selection/creation for deploy/build flow |
| `e2e/runner/planner.py` | Convert matrix entries to `Scenario` objects |
| `e2e/runner/config.py` | Matrix parsing and environment scenario expansion |

## Matrix and Scenario Resolution
`e2e/environments/test_matrix.yaml` defines suites and target environments.

`e2e/runner/config.py` resolves each matrix entry into a scenario:
- mode inference: `docker` vs `containerd`
- env file resolution from `env_dir`
- suite target expansion to project-relative paths
- firecracker inference when `env_dir` / `env_file` contains `firecracker`

`e2e/runner/planner.py` then converts raw dictionaries to typed `Scenario`.

## Failure and Exit Contract
`e2e/run_tests.py` preserves these CLI-visible contracts:

- If any environment fails, process exits with non-zero (`1`).
- Failed environments print tail logs from `e2e/.parallel-<env>.log`.
- `run_parallel` returns `dict[str, bool]` keyed by environment name.
- `--build-only` and `--test-only` cannot be used together.
- `--test-target` requires `--profile` and runs only that target.

## Logging and Diagnostics
- Per-environment logs: `e2e/.parallel-<env>.log`
- Live UI is used only when TTY + parallel + non-verbose.
- Plain reporter is always used as fallback and for summary events.

## Key Regression Tests
- `e2e/runner/test_runner_java_warmup.py`
- `e2e/runner/tests/test_context.py`
- `e2e/runner/tests/test_ports.py`
- `e2e/runner/tests/test_warmup_templates.py`

## Typical Commands
```bash
# Default matrix (serial)
uv run e2e/run_tests.py

# Parallel matrix
uv run e2e/run_tests.py --parallel

# Single profile
uv run e2e/run_tests.py --profile e2e-containerd

# Build only for a profile
uv run e2e/run_tests.py --profile e2e-containerd --build-only --verbose
```

---

## Implementation references
- `e2e/run_tests.py`
- `e2e/runner/runner.py`
- `e2e/runner/context.py`
- `e2e/runner/ports.py`
- `e2e/runner/warmup.py`
- `e2e/runner/lifecycle.py`
- `e2e/runner/cleanup.py`
- `e2e/runner/buildx.py`
- `e2e/runner/config.py`
- `e2e/runner/planner.py`
- `e2e/runner/test_runner_java_warmup.py`
- `e2e/runner/tests/test_context.py`
- `e2e/runner/tests/test_ports.py`
- `e2e/runner/tests/test_warmup_templates.py`
- `e2e/environments/test_matrix.yaml`
