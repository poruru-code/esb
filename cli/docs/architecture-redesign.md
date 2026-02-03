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
