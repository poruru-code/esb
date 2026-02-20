# Milestone 5 Detail Plan: E2E Contract Refresh

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md` and the parent plan `.agent/execplan-artifact-contract-realignment-master.md`.

## Purpose / Big Picture

Milestone 5 refreshes E2E fixture artifacts and validates that runner-side contract checks align with the current deploy artifact contract (no `runtime-base/**` as contract input, no legacy matrix driver fields, artifact-first flow only).

After this milestone, committed E2E fixture manifests and E2E contract tests will reflect the latest schema/ownership decisions, reducing drift between docs, fixtures, and test runtime.

## Progress

- [x] (2026-02-20 03:16Z) Created Milestone 5 detail plan.
- [x] (2026-02-20 03:21Z) Regenerated E2E artifacts from current CLI producer path via `e2e/scripts/regenerate_artifacts.sh`.
- [x] (2026-02-20 03:22Z) Verified regenerated `artifact.yml` fixtures satisfy current contract shape (`runtime_meta` absent, minimal deploy fields only).
- [x] (2026-02-20 03:24Z) Ran E2E runner contract/unit tests (`68 passed`) with required env vars set.
- [x] (2026-02-20 03:25Z) Completed milestone review and recorded GO verdict with residual risks.

## Surprises & Discoveries

- Observation: current committed fixture manifests are already runtime_meta-free, but regeneration is still needed as a contract drift guard.
  Evidence: `e2e/artifacts/e2e-{docker,containerd}/artifact.yml` have only schema/project/env/mode/artifacts/source/generator fields.
- Observation: direct `uv run pytest e2e/runner/tests` is blocked by global E2E `conftest` env checks.
  Evidence: requires `X_API_KEY`, `AUTH_USER`, and `AUTH_PASS`; tests pass once set.

## Decision Log

- Decision: regenerate fixtures using local CLI command (`ESB_CMD='go -C cli run ./cmd/esb'`) instead of relying on prebuilt binary state.
  Rationale: keeps fixture generation deterministic from repository source at current commit.
  Date/Author: 2026-02-20 / Codex
- Decision: keep refreshed fixture timestamp updates (`generated_at`) as-is after regeneration.
  Rationale: regeneration is the source-of-truth operation and timestamp drift is expected artifact metadata.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 5 verdict: GO.

What was achieved:
- fixture artifacts were regenerated from current producer implementation;
- contract shape remained aligned (no `runtime_meta`, no runtime-base payload dependencies);
- runner contract tests passed.

Validation evidence:
- `ESB_CMD='go -C cli run ./cmd/esb' ./e2e/scripts/regenerate_artifacts.sh` succeeded for docker/containerd.
- `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner/tests -q` => `68 passed`.

Residual risks:
- fixture regeneration currently updates `generated_at`, which can create low-signal diffs; consider deterministic timestamp policy in future cleanup if diff churn becomes problematic.

## Context and Orientation

Key files in scope:

- `e2e/scripts/regenerate_artifacts.sh`
- `e2e/artifacts/e2e-docker/artifact.yml`
- `e2e/artifacts/e2e-containerd/artifact.yml`
- `e2e/artifacts/README.md`
- `e2e/runner/config.py`
- `e2e/runner/tests/test_config.py`

Contract points validated in this milestone:

- fixture manifests are deploy-artifact minimal shape;
- no legacy matrix fields (`deploy_driver`, `artifact_generate`);
- fixture regeneration removes `runtime-base/**` from committed artifacts.

## Plan of Work

1. Regenerate both docker/containerd fixture artifacts with the current CLI.
2. Compare output against committed fixtures and confirm contract-shape consistency.
3. Run E2E runner unit/contract tests that enforce matrix and contract rules.
4. Update plan/master progress and capture GO/NO-GO outcome.

## Concrete Steps

From repository root (`/home/akira/esb`):

- `ESB_CMD='go -C cli run ./cmd/esb' ./e2e/scripts/regenerate_artifacts.sh`
- `rg -n "runtime_meta|runtime-base|deploy_driver|artifact_generate" e2e/artifacts e2e/runner -S`
- `uv run pytest e2e/runner/tests`

## Validation and Acceptance

Milestone 5 is accepted when:

- regenerated fixture artifacts are present and contract-compliant;
- runner contract tests pass;
- no stale legacy contract markers remain in E2E artifacts.

## Idempotence and Recovery

Fixture regeneration is idempotent. Re-run script if partial output occurs.
If regeneration introduces unexpected diff volume, validate only contract-relevant files first and isolate non-contract drift before proceeding.

## Artifacts and Notes

Primary artifacts:

- regenerated fixture manifests and template outputs under `e2e/artifacts/`
- runner contract test outputs

## Interfaces and Dependencies

This milestone does not change public interfaces. It validates contract alignment between:

- producer output (`cli`)
- fixture repository state (`e2e/artifacts`)
- runner contract enforcement (`e2e/runner`)
