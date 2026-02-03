<!--
Where: cli/docs/architecture-redesign.md
What: Target CLI architecture proposal and mode inference policy.
Why: Preserve rationale for simplifying the CLI architecture while keeping external boundaries clear.
-->
# CLI Architecture Redesign (Target)

## Context
The current CLI layout evolved for a larger command surface area and a stricter hexagonal style. Given the
current CLI scope (deploy-centric), this feels over-layered and increases maintenance cost. This document
captures a target architecture that keeps external dependencies isolated but removes unnecessary abstraction.

## Design Principles
1. **Functional core, imperative shell**: keep pure logic in `domain`, I/O at the edges.
2. **Interfaces only for external dependencies**: Docker/Compose/FS/UI.
3. **No generic ports layer**: use minimal, use-case-specific abstractions.
4. **Thin commands**: commands only parse flags and call a usecase/domain function.

## Target Structure (Minimum Viable Hex)
```
cli/
  cmd/esb/
    main.go                      # CLI entry, flags
  internal/
    command/                     # command handlers (thin)
      deploy.go
      version.go
      completion.go
    usecase/
      deploy/
        deploy.go                # orchestration only
        inputs.go                # resolve + validate
    domain/
      config/
        merge.go                 # pure
        diff.go                  # pure
      template/
        parse.go                 # pure
        params.go                # pure
      naming/
        project.go               # pure
        env.go                   # pure
      runtime/
        mode.go                  # pure inference logic
    infra/
      compose/
        runner.go                # docker compose exec
      docker/
        client.go                # docker SDK wrappers
      fs/
        fs.go                     # Read/Write/Stat
      ui/
        prompt.go                # prompt + output
    app/
      di.go                       # wiring (tiny)
```

## Implementation-Level Migration Plan (Concrete)
This section translates the target architecture into small, code-level steps. The goal is to reduce
cross-file churn by refactoring in commit-sized changes while preserving behavior.

### Step 0: Introduce new packages without deleting old ones
Create the target folders and add thin adapters that call existing code. This keeps behavior stable
and avoids a large rename sweep.

Create:
- `cli/internal/command/` (new)
- `cli/internal/usecase/deploy/` (new)
- `cli/internal/domain/` (new subpackages below)
- `cli/internal/infra/` (new subpackages below)
- `cli/internal/app/di.go` (new; replaces `wire` later)

Temporary rule: old packages (`commands`, `workflows`, `ports`, `wire`) remain until a step explicitly
removes them.

### Step 1: Move Deploy workflow into usecase (no behavior change)
Move orchestration from `cli/internal/workflows/deploy.go` into:
- `cli/internal/usecase/deploy/deploy.go`
- `cli/internal/usecase/deploy/config_diff.go` (or `domain/config` if already extracted)

Minimal edits:
- Keep function names/structs intact to reduce diff noise.
- Replace `workflows.NewDeployWorkflow` with `usecase/deploy.New`.
- Keep existing dependency shape until Step 3 (ports replacement).

### Step 2: Extract pure logic into domain
Carve out pure functions from commands/workflows/generator. Each move is a pure transformation that
eliminates direct OS/Docker access.

Recommended subpackages and concrete targets:
- `domain/runtime/mode.go`
  - `normalizeMode` (from `commands/deploy.go`)
  - `inferModeFromContainers` (pure logic; inputs are container summaries)
  - `inferModeFromComposeFiles` (pure string/file-name logic)
  - `fallbackDeployMode`
- `domain/naming/project.go`
  - `defaultDeployProject` (change signature to accept `brand`, `env` instead of reading env)
  - `ComposeProjectKey` (from `staging`, if needed as pure naming)
- `domain/config/output.go`
  - `resolveDeployOutputSummary`
  - `normalizeOutputDir`
- `domain/config/diff.go`
  - `diffConfigSnapshots`, `diffMap`, and count formatting helpers
- `domain/template/params.go`
  - `extractSAMParameters` and parameter-default extraction logic
  - suggestion/merge helpers like `buildTemplateSuggestions`, `updateTemplateHistory`

Rules:
- If a function touches `os`, `filepath`, or Docker SDK, split it: keep pure logic in `domain`,
  move I/O to `infra/*` (or keep in command/usecase until Step 4).

### Step 3: Replace ports with concrete infra interfaces (only 4)
Remove `cli/internal/ports` and expose only these interfaces inside `infra/*`:

