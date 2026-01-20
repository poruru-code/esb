<!--
Where: docs/reports/cli_optionb_validation_plan.md
What: Phase 4 validation plan for Option B (Up/Build first).
Why: Prevent regressions during workflow/ports refactor.
-->
# CLI Option B - Phase 4 Validation Plan

## Goals
- Ensure CLI behavior remains identical for `build` and `up`.
- Validate workflow outputs and side effects (env vars, files, ports).
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

## CLI Adapter Tests
- **runBuild** maps errors to exit code = 1.
- **runUp** requires confirmation for reset in non-interactive mode.
- `.env` loading behavior unchanged.

## Regression Checks
- Manual verification of output text ordering for `esb build` / `esb up`.
- `ports.json` written to same location as before.
- Environment variables set by `applyRuntimeEnv` remain unchanged.

## Suggested Commands
- `cd cli && go test ./...`
- `uv run esb build --env <env>`
- `uv run esb up --env <env> --build`

## Documentation Updates
- Update `docs/developer/cli-architecture.md` with workflows/ports overview.
