<!--
Where: cli/docs/architecture-redesign.md
What: Target CLI architecture proposal and mode inference policy.
Why: Preserve rationale for simplifying the CLI architecture while keeping external boundaries clear.
-->
# CLI Architecture Redesign (Target)

## Context
The CLI is deploy-centric and has a small, stable command surface. A full hexagonal layout is too heavy for
this scope, but isolating external dependencies (Docker/Compose/FS/UI) is still valuable. This document
captures a minimal structure and an implementation-level migration plan that keeps behavior stable while
reducing file churn.

## Design Principles
1. **Functional core, imperative shell**: keep pure logic in `domain`, I/O at the edges.
2. **Interfaces only for external dependencies**: Docker/Compose/FS/UI.
3. **No generic ports layer**: use minimal, use-case-specific abstractions.
4. **Thin commands**: commands only parse flags and call a usecase/domain function.

## Target Structure (Current Implementation Aligned)
```
cli/
  cmd/esb/
    main.go                      # CLI entry, flags
  internal/
    command/                     # command handlers (thin)
      deploy.go
      completion.go
      version.go
    usecase/
      deploy/
        deploy.go                # orchestration only
        config_diff.go           # pure diff helpers (can move to domain)
    domain/
      config/                    # pure config logic
      manifest/                  # internal resource specs (pure)
      runtime/                   # mode inference (pure)
      state/                     # pure DTOs
      template/                  # renderer, types, image naming (pure)
      value/                     # typed coercion helpers (pure)
    infra/
      build/                     # build + staging + docker/bake
      compose/                   # docker compose exec
      config/                    # config structs
      env/                       # runtime env application
      envutil/                   # env helpers
      interaction/               # prompt impl
      sam/                       # aws-sam-parser-go boundary + intrinsics
      staging/                   # path helpers (I/O)
      ui/                        # UI output
    app/
      di.go                      # wiring
```

## Implementation-Level Migration Plan (Concrete)
This section translates the target architecture into commit-sized steps and concrete file moves.

### Step 1: Deploy usecase wiring (DONE)
- `cli/internal/workflows/deploy.go` -> `cli/internal/usecase/deploy/deploy.go`
- `cli/internal/commands/deploy.go` -> `cli/internal/command/deploy.go`
- Replace `workflows.NewDeployWorkflow` with `usecase/deploy.NewDeployWorkflow`.
- `cli/internal/app/di.go` wires `build.NewGoBuilder` and passes `Build` into deploy usecase.

### Step 2: Generator split into domain + infra (DONE)
**Pure template logic (domain):**
- `cli/internal/generator/renderer.go` -> `cli/internal/domain/template/renderer.go`
- `cli/internal/generator/image_naming.go` -> `cli/internal/domain/template/image_naming.go`
- `cli/internal/generator/templates/*` -> `cli/internal/domain/template/templates/*`
- `cli/internal/generator/testdata/renderer/*` -> `cli/internal/domain/template/testdata/renderer/*`
- New: `cli/internal/domain/template/types.go` for `ParseResult`, `FunctionSpec`, `EventSpec`.

**SAM parsing boundary (infra):**
- `cli/internal/sam/*` -> `cli/internal/infra/sam/*`
- `cli/internal/generator/parser*.go` -> `cli/internal/infra/sam/template_*.go`
- `cli/internal/generator/intrinsics_resolver.go` -> `cli/internal/infra/sam/intrinsics_resolver.go`

**Build orchestration (infra):**
- `cli/internal/generator/*` -> `cli/internal/infra/build/*` (package renamed to `build`)
- `GenerateFiles` now returns `[]template.FunctionSpec` and consumes `sam.Parser`.

**Supporting types:**
- `cli/internal/manifest/*` -> `cli/internal/domain/manifest/*`
- `cli/internal/generator/value_helpers.go` -> `cli/internal/domain/value/value.go`

### Step 3: Assets relocation (DONE)
- `cli/internal/generator/assets/*` -> `cli/internal/infra/build/assets/*`
- Update compose/build contexts:
  - `docker-compose.*.yml` and `docker-bake.hcl` generator asset paths
  - `DefaultSitecustomizeSource` to `cli/internal/infra/build/assets/site-packages/sitecustomize.py`

### Step 4: Remaining boundary cleanups (TODO)
**4.1 FS abstraction (infra/fs)**
- Extract `ReadFile/WriteFile/Stat/MkdirAll/RemoveAll` into `infra/fs`.
- Apply to:
  - `infra/build/file_ops.go`
  - `infra/build/merge_config.go`
  - `command/deploy.go` (template path checks)

**4.2 Pure config extraction (domain/config)**
- Move pure merge/diff helpers out of usecase/build:
  - `mergeDefaultsSection`, `routeKey`, snapshot diff helpers
- Keep file I/O in `infra/build` or new `infra/fs`.

**4.3 Runtime mode helpers (domain/runtime)**
- Ensure mode normalization/inference are pure functions taking inputs (no env/Docker access).

## Concrete Dependencies for Deploy Usecase (Current Wiring)
Deploy usecase currently depends on:
- `func(build.BuildRequest) error` (builder function)
- `compose.CommandRunner` (docker compose exec)
- `ui.UserInterface` (output)
- `func(state.Context) error` (runtime env applier)

Target boundary (future): keep only Docker/Compose/FS/UI interfaces and pass concrete functions for the rest.

## Runtime Mode: UX + Inference Policy
### Policy (When project is running)
- **Do not prompt for mode**.
- **Infer mode from running project**, precedence:
  1. Running containers: `runtime-node` -> containerd
  2. Running containers: `agent` -> docker
  3. All containers in project (if none running)
  4. Compose config files (`config_files`) as fallback
- If user passes `--mode` and it conflicts, **warn and ignore**.

### Policy (No running project)
- Prompt for `--mode` (or accept flag).

## Compose Config Alignment
When a project is running, reuse the exact compose files that started it by reading
`com.docker.compose.project.config_files`.

Applied to:
- provisioner execution
- service running checks
- port discovery
- E2E port discovery

## Non-Goals
- Remove mode entirely (needed when no project is running).
- Fully remove hex architecture (external boundaries still matter).
- Large refactor in one step (prefer incremental migrations).

## Notes
- This document is a target architecture with concrete steps.
- Keep changes small and verify with `go test ./cli/...` after each step.
- Mapping details are tracked in `cli/docs/architecture-mapping.md`.
