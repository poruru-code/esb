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
