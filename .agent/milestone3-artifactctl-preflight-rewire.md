# Milestone 3 Detail Plan: artifactctl Preflight Rewire

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md` and the parent plan `.agent/execplan-artifact-contract-realignment-master.md`.

## Purpose / Big Picture

Milestone 3 rewires `artifactctl deploy` so runtime compatibility is evaluated at preflight time and deploy no longer depends on artifact-local `runtime-base/**` as a lambda base source. This milestone enforces the frozen contract boundary in runtime behavior.

After this milestone, `artifactctl deploy` fails earlier with compatibility errors, and image preparation uses live runtime environment hints (`runtime observation` / `ESB_TAG`) instead of artifact-time runtime-base inputs.

## Progress

- [x] (2026-02-20 03:11Z) Backfilled this detail plan to match already implemented Milestone 3 changes.
- [x] (2026-02-20 00:58Z) Wired runtime observation probe into `artifactctl` deploy preflight and propagated observation into `artifactcore.ExecuteApply`.
- [x] (2026-02-20 11:20Z) Removed artifact-local runtime-base dependency from prepare path.
- [x] (2026-02-20 11:20Z) Added lambda-base tag rewrite flow driven by runtime observation/`ESB_TAG`.
- [x] (2026-02-20 03:11Z) Verified deployops unit tests pass with rewired preflight flow.

## Surprises & Discoveries

- Observation: runtime compatibility probe quality depends on docker compose project/service detection fidelity.
  Evidence: probe path inspects running containers and labels; multi-project overlap can degrade signal quality.
- Observation: removing artifact-local runtime-base dependency required Dockerfile rewrite safeguards to keep authored explicit non-`latest` tags intact.
  Evidence: deployops tests cover rewrite behavior and explicit tag preservation.

## Decision Log

- Decision: keep runtime observation as preflight input into shared core validator instead of embedding compatibility logic inside deployops.
  Rationale: preserves `pkg/artifactcore` as single validation authority.
  Date/Author: 2026-02-20 / Codex
- Decision: lambda base selection prioritizes observed runtime version/tag and falls back to `ESB_TAG`.
  Rationale: aligns deploy with current runtime environment while preserving deterministic fallback.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 3 verdict: GO.

What was achieved:
- `artifactctl deploy` preflight now includes runtime observation and compatibility enforcement path.
- artifact-local `runtime-base/**` is no longer a hard dependency for prepare path.
- function image build remains supported while base-tag selection is runtime-aligned.

Residual risks:
- observation heuristics still depend on docker inspection assumptions; hardening for ambiguous compose projects remains future work.

## Context and Orientation

Key files:

- `tools/artifactctl/pkg/deployops/execute.go`
- `tools/artifactctl/pkg/deployops/runtime_probe.go`
- `tools/artifactctl/pkg/deployops/prepare_images.go`
- `tools/artifactctl/pkg/deployops/*_test.go`
- `pkg/artifactcore/runtime_compat_validation.go`

## Plan of Work

This milestone was executed as:

1. insert runtime observation probe before apply execution;
2. propagate observation into shared core apply input;
3. replace artifact-local base dependency with runtime-derived tag rewrite in image preparation;
4. verify deployops unit tests for probe, rewrite, and execution paths.

## Concrete Steps

From repository root (`/home/akira/esb`), validation command:

- `go test ./tools/artifactctl/...`

## Validation and Acceptance

Accepted when:

- runtime compatibility probe participates in deploy preflight;
- apply receives runtime observation through shared core API;
- prepare path no longer assumes artifact-local `runtime-base/**`;
- deployops test suite passes.

## Idempotence and Recovery

Changes are idempotent for repeated test runs. If probe behavior regresses, fallback is to stub `RuntimeProbe` in deployops tests and isolate failures before re-enabling default probe.

## Artifacts and Notes

Primary implementation evidence lives in:

- `tools/artifactctl/pkg/deployops/execute.go`
- `tools/artifactctl/pkg/deployops/runtime_probe.go`
- `tools/artifactctl/pkg/deployops/prepare_images.go`

## Interfaces and Dependencies

Milestone 3 interface contract:

- deployops orchestrates probe/build/apply;
- compatibility decision remains in `pkg/artifactcore`;
- no reverse dependency from core to adapter layers.
