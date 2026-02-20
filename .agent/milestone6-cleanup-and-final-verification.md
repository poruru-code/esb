# Milestone 6 Detail Plan: Cleanup and Final Verification

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md` and the parent plan `.agent/execplan-artifact-contract-realignment-master.md`.

## Purpose / Big Picture

Milestone 6 closes the contract realignment work by removing confirmed obsolete residue and proving the repository state with full unit and E2E verification from a clean Docker state.

After this milestone, remaining risk should be operational only (environment/runtime availability), not contract-boundary ambiguity.

## Progress

- [x] (2026-02-20 03:28Z) Created Milestone 6 detail plan.
- [x] (2026-02-20 03:29Z) Ran final stale-marker/dead-code audit across `cli`, `pkg/artifactcore`, `tools/artifactctl`, `e2e`, and docs.
- [x] (2026-02-20 03:30Z) Reviewed audit findings and confirmed no unaddressed contract drift residue (remaining matches are legacy-field rejection paths and docs).
- [x] (2026-02-20 03:31Z) Executed full UT suite relevant to contract boundary change.
- [x] (2026-02-20 03:31Z) Executed full E2E suite from clean Docker state (`ALL MATRIX ENTRIES PASSED`).
- [x] (2026-02-20 03:32Z) Recorded final GO verdict and residual operational risks.

## Surprises & Discoveries

- Observation: Running parallel E2E processes on the same host can invalidate results by removing shared containers/networks.
  Evidence: prior run context had external interference; final run was executed after explicit cleanup and process isolation.

## Decision Log

- Decision: build and pin `ARTIFACTCTL_BIN` from current workspace for final E2E run.
  Rationale: avoids accidental execution of stale global artifactctl binaries.
  Date/Author: 2026-02-20 / Codex
- Decision: keep references to `deploy_driver` / `artifact_generate` only as explicit legacy-field rejection contract, not active behavior.
  Rationale: preserves strict contract boundary while preventing silent fallback to old matrix semantics.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Final verdict: GO.

Evidence summary:
- Audit (`rg -n "runtime_meta|runtime_hooks|deploy_driver|artifact_generate"`) found only expected legacy-field rejection references in `e2e/runner/config.py`, tests, and docs.
- UT passed:
  - `GOWORK=off go -C pkg/artifactcore test ./...`
  - `go test ./cli/...`
  - `go test ./tools/artifactctl/...`
  - `go -C services/agent test ./...`
  - `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner/tests -q` (`68 passed`)
- Full E2E passed from clean Docker state:
  - `ARTIFACTCTL_BIN=/home/akira/esb/.tmp/bin/artifactctl uv run e2e/run_tests.py --parallel --verbose`
  - `e2e-containerd`: `45 passed`
  - `e2e-docker`: `53 passed`
  - Suite summary: `ALL MATRIX ENTRIES PASSED`

Residual risks (non-contract):
- Host-level contention remains possible if another user/process runs destructive Docker operations concurrently.

## Context and Orientation

Final verification scope:

- `cli` producer path and deploy apply adapters
- `pkg/artifactcore` manifest/apply/runtime compatibility core
- `tools/artifactctl` deploy orchestration adapter
- `e2e` fixture and runner contract paths
- contract/operations docs under `docs/`

## Plan of Work

1. Audit for stale contract markers and obsolete options/fields.
2. Apply any required cleanup edits.
3. Run full UT checks (Go/Python contract-relevant suites).
4. Reset Docker runtime state and run full E2E (`e2e/run_tests.py --parallel --verbose`).
5. Update master/milestone docs with final verdict and risk notes.

## Concrete Steps

From repository root (`/home/akira/esb`):

- audit search:
  - `rg -n "runtime_meta|runtime_hooks|deploy_driver|artifact_generate" cli pkg tools e2e docs -S`
- full UT:
  - `GOWORK=off go -C pkg/artifactcore test ./...`
  - `go test ./cli/...`
  - `go test ./tools/artifactctl/...`
  - `go -C services/agent test ./...`
  - `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner/tests -q`
- clean Docker state + full E2E:
  - `docker compose -f docker-compose.infra.yml down --remove-orphans`
  - `docker compose -f e2e/environments/e2e-docker/docker-compose.yml down --remove-orphans || true`
  - `docker compose -f e2e/environments/e2e-containerd/docker-compose.yml down --remove-orphans || true`
  - `go -C tools/artifactctl build -o .tmp/bin/artifactctl ./cmd/artifactctl`
  - `ARTIFACTCTL_BIN=/home/akira/esb/.tmp/bin/artifactctl uv run e2e/run_tests.py --parallel --verbose`

## Validation and Acceptance

Milestone 6 is accepted when:

- obsolete residue audit produces no unaddressed contract drift findings;
- full UT suite passes for changed responsibility boundaries;
- full E2E passes from clean Docker state;
- master/milestone plans are updated with final status and residual risk.

## Idempotence and Recovery

Commands are rerunnable. If E2E fails due external environment contention, re-run from the Docker clean-state step with no parallel E2E process on the same host.

## Artifacts and Notes

Expected evidence:

- grep audit outputs
- UT command outputs
- full E2E command output and final summary

## Interfaces and Dependencies

This milestone does not introduce new interfaces. It verifies boundary integrity and behavior of existing interfaces after refactor milestones.
