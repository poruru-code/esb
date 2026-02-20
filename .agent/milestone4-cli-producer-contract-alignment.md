# Milestone 4 Detail Plan: CLI Producer Contract Alignment

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md` and the parent plan `.agent/execplan-artifact-contract-realignment-master.md`.

## Purpose / Big Picture

Milestone 4 aligns the producer path (`cli`) with the post-freeze deploy artifact contract and reduces long-term maintenance cost by tightening ownership boundaries with `pkg/artifactcore`. The goal is not a superficial conditional cleanup; it is a boundary refactor that removes producer-side knowledge of core-internal construction rules where possible.

After this milestone, `cli` should focus on orchestration and template/output path decisions, while `pkg/artifactcore` should own deterministic manifest semantics (especially artifact entry identity rules) so contract evolution happens in one place.

## Progress

- [x] (2026-02-20 03:00Z) Created Milestone 4 detail plan.
- [x] (2026-02-20 03:05Z) Completed `cli` -> `pkg/artifactcore` call-contract inventory; identified producer-side ID construction as remaining boundary drift.
- [x] (2026-02-20 03:08Z) Implemented boundary refactor for artifact entry/manifest construction by centralizing deterministic ID normalization in `pkg/artifactcore.WriteArtifactManifest`.
- [x] (2026-02-20 03:10Z) Removed producer-side duplicated core logic: deleted direct `ComputeArtifactID` usage in `cli` production path and simplified related tests.
- [x] (2026-02-20 03:11Z) Added docs ownership map update in `docs/artifact-operations.md`.
- [x] (2026-02-20 03:13Z) Ran milestone validation tests and completed GO review.

## Surprises & Discoveries

- Observation: `cli/internal/command/deploy_artifact_manifest.go` still computes artifact IDs directly using `artifactcore.ComputeArtifactID`, which duplicates core ownership concerns in the producer path.
  Evidence: direct ID assignment when assembling `artifactcore.ArtifactEntry`.
- Observation: many CLI tests manually set `manifest.Artifacts[i].ID = ComputeArtifactID(...)`, which indicates repeated core-rule knowledge in caller-side fixtures.
  Evidence: `cli/internal/command/artifact_test.go`, `cli/internal/usecase/deploy/deploy_test.go`, and related test helpers.
- Observation: after refactor, `rg -n "ComputeArtifactID\\(" cli/internal` returns no matches.
  Evidence: producer path and usecase tests no longer perform direct ID derivation.

## Decision Log

- Decision: Treat deterministic artifact ID generation as core-owned behavior and minimize producer-side explicit ID construction.
  Rationale: ID rules are contract logic, and drift risk grows when every producer/test re-implements the rule.
  Date/Author: 2026-02-20 / Codex
- Decision: Keep `runtime_stack` emission optional in producer for now; this milestone focuses on boundary cleanup, not compatibility policy expansion.
  Rationale: Separates contract-shape refactor from policy activation and keeps blast radius manageable.
  Date/Author: 2026-02-20 / Codex
- Decision: normalize/sync deterministic IDs in `WriteArtifactManifest` before validation.
  Rationale: keeps strict read-time contract (`ReadArtifactManifest`) while removing duplicated producer-side ID assignment boilerplate.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 4 verdict: GO.

What was achieved:
- `cli/internal/command/deploy_artifact_manifest.go` no longer computes deterministic artifact IDs.
- `pkg/artifactcore/manifest.go` now syncs deterministic IDs in write path, and `pkg/artifactcore/manifest_test.go` locks this behavior.
- CLI-side tests were simplified by removing redundant direct ID assignment.
- `docs/artifact-operations.md` now has explicit boundary ownership wording for producer/core responsibilities.

Validation evidence:
- `GOWORK=off go -C pkg/artifactcore test ./...` passed.
- `go test ./cli/internal/command/... ./cli/internal/usecase/deploy/...` passed.
- `go test ./cli/...` passed.
- `go test ./tools/artifactctl/...` passed.

Residual risks:
- `WriteArtifactManifest` now normalizes IDs by design; any workflow relying on write-time mismatch failures should use `ReadArtifactManifest` or `manifest sync-ids --check` for explicit drift detection.

## Context and Orientation

Key files:

- Producer path:
  - `cli/internal/command/deploy_artifact_manifest.go`
  - `cli/internal/command/deploy_entry.go`
  - `cli/internal/command/artifact.go`
  - `cli/internal/usecase/deploy/deploy_runtime_provision.go`
- Shared core:
  - `pkg/artifactcore/manifest.go`
  - `pkg/artifactcore/execute.go`
  - `pkg/artifactcore/apply.go`
- Related docs:
  - `docs/deploy-artifact-contract.md`
  - `docs/artifact-operations.md`

Terms in this milestone:

- Producer orchestration logic: template iteration, output root resolution, user-facing command flow.
- Core contract logic: schema validation, deterministic ID rules, apply/merge semantics, strict-mode behavior.
- Boundary drift: same contract rule encoded in both producer and core with independent changes over time.

## Plan of Work

First, create an explicit inventory of values `cli` passes into `artifactcore` and classify each value as either producer-owned or core-owned.

Second, refactor manifest construction so deterministic artifact identity is generated/normalized inside core write path instead of explicit producer-side assignment. Producer still supplies required source fields (`source_template.path`, `sha256`, `parameters`) but should avoid duplicating identity rule implementation details.

Third, simplify caller-side tests that currently embed ID recalculation boilerplate, replacing it with core-owned normalization where appropriate.

Fourth, update operations/contract docs with a concise ownership map so future changes can be reviewed against a stable boundary.

## Concrete Steps

From repository root (`/home/akira/esb`):

1. Inventory call contract:
   - inspect producer files and capture `artifactcore` API usage.
2. Implement refactor:
   - update `pkg/artifactcore/manifest.go` write path behavior for deterministic ID normalization.
   - update `cli/internal/command/deploy_artifact_manifest.go` to remove direct ID construction.
3. Update tests:
   - `cli/internal/command/*_test.go`
   - `cli/internal/usecase/deploy/*_test.go` (only where ID boilerplate is redundant)
   - `pkg/artifactcore/*_test.go` to lock intended write/validate semantics.
4. Update docs:
   - `docs/artifact-operations.md` boundary ownership section.
5. Validate:
   - `GOWORK=off go -C pkg/artifactcore test ./...`
   - `go test ./cli/...`
   - `go test ./tools/artifactctl/...`

## Validation and Acceptance

Milestone 4 is accepted when:

- `cli` no longer directly computes deterministic artifact IDs in production path.
- deterministic ID behavior remains test-verified in `pkg/artifactcore`.
- no `core <- cli` reverse dependency is introduced.
- docs explicitly describe `cli` vs `pkg/artifactcore` ownership after refactor.
- milestone review verdict is GO with residual risks listed.

## Idempotence and Recovery

Refactor steps are safe to re-run. If any change causes behavioral ambiguity, revert only the latest file-scoped edits and keep tests green before proceeding.

For recovery:
- preserve generated fixture files by rerunning `e2e/scripts/regenerate_artifacts.sh` after core write-path changes;
- if write-path normalization changes expected IDs, update only affected fixtures and tests in the same commit.

## Artifacts and Notes

Expected artifacts for this milestone:

- updated `cli` producer manifest assembly code
- updated `pkg/artifactcore` write-path ownership behavior
- updated docs ownership map
- passing unit test evidence across core/cli/artifactctl packages

## Interfaces and Dependencies

Interface expectation at end of milestone:

- `cli` passes source facts to core (`source_template.path/sha256/parameters`, artifact root, runtime config dir).
- `pkg/artifactcore` owns deterministic ID normalization/consistency during manifest write/validation.
- callers may still enforce strict read-time validation via `ReadArtifactManifest`, while producer write path avoids duplicated identity logic.
