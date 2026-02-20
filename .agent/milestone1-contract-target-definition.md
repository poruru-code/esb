# Milestone 1 Detail Plan: Contract Target Definition

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md` and the parent plan `.agent/execplan-artifact-contract-realignment-master.md`.

## Purpose / Big Picture

Milestone 1 will define the exact contract target we are validating during deploy. The outcome is a clear and implementable separation between:

- artifact payload requirements (what files/metadata must exist to apply)
- runtime stack compatibility requirements (what running service versions/capabilities must be validated at execution time)

After this milestone, implementation teams can code against an unambiguous boundary, avoiding further drift where artifact contents are treated as a proxy for runtime compatibility.

## Progress

- [x] (2026-02-20 00:25Z) Created milestone detail plan.
- [x] (2026-02-20 00:30Z) Audited current contract wording and code behavior for boundary violations.
- [x] (2026-02-20 00:35Z) Updated contract definitions in `docs/deploy-artifact-contract.md` and `docs/artifact-operations.md` with explicit payload/stack split.
- [x] (2026-02-20 00:37Z) Completed Milestone 1 review and recorded GO with residual risks.

## Surprises & Discoveries

- Observation: Current docs encode strict digest checks from artifact-local runtime hook files, which may conflate deploy input verification and stack compatibility.
  Evidence: `docs/deploy-artifact-contract.md` strict failure list and digest policy.
- Observation: Runtime metadata validation in core reads only artifact payload files and has no runtime stack probe.
  Evidence: `pkg/artifactcore/runtime_meta_validation.go` validates `artifact_root/runtime-base/runtime-hooks/python/**` digests and `api_version`, but does not query running gateway/agent/provisioner/runtime-node.
- Observation: `artifactctl deploy` currently executes `prepareImages -> ExecuteApply` and therefore validates payload integrity, not live stack compatibility.
  Evidence: `tools/artifactctl/pkg/deployops/execute.go` calls `prepareImages(...)` then `artifactcore.ExecuteApply(...)`, with no stack introspection step.
- Observation: Producer writes runtime meta from repo-local runtime hooks hash, which is correct for payload integrity but not sufficient for runtime compatibility assertions.
  Evidence: `cli/internal/command/deploy_artifact_manifest.go` computes digest from `runtime-hooks/python/sitecustomize/site-packages/sitecustomize.py`.

## Decision Log

- Decision: Keep Milestone 1 implementation doc-first (contract and migration policy), with no broad refactor until terms are finalized.
  Rationale: Prevents coding to moving targets and reduces rework.
  Date/Author: 2026-02-20 / Codex
- Decision: Define two explicit contracts: Payload Apply Contract (implemented now) and Runtime Stack Compatibility Contract (implemented in later milestones).
  Rationale: Removes the false implication that artifact-contained Dockerfiles/runtime-base alone prove live stack compatibility.
  Date/Author: 2026-02-20 / Codex
- Decision: Keep `runtime_meta.*` as payload integrity metadata and do not reinterpret it as live compatibility proof.
  Rationale: Preserves current behavior while enabling a separate runtime compatibility probe path in M2/M3.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 1 verdict: GO.

What was achieved:
- Contract boundary was made explicit in docs: payload integrity and runtime stack compatibility are now separate concepts with separate failure semantics.
- Operational flow now includes a dedicated runtime compatibility phase (declared contract-first, phased implementation).
- Failure policy now labels payload digest failures as payload integrity checks, preventing architectural misunderstanding.

Residual risks carried to next milestones:
- Runtime stack compatibility preflight is contract-defined but not yet implemented in code.
- Artifact schema has no dedicated `runtime_stack_requirements` field yet; introduction and migration are pending.
- E2E does not yet assert stack compatibility mismatch behavior.
- Contract freeze follow-up: `runtime-base/**` exclusion and apply-only responsibility (`artifactctl deploy`) are now documented and must be implemented in Milestone 3+.

## Context and Orientation

Files to read and align in this milestone:

- `docs/deploy-artifact-contract.md`
- `docs/artifact-operations.md`
- `tools/artifactctl/pkg/deployops/prepare_images.go`
- `pkg/artifactcore/runtime_meta_validation.go`
- `cli/internal/command/deploy_artifact_manifest.go`

Terms used in this milestone:

- Artifact payload contract: the schema and file set required to execute apply/build operations from artifact inputs.
- Runtime stack compatibility contract: required runtime service versions/capabilities checked at deploy execution.
- Boundary violation: logic or docs that use one contract to represent the other.

## Plan of Work

First, map current behaviors into a two-column matrix (payload requirement vs stack compatibility check) and mark violations. Next, rewrite contract docs so each rule is assigned to one side only, including strict/warn policy and failure classes.

Then define migration policy for existing artifacts and commands, including schema version strategy and fallback policy. Finally, run a design review (GO/NO-GO) against the user objective: `artifactctl` must remain deploy-focused and must not become an artifact generator.

## Concrete Steps

From repository root (`/home/akira/esb`):

1. Read current contracts and deploy path code.
2. Draft revised sections in:
   - `docs/deploy-artifact-contract.md`
   - `docs/artifact-operations.md`
3. Add a short migration note in master plan decision log.
4. Record milestone review output with unresolved risks.

## Validation and Acceptance

Milestone 1 is accepted when:

- every contract rule is classified as either payload or runtime compatibility
- no ambiguous rule remains in docs
- migration policy is explicit enough for coding milestones
- review verdict is GO with named residual risks

## Idempotence and Recovery

Doc updates can be rerun safely. If review returns NO-GO, revert only the unapproved doc edits and keep decision log entries with reasons.

## Artifacts and Notes

Expected artifacts:

- Updated `docs/deploy-artifact-contract.md`
- Updated `docs/artifact-operations.md`
- Updated decision/progress entries in both plan files

## Interfaces and Dependencies

Milestone 1 must not change public command behavior yet. It only defines the target interface and validation contract that later milestones will implement.

Revision note (2026-02-20): Completed Milestone 1 with GO verdict; boundary split finalized in docs and residual implementation risks explicitly tracked.
