<!--
Where: docs/reports/cli_optionb_dependency_inventory.md
What: Phase 1 dependency and side-effect inventory for Option B (Up/Build).
Why: Establish precise boundaries for workflow/ports refactor.
-->
# CLI Option B - Phase 1 Dependency Inventory (Up/Build)

Scope: `cli/internal/app/runBuild`, `cli/internal/app/runUp`, and shared context/env setup.

## Current Execution Flows

### Build
1. `runBuild` -> `resolveCommandContext`
2. `resolveCommandContext` -> `resolveProjectSelection` -> `loadGlobalConfigWithPath` -> `state.ResolveAppState`
3. `resolveCommandContext` -> `loadProjectConfig` -> `config.LoadGeneratorConfig`
4. `resolveCommandContext` -> `state.ResolveProjectState` -> `state.ResolveContext`
5. `runBuild` -> `applyRuntimeEnv` (sets env vars)
6. `runBuild` -> `Builder.Build(manifest.BuildRequest)`
7. Output via `fmt.Fprintln`

### Up
1. `runUp` -> `resolveCommandContext` (same chain as Build)
2. `runUp` -> `applyRuntimeEnv` (sets env vars)
3. Optional reset flow:
   - `printResetWarning`
   - `promptYesNo` (interactive)
   - `Downer.Down`
4. `EnsureAuthCredentials` (sets env vars + prints)
5. Optional build flow: `Builder.Build`
6. `Upper.Up` (docker compose up)
7. `DiscoverAndPersistPorts` (compose port discovery + `ports.json` write + env vars)
8. `os.ReadFile(templatePath)` -> `Parser.Parse` -> `Provisioner.Apply`
9. Optional: `Waiter.Wait` (HTTP health polling)
10. Output via `fmt.Fprintln` + `ui.Console`

## Dependencies Used by Up/Build

### External Ports (current interfaces)
- `Builder`: builds images and generated configs.
- `Upper`: docker compose up.
- `Downer`: docker compose down (reset flow).
- `PortDiscoverer`: discovers runtime ports.
- `Parser`: parses SAM template into manifest resources.
- `Provisioner`: applies resources to runtime.
- `GatewayWaiter`: waits for gateway health.
- `RepoResolver`: resolves repo root for compose and env.
- `Prompter`: interactive selection/input.

### Global Functions / Static State
- `applyRuntimeEnv` (mutates env vars, reads CA cert file)
- `EnsureAuthCredentials` (mutates env vars, prints credentials)
- `DiscoverAndPersistPorts` (writes `~/.<brand>/<env>/ports.json`, mutates env vars)
- `promptYesNo` / `isTerminal`

### File / OS Side Effects
- Read: `generator.yml`, `template.yaml`, CA cert file, `.env` (optional)
- Write: `ports.json`, generated files under `out/` (via builder), env vars
- Network: docker compose calls, HTTP health polling to gateway

## Refactor Boundaries (Option B Targets)

### CLI Adapter Responsibilities
- Parse flags, handle prompts, assemble DTOs.
- Own interactivity and UX (no prompts inside workflows).
- Convert workflow errors to exit codes and formatted output.

### Workflow Responsibilities (pure orchestration)
- `UpWorkflow` and `BuildWorkflow` with explicit ports:
  - `ContextResolver` (project/env/context resolution)
  - `RuntimeEnvApplier` (env var setup)
  - `CredentialManager` (auth credential generation + reporting)
  - `BuildExecutor` (builder)
  - `UpExecutor` (compose up)
  - `PortPublisher` (discover + persist + env)
  - `TemplateLoader` + `TemplateParser`
  - `Provisioner`
  - `GatewayWaiter`
- Output via a single `UserInterface` abstraction.

### Data Transfer Objects (DTOs)
- `BuildRequest` (project/env/template override/no-cache/verbose)
- `UpRequest` (project/env/template override/build/reset/wait/detach)
- `BuildResult` / `UpResult` for structured output and testing.

## Notes / Risks
- `applyRuntimeEnv` mixes env defaults, branding, and proxy handling; keep behavior identical.
- `EnsureAuthCredentials` currently prints secrets; re-evaluate if UI abstraction should redact.
- `resolveCommandContext` uses prompts and config reads; it should be split into:
  - selection (interactive) and
  - resolution (pure).
