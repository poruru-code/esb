<!--
Where: cli/docs/architecture-mapping.md
What: Mapping from current CLI structure to target architecture.
Why: Guide incremental refactors without losing intent.
-->
# CLI Architecture Mapping (Current -> Target)

This document maps the current CLI structure to the target architecture described in
`cli/docs/architecture-redesign.md`. It is intentionally pragmatic and supports
incremental migration.

## High-Level Mapping

| Current Area | Target Area | Action | Notes |
| --- | --- | --- | --- |
| `cli/internal/commands/*` | `cli/internal/command/*` | keep (thin) | Commands should only parse flags and call usecase/domain. |
| `cli/internal/workflows/*` | `cli/internal/usecase/deploy/*` | move (only deploy) | Keep deploy orchestration only. Other workflows should collapse. |
| `cli/internal/ports/*` | (remove) | remove | Replace with concrete infra implementations. |
| `cli/internal/wire/*` | `cli/internal/app/di.go` | reduce | Single small wiring point. |
| `cli/internal/generator/*` | `cli/internal/domain/*` + `cli/internal/infra/*` | split | Pure logic to domain, I/O to infra. |
| `cli/internal/compose/*` | `cli/internal/infra/compose/*` | keep | External dependency boundary. |
| `cli/internal/envutil/*` | `cli/internal/infra/env/*` | keep | Env access is I/O. |
| `cli/internal/helpers/*` | domain/infra | review | Move pure helpers to domain. |
| `cli/internal/interaction/*` | `cli/internal/infra/ui/*` | keep | UI is I/O. |
| `cli/internal/state/*` | `cli/internal/domain/*` | move | Only pure state models. |

## Concrete File/Function Mapping (Implementation-Level)
This section lists exact files/functions to move or split. Use this as a checklist.

### Commands -> Command/Domain/Infra
| Current File | Target | Concrete Action |
| --- | --- | --- |
| `cli/internal/commands/app.go` | `cli/internal/command/root.go` | Keep Kong parsing, env-file loading, and `dispatchCommand`. Move dependency wiring out. |
| `cli/internal/commands/deploy.go` | `cli/internal/command/deploy.go` | Keep input resolution and prompts only. Extract pure helpers to `domain/*` (see below). |
| `cli/internal/commands/deploy_runtime_env.go` | `cli/internal/command/deploy_runtime_env.go` + `infra/docker` | Keep prompt/decision logic in command; move Docker inspect into `infra/docker`. |
| `cli/internal/commands/output.go` | `cli/internal/infra/ui` | Merge with `ui.Console` + `interaction.HuhPrompter` under a single UI interface. |
| `cli/internal/commands/error_helpers.go` | `cli/internal/command/errors.go` | Keep as command-only helpers. |

### Workflow -> Usecase + Domain + Infra
| Current File | Target | Concrete Action |
| --- | --- | --- |
| `cli/internal/workflows/deploy.go` | `cli/internal/usecase/deploy/deploy.go` | Move orchestration as-is first. Then split: I/O helpers -> `infra`, pure logic -> `domain`. |
| `cli/internal/workflows/config_diff.go` | `cli/internal/domain/config/diff.go` + `infra/fs` | Keep diff logic pure. Move YAML load + file read into infra or usecase. |

### Compose/Docker -> Infra + Domain
| Current File | Target | Concrete Action |
| --- | --- | --- |
| `cli/internal/compose/*` | `cli/internal/infra/compose/*` | Keep `CommandRunner` concrete impl here. Expose only `ComposeRunner` interface to usecase. |
| `cli/internal/compose/docker.go` | `cli/internal/infra/docker/*` | Keep Docker SDK access here. Export small `DockerClient` interface only. |
| `cli/internal/compose/modes.go` | `cli/internal/domain/runtime/mode.go` | Move mode normalization and mode inference helpers (pure). |
| `cli/internal/compose/project_files.go` | `cli/internal/infra/compose/*` | Keep label-based compose file lookup (I/O). |

