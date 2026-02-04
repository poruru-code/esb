<!--
Where: cli/docs/architecture-mapping.md
What: Mapping from current CLI structure to target architecture.
Why: Guide incremental refactors without losing intent.
-->
# CLI Architecture Mapping (Legacy -> Current)

This document records concrete file moves and callsite updates used to reach the target architecture in
`cli/docs/architecture-redesign.md`. Use it as a checklist for future cleanups.

## High-Level Mapping

| Legacy Area | Current Area | Status | Notes |
| --- | --- | --- | --- |
| `cli/internal/commands/*` | `cli/internal/command/*` | done | Commands are thin flag parsers + prompts. |
| `cli/internal/workflows/*` | `cli/internal/usecase/deploy/*` | done | Only deploy orchestration remains. |
| `cli/internal/ports/*` | (removed) | done | Replace with concrete infra packages. |
| `cli/internal/wire/*` | `cli/internal/app/di.go` | done | Single wiring point. |
| `cli/internal/generator/*` | `cli/internal/infra/build/*` + `cli/internal/domain/template/*` | done | Split pure template logic vs build I/O. |
| `cli/internal/sam/*` | `cli/internal/infra/sam/*` | done | External parser boundary. |
| `cli/internal/manifest/*` | `cli/internal/domain/manifest/*` | done | Pure internal specs. |
| `cli/internal/state/*` | `cli/internal/domain/state/*` | done | Plain DTOs. |
| `cli/internal/compose/*` | `cli/internal/infra/compose/*` | done | External dependency boundary. |
| `cli/internal/envutil/*` | `cli/internal/infra/envutil/*` | done | Env access is I/O. |
| `cli/internal/interaction/*` | `cli/internal/infra/interaction/*` | done | Prompt implementation. |

## Concrete File Mapping (Implementation-Level)

| Legacy File/Dir | New Location | Status | Notes |
| --- | --- | --- | --- |
| `cli/internal/generator/renderer.go` | `cli/internal/domain/template/renderer.go` | done | Pure rendering. |
| `cli/internal/generator/image_naming.go` | `cli/internal/domain/template/image_naming.go` | done | Pure naming. |
| `cli/internal/generator/templates/*` | `cli/internal/domain/template/templates/*` | done | Template assets. |
| `cli/internal/generator/testdata/renderer/*` | `cli/internal/domain/template/testdata/renderer/*` | done | Snapshot fixtures updated. |
| `cli/internal/generator/parser*.go` | `cli/internal/infra/sam/template_*.go` | done | SAM parsing + defaults. |
| `cli/internal/generator/intrinsics_resolver.go` | `cli/internal/infra/sam/intrinsics_resolver.go` | done | Intrinsic resolution. |
| `cli/internal/sam/parser.go` | `cli/internal/infra/sam/parser.go` | done | DecodeYAML/ResolveAll wrappers. |
| `cli/internal/sam/template.go` | `cli/internal/infra/sam/template.go` | done | Schema decode helpers. |
| `cli/internal/manifest/*` | `cli/internal/domain/manifest/*` | done | Internal resource specs. |
| `cli/internal/generator/value_helpers.go` | `cli/internal/domain/value/value.go` | done | `AsMap/AsSlice/AsString/AsInt` helpers. |
| `cli/internal/generator/*` | `cli/internal/infra/build/*` | done | `package build` + new imports. |
| `cli/internal/generator/assets/*` | `cli/internal/infra/build/assets/*` | done | Update compose + bake contexts. |

## Required Call-Site Updates
When applying the above moves, update imports and types:
- `generator.BuildRequest` -> `build.BuildRequest`
- `generator.NewGoBuilder` -> `build.NewGoBuilder`
- `generator.ParseResult` / `FunctionSpec` -> `template.ParseResult` / `template.FunctionSpec`
- `RenderFunctionsYml` / `RenderRoutingYml` -> `template.RenderFunctionsYml` / `template.RenderRoutingYml`
- `DefaultSitecustomizeSource` now lives in `domain/template` and points to
  `cli/internal/infra/build/assets/site-packages/sitecustomize.py`.

## Remaining Cleanups (Optional)
- Introduce `infra/fs` and replace direct `os`/`filepath` usage in build/merge/staging.
- Move pure config helpers from usecase/build into `domain/config`.
- Keep domain functions free of external dependencies.

## Commit-Sized Refactor Checklist
1. Move one package at a time (renderer, parser, build, assets).
2. Update callsites + tests immediately.
3. Run `go test ./cli/...` after each move.
