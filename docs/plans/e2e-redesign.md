# E2E Runner Redesign (Zero-Base)

## Goal
- Rebuild E2E runner around the real user flow: `docker compose up` → `esb deploy` → `pytest`.
- Run multiple environments in parallel with clean, deterministic isolation.
- Minimize hidden side effects and split responsibilities clearly.
- Default output is plain logs; verbose is the default and `--no-verbose` reduces output.

## Current State (As-Is)

### Flow
- `e2e/run_tests.py` loads matrix, warms up infra (registry), and spawns parallel workers.
- Worker is **the same module** (`run_tests.py`) re-invoked with `--profile`.
- Worker executes `run_scenario()` in `e2e/runner/executor.py`.

### Observed Issues
- **Responsibility bloat**: `executor.py` does env computation, reset, buildx, compose, deploy, pytest, logging, and parallel output.
- **Side effects everywhere**: frequent `os.environ` mutations across layers.
- **Registry readiness** logic duplicated (infra + builder path).
- **Logging/UI coupling**: output formatting mixed into execution logic.
- **Worker re-entry**: subprocess uses the same script, making control flow hard to follow and debug.

## Design Principles (To-Be)
1. **Pure planning**: matrix → execution plan should be side-effect free.
2. **Single responsibility**: each module does one thing well.
3. **Explicit inputs/outputs**: pass env as data; do not depend on global environment changes.
4. **Parallelism by runner**: each environment is an independent runner with isolated env, logs, and resources.
5. **Plain logs only**: keep output deterministic; no Rich dependency.

## Proposed Architecture

### Modules
- `planner`:
  - Input: matrix/suites + CLI args.
  - Output: execution plan (list of environment runs).
  - **No side effects**.

- `env`:
  - Input: env_name + mode + base env file.
  - Output: resolved runtime env dict (no global mutation).

- `infra`:
  - Start shared registry and wait ready.

- `lifecycle`:
  - Reset (compose down -v + cleanup), compose up, optional compose down.

- `deploy`:
  - Run `esb deploy` with explicit env.

- `test`:
  - Run pytest with explicit env.

- `runner`:
  - Orchestrates: reset → compose up → deploy → pytest for one environment.
  - Emits events for UI/logging.

- `events`:
  - Event model for progress reporting (env started, phase started, phase progress, etc.).

- `ui`:
  - Plain reporter only (Rich removed).

- `logging`:
  - Always write full output to env-specific file.

### Execution Flow
1. Build plan (planner).
2. Ensure registry ready (infra).
3. Run N environment runners in parallel.

Each runner:
1. Reset (clean environment, always).
2. `docker compose up -d`.
3. `esb deploy` (template + params).
4. `pytest`.

## UI Spec (Plain)
- No live layout; logs stream in normal order with `[env]` prefixes.
- Verbose output is the default; `--no-verbose` limits output to phase/event markers.

## Logging Rules
- Always write full logs to `e2e/.parallel-<env>.log`.
- Full logs always written to `e2e/.parallel-<env>.log`.
- Plain output always uses `[env]` prefix when streaming logs.

## Parallelism
- Use `ThreadPoolExecutor` for N environment runners.
- Each runner runs subprocesses for docker/pytest/esb with explicit env.
- No shared mutable global state.

## Open Questions
- Do we keep current `run_tests.py` entrypoint or create a new driver?
- What is the exact list of phases (e.g., include build phase or not)?
- Should we keep `--build`, or rely on auto-detection only?

## Migration Plan
1. Implement new `planner` + `runner` and keep old path.
2. Remove old executor logic after parity check.