### Helpers/Env/State -> Domain + Infra
| Current File | Target | Concrete Action |
| --- | --- | --- |
| `cli/internal/helpers/env_defaults.go` | `cli/internal/infra/env` | All env var reads/writes are I/O. Split pure calculations (hash indices) into `domain/naming` or `domain/config`. |
| `cli/internal/helpers/mode.go` | `cli/internal/domain/runtime/mode.go` | Make it pure by passing existing mode as input instead of reading env. |
| `cli/internal/helpers/runtime_env.go` | `cli/internal/infra/env` | Keep as infra adapter until ports removal. |
| `cli/internal/state/context.go` | `cli/internal/domain/context.go` | Keep as a plain struct in domain. |
| `cli/internal/staging/staging.go` | `cli/internal/domain/staging` + `infra/env` | Make `RootDir` pure by passing in `home`/`xdg` from infra. |

### Generator -> Domain + Infra (Optional but recommended)
| Current File | Target | Concrete Action |
| --- | --- | --- |
| `cli/internal/generator/parser*.go` | `cli/internal/domain/template/*` | Keep SAM parsing and intrinsic resolution as pure logic. |
| `cli/internal/generator/renderer.go` | `cli/internal/domain/template/render.go` | Pure template rendering; no file I/O. |
| `cli/internal/generator/stage.go` + `file_ops.go` | `cli/internal/infra/fs` + `infra/build` | Move filesystem actions to `infra/fs`. Keep staging orchestration in `infra/build`. |
| `cli/internal/generator/go_builder*.go` | `cli/internal/infra/build` | Keep buildx/bake/docker operations as infra. |
| `cli/internal/generator/merge_config.go` | `domain/config/merge.go` + `infra/fs` | Merge logic in domain; file access in infra. |

## Concrete Extraction Targets (Pure Functions)
Move these functions into domain packages with no OS/Docker access:
- `normalizeMode`, `fallbackDeployMode`, `inferModeFromComposeFiles` -> `domain/runtime/mode.go`
- `resolveDeployOutputSummary`, `normalizeOutputDir` -> `domain/config/output.go`
- `buildTemplateSuggestions`, `updateTemplateHistory` -> `domain/template/history.go`
- `diffConfigSnapshots`, `diffMap`, `formatCountsLabel` -> `domain/config/diff.go`
- `sanitizeLayerName`, `imageSafeName`, `applyImageNames` -> `domain/template/naming.go`

## Commit-Sized Refactor Checklist
1. Create new folders (`command`, `usecase/deploy`, `domain/*`, `infra/*`) with thin adapters.
2. Move `workflows/deploy.go` to `usecase/deploy` with minimal edits.
3. Extract pure helpers into `domain/*` and update call sites.
4. Introduce `infra/docker`, `infra/compose`, `infra/fs`, `infra/ui` and switch usecase to them.
5. Remove `ports` by inlining dependency usage (use only the 4 infra interfaces).
6. Replace `wire` with `app/di.go` and update `main.go` imports.
7. Delete old packages once `go test ./cli/...` passes.

## Deploy-Specific Mapping

| Current Component | Target | Action | Notes |
| --- | --- | --- | --- |
| `workflows/deploy.go` | `usecase/deploy/deploy.go` | move | Keep orchestration. |
| `commands/deploy.go` | `command/deploy.go` | thin | Input parsing, prompt only. |
| `workflows/config_diff.go` | `domain/config/diff.go` | move | Pure diff logic. |
| `staging/*` | `domain/config/*` | move | If purely path logic, keep in domain. |

## Runtime Mode Inference
- **Target**: resolve from running project when available.
- **Source**: running container services (`runtime-node`/`agent`).
- **Fallback**: compose `config_files`.
- **Prompt**: only when no project is running.

## Incremental Plan
1. Create `usecase/deploy` with current workflow logic.
2. Move pure functions (merge/diff, template parsing) into `domain`.
3. Replace `ports` with concrete `infra/*` modules.
4. Keep only a small DI/wire entry point.

## Non-Goals
- Removing all abstraction (external boundaries stay).
- One-shot refactor.
