# Milestone 5 Plan: Extraction-Readiness Sign-off

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

This milestone finalizes separation readiness and documents the operational handoff. After completion, extraction can be executed as a controlled move with explicit checklist and verification steps.

## Progress

- [ ] Update docs to reflect post-split ownership and workflows.
- [ ] Add a precise extraction checklist (pre-checks, move, post-checks).
- [ ] Run final verification set (quality gates + full E2E) from clean state.
- [ ] Publish readiness statement in master plan retrospective.

## Surprises & Discoveries

To be filled during implementation.

## Decision Log

- Decision: Treat this milestone as sign-off gate, not code-heavy refactor.
  Rationale: by this point technical blockers should be resolved; remaining risk is operational ambiguity.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

To be filled after implementation.

## Context and Orientation

By Milestone 5, dependency and CI contracts should already be hardened. The remaining task is to ensure maintainers can perform extraction without relying on hidden assumptions.

## Plan of Work

1. Update separation-relevant docs under `docs/` and `.agent/` if needed.
2. Add extraction-ready checklist with exact commands and expected outputs.
3. Execute final clean verification and capture evidence.
4. Close master plan with explicit complete/incomplete list.

## Concrete Steps

Run from `/home/akira/esb`.

    ./tools/ci/check_tooling_boundaries.sh
    ./tools/ci/check_repo_layout.sh
    GOWORK=off go -C tools/artifactctl test ./...
    if [ -f cli/go.mod ]; then GOWORK=off go -C cli test ./...; fi
    ARTIFACTCTL_BIN=/tmp/artifactctl-local uv run e2e/run_tests.py --verbose --cleanup

## Validation and Acceptance

Acceptance conditions:

- Final checklist exists and is executable by a new contributor.
- Boundary/layout guards and tests are green.
- Full E2E matrix is green from clean environment.
- Master plan retrospective records completion and residual risks (if any).

## Idempotence and Recovery

All validation commands are rerunnable. If clean-state verification fails due to environment drift, reset Docker resources and rerun checklist.

## Artifacts and Notes

Capture final command transcripts and links to merged PRs for Milestones 1–5.

## Interfaces and Dependencies

No new runtime interfaces; this is release-readiness closure.

## Revision Notes

- 2026-02-20: Initial detailed milestone plan created from master plan.