```
type DockerClient interface { ... }        // Docker SDK subset
type ComposeRunner interface { ... }       // docker compose exec/run
type FS interface { ... }                  // ReadFile/WriteFile/Stat/ReadDir/MkdirAll/RemoveAll
type UI interface {
  Prompt() Prompter // Input/Select/SelectValue
  Print() Printer   // Info/Warn/Success/Block
}
```

Concrete implementations:
- `infra/docker/client.go` wraps `compose.NewDockerClient` and SDK calls.
- `infra/compose/runner.go` wraps `compose.ExecRunner`.
- `infra/fs/osfs.go` wraps `os`/`filepath`.
- `infra/ui/console.go` wraps `interaction.HuhPrompter` + `ui.Console`.

Usecases depend only on these four interfaces. No `Builder` or `RuntimeEnvApplier` interface:
replace them with concrete functions that accept these deps as parameters.

### Step 4: Shrink command layer to thin adapters
Move input resolution helpers into `command/deploy_inputs.go` and limit command code to:
- parse flags
- call `usecase/deploy.Run(ctx, inputs, deps)`
- map errors to exit codes

Concrete changes:
- `commands.Run` becomes `command.Run` (or keep `commands` as a shim).
- `commands/deploy.go` keeps only CLI input logic and uses `domain/*` for pure transforms.

### Step 5: Collapse wire -> app/di.go
Replace `cli/internal/wire/wire.go` with `cli/internal/app/di.go`:
- build infra implementations
- pass them to command/usecase
- keep `main.go` unchanged except for import path

### Step 6: Remove old layers
Delete in order:
- `cli/internal/ports/*`
- `cli/internal/workflows/*` (after usecase is stable)
- `cli/internal/wire/*` (after app/di.go is wired)

### Step 7: Generator/Build split (optional but ideal)
Generator is currently a mix of parsing (pure) and I/O (filesystem + docker). Split by dependency:
- `domain/template/*` and `domain/config/*`: parsing, name normalization, image naming
- `infra/build/*`: docker/buildx/bake, registry checks, file staging
- `usecase/deploy` composes domain + infra to run "generate + build + stage + provision"

## Concrete Dependencies for Deploy Usecase
To enforce the boundary, the deploy usecase should only know about:
- `infra.DockerClient`
- `infra.ComposeRunner`
- `infra.FS`
- `infra.UI`

Everything else should be a plain function call in `domain` or `usecase`.

## Decisions (Senior Review)
### 1) Usecase Scope
- **Keep usecase only for `deploy`**.
- Other commands are thin; they can call `domain` or `infra` directly.

### 2) Domain Scope
- **Only pure logic** (no I/O):
  - config merge/diff
  - template parsing/parameters
  - naming/normalization
  - runtime mode inference (pure functions only)

### 3) Required Interfaces (Only 4)
- `DockerClient`
- `ComposeRunner`
- `FS`
- `UI`

Everything else should be concrete implementation code.

## Runtime Mode: UX + Inference Policy
### Problem
- Function images are **runtime-agnostic**, but deploy still needs runtime context for:
  - provisioner compose selection
  - service status checks
  - registry/port resolution

### Policy (When project is running)
- **Do not prompt for mode**.
- **Infer mode from running project**, with the following precedence:
  1. Running containers: `runtime-node` -> containerd
  2. Running containers: `agent` -> docker
  3. All containers in project (if none running)
  4. Compose config files (`config_files`) as fallback
- If user passes `--mode` and it conflicts, **warn and ignore**.

### Policy (No running project)
- Prompt for `--mode` (or accept flag).

## Compose Config Alignment
When a project is running, the CLI should **reuse the exact compose files that started it** by reading
`com.docker.compose.project.config_files`. This avoids mismatches (e.g., orphan warnings, missing services).

Applied to:
- provisioner execution
- service running checks
- port discovery
- E2E port discovery

## Non-Goals
- Remove mode entirely (needed when no project is running).
- Fully remove hex architecture (external boundaries still matter).
- Large refactor in one step (prefer incremental migrations).

## Migration Outline (Incremental)
1. Introduce `usecase/deploy` and move orchestration there.
2. Move pure logic into `domain/*`.
3. Collapse unused `ports` layers into concrete implementations.
4. Keep `infra/*` only for Docker/Compose/FS/UI.

## Notes
- This document is a target architecture, not a full refactor plan.
- Actual refactoring should be done in small, safe steps.
- A pragmatic migration map is available in `cli/docs/architecture-mapping.md`.
