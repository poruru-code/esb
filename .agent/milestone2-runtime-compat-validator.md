# Milestone 2 Detail Plan: Runtime Compatibility Validator

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md` and the parent plan `.agent/execplan-artifact-contract-realignment-master.md`.

## Purpose / Big Picture

Milestone 2 introduces a runtime compatibility validator in shared core (`pkg/artifactcore`) so deploy can fail-fast when the running ESB stack is incompatible with the artifact contract. This milestone does not finalize all orchestration wiring, but it defines and implements the core data model, validation rules, and tests.

After Milestone 2, we will have a stable core API that answers: “given artifact requirements and observed runtime stack facts, is this deploy compatible?”

## Progress

- [x] (2026-02-20 00:41Z) Created milestone detail plan.
- [x] (2026-02-20 00:48Z) Defined runtime stack requirement schema in `pkg/artifactcore/manifest.go` (`runtime_stack.api_version/mode/esb_version`).
- [x] (2026-02-20 00:50Z) Implemented compatibility validator in `pkg/artifactcore/runtime_compat_validation.go` with strict/non-strict behavior.
- [x] (2026-02-20 00:52Z) Added UTs for success, warning, and hard-fail compatibility scenarios.
- [x] (2026-02-20 00:58Z) Wired `artifactctl` runtime observation probe into deploy preflight and propagated observation into `artifactcore.ExecuteApply`.
- [x] (2026-02-20 01:10Z) Wired `esb deploy` apply path runtime observation and propagated observation into `artifactcore.ExecuteApply`.
- [x] (2026-02-20 00:53Z) Completed milestone review and recorded GO with residual risks.

## Surprises & Discoveries

- Observation: Existing metadata (`runtime_meta.*`) is payload-oriented and cannot represent live stack compatibility by itself.
  Evidence: historical validator path relied on payload metadata checks; current compatibility logic is isolated in `pkg/artifactcore/runtime_compat_validation.go`.
- Observation: Compose definitions already use a common ESB tag (`${ESB_TAG}`) across first-party control-plane images.
  Evidence: `docker-compose.docker.yml` and `docker-compose.containerd.yml` image refs for gateway/agent/provisioner/runtime-node.
- Observation: Current `go.work` roots do not include `pkg/artifactcore`, so module-local tests require `GOWORK=off` when running directly inside that module.
  Evidence: `go test ./...` from `pkg/artifactcore` reports outside module roots unless `GOWORK=off` is set.

## Decision Log

- Decision: Introduce runtime compatibility requirements as a dedicated manifest section, separate from `runtime_meta`.
  Rationale: Keeps payload integrity validation and runtime compatibility validation independent and composable.
  Date/Author: 2026-02-20 / Codex
- Decision: Use a minimal first implementation keyed by ESB control-plane version/tag and deployment mode.
  Rationale: This is implementable with current runtime observability and aligns with current compose conventions.
  Date/Author: 2026-02-20 / Codex
- Decision: Keep `runtime_stack` optional in manifest during migration and emit no requirement by default until probe/wiring is complete.
  Rationale: Avoids breaking existing apply flows before Milestone 3 introduces runtime observation wiring.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 2 verdict: GO.

What was achieved:
- Added manifest schema support for runtime compatibility requirements via `runtime_stack`.
- Implemented shared validator in `pkg/artifactcore` and integrated it into `ExecuteApply` path.
- Added coverage for missing observation, mode mismatch, version mismatch, and API minor mismatch strict/non-strict outcomes.
- Added `artifactctl` deploy-side runtime probe (`docker ps` project/service inspection) and warning/error propagation behavior.
- Added `esb deploy` apply-side runtime observation wiring (docker-derived values first, request mode/tag fallback).

Validation evidence:
- `GOWORK=off go test ./...` (workdir: `pkg/artifactcore`) passed.
- `go test ./tools/artifactctl/...` passed.
- `go test ./cli/internal/command/... ./cli/internal/usecase/deploy/...` passed.

Residual risks:
- `artifactctl` probe currently depends on Docker CLI output shape and project-label inference; multi-project ambiguity handling needs hardening.
- `esb deploy` path now passes runtime observation, but it currently uses request mode/tag as fallback when docker observation is unavailable.
- Producer (`cli`) does not emit `runtime_stack` yet, so runtime compatibility is not enforced in normal default flow.

## Context and Orientation

Core files in scope:

- `pkg/artifactcore/manifest.go`
- `pkg/artifactcore/runtime_compat_validation.go`
- `pkg/artifactcore/execute.go`
- `pkg/artifactcore/errors.go`
- `pkg/artifactcore/*_test.go`

Integrator files referenced but not fully rewired in this milestone:

- `tools/artifactctl/pkg/deployops/execute.go`
- `tools/artifactctl/pkg/deployops/*`

Definitions used in this milestone:

- Runtime requirement: what artifact expects from running stack (mode/version baseline).
- Runtime observation: what deploy process can detect from running stack.
- Compatibility decision: pass / warn / fail outcome from comparing requirement vs observation.

## Plan of Work

First, extend artifact manifest schema with a dedicated runtime compatibility section. Initial contract fields are:

- `runtime_stack.api_version`
- `runtime_stack.mode`
- `runtime_stack.esb_version`

The values represent required deployment mode (`docker` or `containerd`) and expected first-party stack version/tag.

Second, add validator types and functions in `pkg/artifactcore`:

- `RuntimeObservation` type (mode, esb_version, source)
- `ValidateRuntimeCompatibility(manifest, observation, strict)` function returning warnings or error

Validation policy:

- requirement missing: validator is no-op (backward-safe for existing artifacts during migration)
- mode mismatch: hard fail
- `api_version` major mismatch: hard fail
- `api_version` minor mismatch: warning unless strict
- `esb_version` mismatch: warning unless strict (hard fail in strict)

Third, integrate validator invocation point in core apply execution path behind explicit observation input. If observation is absent, return warning in non-strict and hard fail in strict only when runtime_stack requirements are present.

Fourth, add unit tests covering all transitions.

## Concrete Steps

From repository root (`/home/akira/esb`):

1. Edit manifest schema and related normalize/validate methods:
   - `pkg/artifactcore/manifest.go`
2. Add compatibility validator implementation:
   - `pkg/artifactcore/runtime_compat_validation.go` (new)
   - update `pkg/artifactcore/execute.go` / `apply.go` as needed
3. Add/adjust tests:
   - `pkg/artifactcore/runtime_compat_validation_test.go` (new)
   - `pkg/artifactcore/execute_test.go`
   - `pkg/artifactcore/manifest_test.go`
4. Run tests:
   - `go test ./pkg/artifactcore/...`

## Validation and Acceptance

Milestone 2 is accepted when:

- manifest can express runtime compatibility requirements without overloading `runtime_meta`
- validator behavior is deterministic for strict/non-strict modes
- all new validator paths are covered by unit tests
- review verdict is GO with residual integration risks documented

## Idempotence and Recovery

Schema and validator changes are additive in this milestone. If migration conflicts appear, keep new fields optional and preserve read compatibility for existing manifests.

## Artifacts and Notes

Expected artifacts:

- New/updated validator source under `pkg/artifactcore`
- Updated manifest schema and tests
- Review verdict + risk list updates in this file and master plan

## Interfaces and Dependencies

Target interface for this milestone (subject to implementation naming):

- `type RuntimeObservation struct { Mode string; ESBVersion string; Source string }`
- `func ValidateRuntimeCompatibility(manifest ArtifactManifest, obs RuntimeObservation, strict bool) ([]string, error)`

No dependency from `pkg/artifactcore` to `cli/internal/*` is allowed.

## Milestone 2 Plan Review (Architectural)

Verdict: GO.

Rationale:
- Scope is constrained to core data model and validation, which is the correct prerequisite before orchestration rewiring.
- The plan preserves backward safety by making new requirements optional during migration.
- Responsibility split remains clean: core validates, adapters/probes observe runtime.

Residual risks:
- Exact runtime observation source (docker inspect vs env endpoint) is deferred to Milestone 3 and may affect error fidelity.
- Version normalization rules (tag vs semver) must be fixed before enforcing strict defaults broadly.

Revision note (2026-02-20): Initial Milestone 2 plan created and reviewed with GO verdict.
