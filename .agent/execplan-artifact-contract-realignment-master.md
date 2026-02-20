# Artifact Contract Realignment Master Plan

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

The current deploy-artifact contract is mixing two different concerns: artifact reproducibility and runtime stack compatibility. That mismatch causes false confidence, because preserving Dockerfiles inside artifacts does not prove the runtime stack is compatible at apply time. After this change, we will separate these concerns explicitly: artifact contract will define only what is needed to apply artifacts, while runtime stack compatibility will be validated at execution time using explicit version requirements.

A user-visible result will be: `artifactctl deploy` fails fast with a clear compatibility error when gateway/provisioner/runtime versions are incompatible, instead of failing later during image pull/build/provision with opaque errors.

## Progress

- [x] (2026-02-20 00:25Z) Created this master plan and fixed the execution flow to `master plan -> milestone detail plan -> implementation`.
- [x] (2026-02-20 00:25Z) Created Milestone 1 detail plan file: `.agent/milestone1-contract-target-definition.md`.
- [x] (2026-02-20 00:37Z) Defined final boundary between `artifact payload contract` and `runtime stack compatibility contract`; updated `docs/deploy-artifact-contract.md` and `docs/artifact-operations.md`.
- [x] (2026-02-20 00:41Z) Created Milestone 2 detail plan (`.agent/milestone2-runtime-compat-validator.md`) with concrete interface and migration strategy.
- [x] (2026-02-20 03:11Z) Backfilled Milestone 3 detail plan (`.agent/milestone3-artifactctl-preflight-rewire.md`) to match implemented preflight rewire scope.
- [x] (2026-02-20 00:53Z) Implemented runtime compatibility requirement schema + validator in `pkg/artifactcore` and added UT coverage.
- [x] (2026-02-20 00:58Z) Wired runtime observation probe in `tools/artifactctl` deploy preflight and fed observation into `artifactcore.ExecuteApply`.
- [x] (2026-02-20 01:10Z) Wired equivalent runtime observation path for `esb deploy` flow (CLI usecase path).
- [x] (2026-02-20 10:35Z) Contract freeze updated: removed `runtime-base/**` from deploy artifact contract layout/requirements and fixed responsibility wording (`artifactctl deploy` may build images, but lambda base source is runtime environment, not artifact-time runtime-base).
- [x] (2026-02-20 11:20Z) Reworked `artifactctl` prepare path: removed artifact-local runtime-base build dependency, added lambda-base tag rewrite from runtime observation/`ESB_TAG`, and kept function image build/push flow.
- [x] (2026-02-20 11:20Z) Simplified `artifactcore` runtime metadata validation by removing artifact-local runtime digest checks (`python_sitecustomize_digest`) from apply gating.
- [x] (2026-02-20 03:00Z) Created Milestone 4 detail plan file: `.agent/milestone4-cli-producer-contract-alignment.md` and fixed milestone scope for boundary-focused refactor.
- [x] (2026-02-20 03:13Z) Completed Milestone 4 producer/core boundary refactor: removed direct ID construction from `cli`, centralized deterministic ID normalization in `pkg/artifactcore` write path, and added boundary ownership docs.
- [x] (2026-02-20 03:16Z) Created Milestone 5 detail plan file: `.agent/milestone5-e2e-contract-refresh.md`.
- [x] (2026-02-20 03:25Z) Completed Milestone 5 fixture/E2E refresh: regenerated artifacts and passed runner contract tests (`68 passed`).
- [x] (2026-02-20 03:28Z) Created Milestone 6 detail plan file: `.agent/milestone6-cleanup-and-final-verification.md`.
- [x] (2026-02-20 03:32Z) Completed Milestone 6 cleanup/final verification: stale-marker audit clean, full UT pass, and full E2E pass from clean Docker state (`ALL MATRIX ENTRIES PASSED`).
- [x] (2026-02-20 03:13Z) Contract-change follow-up (phase 1) completed for `cli` vs `pkg/artifactcore`: removed duplicated ID logic, updated tests, and documented ownership map in operations guide.

## Surprises & Discoveries

- Observation: 旧契約では `runtime-base` を deploy 入力と互換性根拠の両方に使っていた。
  Evidence: freeze 前の `docs/deploy-artifact-contract.md` で runtime-base 必須と digest 検証が混在していた。
- Observation: `artifactctl deploy` already treats artifact root as read-only, but compatibility still depends on artifact-contained files rather than live runtime stack versions.
  Evidence: `tools/artifactctl/pkg/deployops/prepare_images.go` now uses temporary workspaces, yet preflight logic still derives base-image readiness from artifact file layout.

## Decision Log

