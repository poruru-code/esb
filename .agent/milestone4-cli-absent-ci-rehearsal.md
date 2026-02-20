# Milestone 4 Plan: CLI-Absent Rehearsal CI

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

This milestone proves the repository stays healthy when `cli/` is absent. After completion, CI has a dedicated job that simulates "no CLI" and still passes required checks for remaining components.

## Progress

- [x] (2026-02-20 21:47Z) Add CI job for CLI-absent mode (`cli-absent-rehearsal`).
- [x] (2026-02-20 21:47Z) Simulate missing `cli/` safely in workflow (`rm -rf cli` in dedicated job only).
- [x] (2026-02-20 21:48Z) Run boundary/layout checks and artifactctl/package tests in that mode.
- [x] (2026-02-20 21:49Z) Confirm existing guard/compile checks remain unaffected.

## Surprises & Discoveries

- Observation: existing `check_repo_layout.sh` fails in CLI-absent tree due hard requirement on `cli/assets/runtime-templates/...`.
  Evidence: local rehearsal without `cli/` reported missing runtime template paths.

- Observation: introducing `CLI_ABSENT_MODE=1` to layout check cleanly isolates split-rehearsal behavior without weakening default checks.
  Evidence: normal layout check still passes; CLI-absent layout check passes only when flag is explicitly set.

## Decision Log

- Decision: Validate CLI-absent mode in CI before physical split.
  Rationale: this catches latent hard dependencies early and repeatedly.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone completed. Quality gates now include a dedicated `cli-absent-rehearsal` job that removes `cli/`, runs boundary/layout checks, and validates non-CLI compile contracts. Local rehearsal run confirmed this path is green.

## Context and Orientation

Current CI (`quality-gates.yml`) assumes normal checkout with `cli/` present. This milestone adds an explicit rehearsal mode, not replacing existing jobs.

## Plan of Work

1. Add new workflow job (or extend existing) named clearly for CLI-absent rehearsal.
2. In that job, run the minimal authoritative checks for non-CLI repo core.
3. Ensure job output is easy to interpret as split-readiness signal.

## Concrete Steps

Example local simulation commands from repo root:

    tmpdir=$(mktemp -d)
    rsync -a --exclude '.git' . "$tmpdir/repo"
    rm -rf "$tmpdir/repo/cli"
    (cd "$tmpdir/repo" && ./tools/ci/check_tooling_boundaries.sh)
    (cd "$tmpdir/repo" && ./tools/ci/check_repo_layout.sh)
    (cd "$tmpdir/repo" && GOWORK=off go -C tools/artifactctl test ./... -run '^$')

## Validation and Acceptance

Acceptance conditions:

- Dedicated CI job is present and green.
- Job succeeds when `cli/` is excluded.
- No existing required quality gate regresses.

## Idempotence and Recovery

Workflow-only changes are safe to rerun. If CI logic is flaky, reduce rehearsal scope to deterministic checks and re-expand after stabilization.

## Artifacts and Notes

Record sample CI run URL and key job log snippets.

## Interfaces and Dependencies

No runtime interface changes; CI contract only.

## Revision Notes

- 2026-02-20: Initial detailed milestone plan created from master plan.
- 2026-02-20: Completed milestone with dedicated CI job and `CLI_ABSENT_MODE` layout-check switch.
