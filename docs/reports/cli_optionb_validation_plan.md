<!--
Where: docs/reports/cli_optionb_validation_plan.md
What: Phase 4 validation plan for Option B (Up/Build first).
Why: Prevent regressions during workflow/ports refactor.
-->
# CLI Option B - Phase 4 Validation Plan

## Goals
- Ensure CLI behavior remains identical for `build` and `up`.
- Validate workflow outputs and side effects (env vars, files, ports).
- Cover newly migrated commands (`down`/`logs`/`stop`/`prune`/`env`/`project`) for regressions.
- Keep coverage focused on critical orchestration paths.

## Unit Tests (Workflows)

### BuildWorkflow
- **BuildSuccess**
  - Given context/env/template path, invokes `RuntimeEnvApplier` and `Builder`.
  - Emits success message via `UserInterface`.
- **BuildFailure**
  - Builder returns error -> workflow returns error, no success output.

### UpWorkflow
- **UpResetFlow**
  - When `Reset=true` and `ResetConfirmed=true`, calls `Downer`.
- **UpBuildOptional**
  - When `Build=true`, calls `Builder`; when false, does not.
- **UpProvision**
  - Loads template content, parses, applies provisioner.
- **UpPortsPublish**
  - Calls `PortPublisher.Publish`, surfaces ports in result.
- **UpWait**
  - When `Wait=true`, calls `GatewayWaiter`.
- **UpCredentials**
  - When credentials generated, UI outputs credential block.

### LogsWorkflow
- **LogsHappyPath**
  - Applies runtime env (when adapter provided) and delegates to Logger.Logs.
- **LogsErrorPropagation**
  - Logger returns error -> workflow returns error.

### DownWorkflow
- **DownHappyPath**
  - Delegates to Downer and emits legacy success output.

### StopWorkflow
- **StopHappyPath**
  - Applies runtime env and delegates to Stopper.

### PruneWorkflow
- **PruneHappyPath**
  - Delegates to Pruner and emits legacy success output.

### EnvWorkflows
- **EnvList**
  - Detects status when DetectorFactory is configured.
  - Marks active env using generator app last_env.
- **EnvAdd**
  - Adds a new env and persists generator.yml.
  - Rejects duplicate names.
- **EnvUse**
  - Updates generator app last_env and global config last_used/path.
- **EnvRemove**
  - Removes env and clears last_env if needed.
  - Rejects removing the last environment.

### ProjectWorkflows
- **ProjectList**
  - Marks active project using ESB_PROJECT.
- **ProjectRecent**
  - Sorts by last_used with deterministic tie-breaks.
- **ProjectUse**
  - Updates global config last_used timestamp.
- **ProjectRemove**
  - Removes project entry from global config.
- **ProjectRegister**
  - Registers project with path + last_used.

## CLI Adapter Tests
- **runBuild** maps errors to exit code = 1.
- **runUp** requires confirmation for reset in non-interactive mode.
- `.env` loading behavior unchanged.
- **runEnv** list/add/use/remove interactive and non-interactive flows unchanged.
- **runProject** list/use/remove/recent selection flows unchanged.

## Regression Checks
- Manual verification of output text ordering for `esb build` / `esb up`.
- `ports.json` written to same location as before.
- Environment variables set by `applyRuntimeEnv` remain unchanged.
- Output text parity for `down`/`logs`/`stop`/`prune`/`env`/`project`.

## Suggested Commands
- `cd cli && go test ./...`
- `uv run esb build --env <env>`
- `uv run esb up --env <env> --build`
- `uv run esb down --env <env>`
- `uv run esb logs --env <env> <service>`
- `uv run esb stop --env <env>`
- `uv run esb prune --env <env> --yes`
- `uv run esb env list`
- `uv run esb project list`

## Documentation Updates
- Update `docs/developer/cli-architecture.md` with workflows/ports overview.