- Decision: Use a two-layer contract model (`artifact apply contract` and `runtime stack compatibility contract`) instead of one overloaded contract.
  Rationale: This aligns with the user goal that compatibility must be checked at execution time, not inferred from preserved build files.
  Date/Author: 2026-02-20 / Codex
- Decision: For large-scope work, create a master plan first and create per-milestone detail plans before implementing each milestone.
  Rationale: Prevents drifting implementation and keeps reviews focused on one boundary change at a time.
  Date/Author: 2026-02-20 / Codex
- Decision: Treat `runtime_meta` as payload metadata only, and model runtime stack compatibility as a distinct preflight contract.
  Rationale: Aligns contract semantics with user requirement that compatibility must be validated against running services, not inferred from artifact-contained build assets.
  Date/Author: 2026-02-20 / Codex
- Decision: Supersede digest-driven payload checks that depend on `runtime-base/**`; deploy contract is now runtime-base independent.
  Rationale: Keep manual artifact producer requirements minimal and avoid coupling apply correctness to artifact-local build assets.
  Date/Author: 2026-02-20 / Codex
- Decision: Implement runtime observation in `artifactctl` first via docker compose project/service inspection and keep `runtime_stack` optional until CLI path is wired.
  Rationale: Enables incremental rollout without breaking existing apply flows while preserving strict-mode semantics when requirements are present.
  Date/Author: 2026-02-20 / Codex
- Decision: In CLI apply path, runtime observation uses docker-derived values first and request mode/tag as fallback.
  Rationale: Keeps deploy robust when docker observation is unavailable while still preferring live stack facts.
  Date/Author: 2026-02-20 / Codex
- Decision: `runtime-base/**` is excluded from Deploy Artifact Contract, and `artifactctl deploy` must not rely on artifact-local runtime-base/image-prepare logic.
  Rationale: Runtime compatibility must be determined from live stack observation, and artifact contract must stay minimal for manual producers.
  Date/Author: 2026-02-20 / Codex
- Decision: deterministic artifact ID normalization is core-owned in `WriteArtifactManifest`; producer path should not compute IDs directly.
  Rationale: avoids boundary drift and keeps ID rule changes localized to `pkg/artifactcore`.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestones 1-6 completed with GO verdict.

Completed outcomes:
- Contract language now explicitly separates payload integrity from runtime stack compatibility.
- Operations guide phase model now includes runtime compatibility as a distinct phase.
- Contract fixed point established: `runtime-base/**` excluded from deploy artifact contract and `artifactctl deploy` responsibility clarified as validation + apply (必要時 build/pull あり、base は実行時環境由来)。
- Producer/core boundary tightened: `cli` no longer computes deterministic artifact IDs directly; normalization is centralized in `pkg/artifactcore` write path and documented.
- Final cleanup and verification complete: stale-marker audit result is intentional-only, full UT pass, and full E2E matrix pass on clean Docker state.

Residual risks:
- Host-level concurrent Docker operations can still disturb E2E execution; enforce isolated runner/host policy for deterministic CI and local verification.

## Context and Orientation

This repository currently splits responsibilities across these areas:

- `cli/`: template-driven producer and composite deploy command.
- `tools/artifactctl/`: artifact-first applier (`deploy` command).
- `pkg/artifactcore/`: shared apply/merge/validation logic.
- `e2e/`: artifact fixtures, runner, and contract tests.
- `docs/deploy-artifact-contract.md` and `docs/artifact-operations.md`: operational and schema contract documents.

The specific architecture issue is that compatibility is currently encoded partly as “artifact contains certain Docker/build assets.” That is not equivalent to “the running stack is compatible now.” We need explicit runtime version requirements and runtime checks, while keeping artifact payload requirements minimal and bounded to deploy-time inputs.

## Plan of Work

We will execute in six milestones. Each milestone must have its own detailed ExecPlan before code changes begin.

Milestone 1 defines target contracts and migration policy. It will produce final schema direction and validation rules in prose, without broad code movement.

Milestone 2 introduces runtime stack compatibility validation in shared core (`pkg/artifactcore`) with precise error classification and strict/non-strict handling.

Milestone 3 updates `artifactctl deploy` to execute the new validation flow at deterministic preflight points and remove incompatible assumptions that artifact file presence alone guarantees compatibility.
Milestone 3 also removes artifact-local image prepare/runtime-base dependency from deploy path so implementation matches the frozen contract.

Milestone 4 updates `cli` producer outputs and metadata emission so artifacts are deploy-focused while still carrying enough information for apply-time checks.

Milestone 5 updates E2E fixtures, regenerate scripts, and contracts so tests validate the new boundary model and no legacy assumptions remain.

