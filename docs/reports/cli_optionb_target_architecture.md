<!--
Where: docs/reports/cli_optionb_target_architecture.md
What: Phase 2 target architecture spec for Option B (minimal skeleton + Up/Build).
Why: Provide a concrete design baseline before implementation.
-->
# CLI Option B - Phase 2 Target Architecture (Minimal)

Scope: introduce `workflows` + `ports` for `build`/`up` without behavior changes.

## Goals
- Separate CLI input collection from orchestration logic.
- Replace `Dependencies` mega-struct with per-command wiring.
- Provide a single output surface (UI) for workflow results.
- Preserve current behavior and side effects.

## Non-Goals
- No CLI feature changes, no flag changes.
- No large directory restructuring beyond new packages.
- No change to external interfaces (compose, generator, provisioner).

## Proposed Package Layout (Minimal)

```
cli/internal/
  workflows/
    build.go
    up.go
    down.go
    logs.go
  ports/
    build.go
    up.go
    logs.go
    ui.go
```

- `cli/internal/app`: remains CLI adapter and command dispatch for now.
- `cli/internal/workflows`: orchestration and policy (no prompts).
- `cli/internal/ports`: small interfaces and request/result DTOs.

## Ports (Interfaces)

### Shared
- `RuntimeEnvApplier`: applies env defaults for runtime.
  - `Apply(ctx state.Context) error` (wraps `applyRuntimeEnv`)
- `UserInterface`: unified output surface for workflows.
  - `Info(msg string)`
  - `Warn(msg string)`
  - `Success(msg string)`
  - `Block(title string, items map[string]string)` (for credentials/ports)

### Build
- `Builder`: current `Build(request manifest.BuildRequest) error`.
- `TemplatePathResolver`: resolves override vs context template path.

### Up
- `Upper`: current `Up(request UpRequest) error`.
- `Downer`: current `Down(project string, removeVolumes bool) error`.
- `PortPublisher`: discover + persist + env apply.
  - `Publish(ctx state.Context) (map[string]int, error)`
- `CredentialManager`: ensure auth creds.
  - `Ensure() (AuthCredentials, error)`
- `TemplateLoader`: read template content.
  - `Read(path string) (string, error)`
- `TemplateParser`: parse template into resources.
  - `Parse(content string, params map[string]string) (manifest.Template, error)`
- `Provisioner`: apply parsed resources.
  - `Apply(ctx context.Context, resources manifest.ResourcesSpec, project string) error`
- `GatewayWaiter`: wait for gateway.
  - `Wait(ctx state.Context) error`

### Logs
- `Logger`: log streaming + service listing.
  - `Logs(request LogsRequest) error`
  - `ListServices(request LogsRequest) ([]string, error)`
  - `ListContainers(project string) ([]state.ContainerInfo, error)`

## Workflow APIs

### BuildWorkflow
- Input: `BuildRequest`
  - `Context state.Context`
  - `Env string`
  - `TemplatePath string`
  - `NoCache bool`
  - `Verbose bool`
- Output: `BuildResult`
  - `TemplatePath string`
  - `ProjectName string`
- Behavior:
  - Apply runtime env.
  - Build via Builder.
  - UI success message (no prompts).

### UpWorkflow
- Input: `UpRequest`
  - `Context state.Context`
  - `Env string`
  - `TemplatePath string`
  - `Detach bool`
  - `Wait bool`
  - `Build bool`
  - `Reset bool`
  - `EnvFile string`
  - `ResetConfirmed bool`
- Output: `UpResult`
  - `Ports map[string]int`
  - `CredentialsGenerated bool`
- Behavior:
  - Apply runtime env.
  - If `Reset` -> Downer (no prompt here).
  - Ensure credentials; if generated, emit via UI.
  - If `Build` -> Builder.
  - Upper.Up
  - Publish ports; UI prints.
  - Load/Parse template; Provisioner.Apply.
  - If `Wait` -> Waiter.Wait.
- UI success message.

### LogsWorkflow
- Input: `LogsRequest` (context/follow/tail/timestamps/service)
- Output: error only
- Behavior:
  - Apply runtime env.
  - Delegate to `Logger.Logs`.

## CLI Adapter Responsibilities
- Parse flags (Kong).
- Handle interactive prompts and confirmation.
- Build `BuildRequest`/`UpRequest` with resolved context/template path.
- Provide UI implementation and map workflow errors to exit codes.
  - `logs` uses CLI prompts to select service and then calls `LogsWorkflow`.

## Error / Exit Strategy
- Workflow returns `error` only; no `os.Exit`.
- CLI maps:
  - `nil` -> 0
  - `error` -> 1 + formatted message
- Use `UserInterface` for any user-visible output within workflows.

## Migration Notes (Build/Up)
- `resolveCommandContext` remains in CLI adapter for now.
- `applyRuntimeEnv`, `EnsureAuthCredentials`, `DiscoverAndPersistPorts` get wrapped
  by ports adapters to avoid global calls inside workflows.
- `fmt.Fprintln` in `runBuild/runUp` becomes UI output in workflows.

## Risks and Compatibility
- Ensure `ResetConfirmed` is enforced in CLI to keep safety behavior identical.
- Preserve `.env` loading behavior in CLI adapter (pre-workflow).
- Keep output wording identical to avoid CLI snapshot drift.
