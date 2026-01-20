<!--
Where: docs/reports/cli_optionb_migration_plan_archive.md
What: Archived Stage 0-6 plan and rollback strategy for Option B migration.
Why: Preserve historical migration steps after Stage 7 extraction.
-->
# CLI Option B - Phase 3 Migration Plan (Archived Stages 0-6)

## Stage 0: Preparation
- Add `cli/internal/ports` and `cli/internal/workflows` packages (empty placeholders).
- Add minimal UI interface and adapters (wrapper over existing `ui` helpers).
- Ensure no behavior change at this stage.

## Stage 1: Build Workflow (Low Risk)
- Create `ports` for Build:
  - `Builder` interface (reuse existing)
  - `RuntimeEnvApplier`
  - `UserInterface`
- Create `workflows/build.go`:
  - Accept `BuildRequest` (context/env/template/no-cache/verbose)
  - Call `RuntimeEnvApplier.Apply`
  - Call `Builder.Build`
  - UI success output
- Update `runBuild`:
  - Keep `resolveCommandContext` and `.env` loading in CLI adapter
  - Build request and call `BuildWorkflow.Run`
  - Convert error to exit code

## Stage 2: Up Workflow (Medium Risk)
- Create `ports` for Up:
  - `Upper`, `Downer`, `PortPublisher`, `CredentialManager`
  - `TemplateLoader`, `TemplateParser`, `Provisioner`, `GatewayWaiter`
- Create `workflows/up.go`:
  - Apply env, reset, credentials, build (optional), up, ports, parse+provision, wait
  - UI outputs for warnings/credentials/ports/success
- Update `runUp`:
  - Keep prompt logic in CLI adapter (confirmation before reset)
  - Build request and call `UpWorkflow.Run`

## Stage 3: Dependency Wiring Split
- Replace `app.Dependencies` usage in `runBuild/runUp` with smaller constructor inputs:
  - `NewBuildCmd(builder, envApplier, ui)`
  - `NewUpCmd(upper, downer, portPublisher, credentialMgr, parser, provisioner, waiter, builder, envApplier, ui)`
- Keep legacy `Dependencies` for other commands temporarily.

## Stage 4: Logs/Down Workflows (Low Risk)
- Create `LogsWorkflow` with `Logger` port and legacy output behavior.
- Create `DownWorkflow` with `Downer` port and legacy output behavior.
- Update `runLogs`/`runDown` to delegate to workflows while keeping prompts in CLI.

## Stage 5: Cleanup and Consistency
- Move `fmt.Fprintln` in workflows to `UserInterface`.
- Remove direct calls to `EnsureAuthCredentials` and `DiscoverAndPersistPorts` from CLI adapter.
- Confirm outputs match prior behavior (message text, ordering).

## Stage 6: Documentation & Tests
- Update `docs/developer/cli-architecture.md` with workflows/ports model.
- Add unit tests for workflow behavior (build success, up reset flow, ports published).
- Run `cd cli && go test ./...` if feasible.

## Rollback Strategy
- Each stage is independently revertible.
- Keep `runBuild`/`runUp` old implementation temporarily behind feature flag if needed.