Milestone 6 removes dead options/fields/docs, executes explicit `cli` vs `pkg/artifactcore` boundary re-audit after contract changes, runs final full validation, and records retrospective outcomes.

## Boundary Invariants (Must Not Change)

- Deploy artifact contract must not include `runtime-base/**`.
- `artifactctl deploy` must not perform artifact generation.
- `artifactctl deploy` may perform image build/pull, but must not use artifact-time `runtime-base/**` as lambda base source.
- Compatibility decisions must come from runtime observation, not artifact-contained Docker/build assets.
- Any change that violates these invariants is automatic NO-GO.

## Boundary Re-Audit Scope (Contract-Change Follow-Up)

The final cleanup milestone must include an explicit architectural re-audit for `cli` and `pkg/artifactcore`:

- list all parameters passed from `cli` to `pkg/artifactcore` and remove those no longer needed by the new contract;
- identify duplicate logic that exists in both layers and keep only one owner;
- verify that `pkg/artifactcore` contains shared deploy/apply core behavior only, while `cli` remains orchestration/adaptor only;
- document the final boundary and ownership map in docs so future contract edits do not reintroduce drift.

Detailed plan files to be maintained:

- `.agent/milestone1-contract-target-definition.md`
- `.agent/milestone2-runtime-compat-validator.md`
- `.agent/milestone3-artifactctl-preflight-rewire.md`
- `.agent/milestone4-cli-producer-contract-alignment.md`
- `.agent/milestone5-e2e-contract-refresh.md` (to be created before M5 implementation)
- `.agent/milestone6-cleanup-and-final-verification.md` (to be created before M6 implementation)

## Concrete Steps

From repository root (`/home/akira/esb`):

1. Draft and review Milestone 5 detail plan:
   - create `.agent/milestone5-e2e-contract-refresh.md`
2. Implement Milestone 5 (fixture regeneration + E2E contract refresh):
   - `e2e/scripts/regenerate_artifacts.sh`
   - `e2e/artifacts/*`
   - `e2e/environments/test_matrix.yaml` and related runner/contracts if needed
3. Draft and review Milestone 6 detail plan:
   - create `.agent/milestone6-cleanup-and-final-verification.md`
4. Implement Milestone 6:
   - remove obsolete options/fields/docs
   - run full UT and full E2E from clean Docker state
   - update `Progress`, `Decision Log`, and `Outcomes & Retrospective`.

Expected review output for each milestone:

- explicit GO/NO-GO
- unresolved risks list
- update decision log before coding

## Validation and Acceptance

For the planning phase, acceptance is:

- master plan exists and is self-contained
- milestone detail plan exists before implementation begins
- boundary terms are explicit: artifact contract vs runtime compatibility contract

For implementation milestones, acceptance will require:

- relevant unit tests in `pkg/artifactcore`, `tools/artifactctl`, and `cli` passing
- full E2E pass from clean Docker state
- regenerated artifacts validated and documented

## Idempotence and Recovery

Planning edits are idempotent: re-running this step only updates markdown docs under `.agent/`. If milestone scope changes, update both the master and affected milestone detail file, and record why in the `Decision Log` sections.

## Artifacts and Notes

Primary planning artifacts in this phase:

- `.agent/execplan-artifact-contract-realignment-master.md`
- `.agent/milestone1-contract-target-definition.md`

## Interfaces and Dependencies

The implementation phase will constrain interfaces around:

- `pkg/artifactcore` as shared compatibility/validation core
- `tools/artifactctl` as thin command adapter + orchestrator
- `cli` as producer/composite command without reverse dependency from core

No component may introduce `core <- cli` reverse dependency.

Revision note (2026-02-20): Marked Milestone 1 complete (GO), recorded contract boundary decisions, and promoted Milestone 2 detail planning as the next required gate.
Revision note (2026-02-20 02:51Z): Added explicit post-contract cleanup/boundary re-audit scope for `cli` and `pkg/artifactcore` to prevent responsibility drift after schema/contract changes.
Revision note (2026-02-20 03:00Z): Added Milestone 4 detail plan and constrained M4 scope to structural boundary refactor (not just conditional cleanup).
Revision note (2026-02-20 03:11Z): Added missing Milestone 3 detail plan file to restore milestone-to-plan traceability with implemented artifactctl preflight rewire.
Revision note (2026-02-20 03:13Z): Marked Milestone 4 producer/core boundary refactor complete (GO) with deterministic ID ownership centralized in `pkg/artifactcore`.
Revision note (2026-02-20 03:25Z): Marked Milestone 5 fixture/E2E contract refresh complete (GO) after regeneration and runner contract tests.
Revision note (2026-02-20 03:32Z): Marked Milestone 6 final cleanup/verification complete (GO) after full UT + full clean-state E2E pass.
