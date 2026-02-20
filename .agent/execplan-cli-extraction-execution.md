# CLI Extraction Execution Plan (esb -> esb-cli)

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

Execute the physical CLI split from `github.com/poruru-code/esb` to `github.com/poruru-code/esb-cli` using the already-completed readiness contract (Milestones 1-5 in previous plan). After completion:

- `esb` repo no longer contains `cli/`.
- `esb` repo CI and E2E are green without CLI source tree.
- `esb-cli` repo is initialized from existing `cli/` history and builds standalone.

## Progress

- [x] (2026-02-20) Milestone 1: Seed `esb-cli` with `cli/` history from this repo.
- [x] (2026-02-20) Milestone 2: Make `esb-cli` standalone-compilable (module path, imports, tests).
- [x] (2026-02-20) Milestone 3: Remove `cli/` from `esb` and update CI/docs/contracts.
- [x] (2026-02-20) Milestone 4: Final verification and sign-off (`esb` no-CLI checks + full E2E).

## Surprises & Discoveries

- Observation: readiness milestones are already complete in `esb`, including CLI-absent rehearsal CI.
  Evidence: `.agent/execplan-cli-separation-master-20260220.md` marked complete through Milestone 5.
- Observation: `esb-cli` standalone tests initially failed due repo-root assumptions and monorepo fixture path assumptions.
  Evidence: `TestRunNoArgsShowsUsage` / `TestCompletionCommandRemoved` and env contract fixture load failed before fallback/path fixes.
- Observation: full E2E passed in no-CLI state with local `artifactctl` binary, but emitted non-blocking `runtime_stack.esb_version` warning.
  Evidence: `uv run e2e/run_tests.py --parallel --verbose --cleanup` reported all matrix entries passed, with warning log only.

## Decision Log

- Decision: preserve `cli/` commit history in the initial `esb-cli` seed via subtree split.
  Rationale: keeps blame/history continuity for future maintenance in new repo.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Extraction execution completed end-to-end.

- `esb-cli` repository has seeded history from `cli/` subtree and a standalone hardening branch with passing `GOWORK=off go test ./...`.
- `esb` repository no longer contains `cli/` nor `go.work.cli*`.
- CI/docs/contracts were updated to no-CLI steady state (external CLI docs links, no local CLI lint/build hooks).
- Validation passed: boundary/layout checks, `tools/artifactctl` + `pkg/*` tests, gateway/runner/agent unit tests, and full E2E matrix.

## Context and Orientation

`esb-cli` repository exists and is empty. `esb` repository currently still includes `cli/` and `go.work.cli` for in-repo development. This execution plan performs the actual cutover.

## Milestones

- Milestone 1 detail: `.agent/milestone1-cli-extraction-seed.md`
- Milestone 2 detail: `.agent/milestone2-cli-standalone-hardening.md`
- Milestone 3 detail: `.agent/milestone3-esb-cli-removal-and-contract-update.md`
- Milestone 4 detail: `.agent/milestone4-final-verification-no-cli.md`

## Plan of Work

### Milestone 1

1. Split `cli/` history from current branch (`git subtree split --prefix=cli`).
2. Push split history into `https://github.com/poruru-code/esb-cli` default branch.
3. Record seed commit mapping in this plan.

### Milestone 2

1. Update module path to `github.com/poruru-code/esb-cli`.
2. Replace internal import path prefixes from old module path.
3. Ensure go test passes in `esb-cli` repository.

### Milestone 3

1. Remove `cli/`, `go.work.cli`, `go.work.cli.sum` from `esb`.
2. Update workflows/scripts/docs referencing in-repo CLI paths.
3. Keep boundary checks strict and compatible with no-CLI steady state.

### Milestone 4

1. Run boundary/layout and module tests in `esb` without CLI tree.
2. Run full E2E with local `artifactctl` binary.
3. Document completion and residual risks.

## Validation and Acceptance

Acceptance requires all of:

- `esb-cli` contains migrated CLI history and passes `go test ./...`.
- `esb` has no `cli/` directory.
- `./tools/ci/check_tooling_boundaries.sh` passes.
- `./tools/ci/check_repo_layout.sh` passes in no-CLI steady state.
- `GOWORK=off` tests for `tools/artifactctl` and `pkg/*` pass.
- `uv run e2e/run_tests.py --parallel --verbose --cleanup` passes.

## Idempotence and Recovery

- Seeding is idempotent at branch level; rerun subtree split from updated HEAD if needed.
- If split introduces regression in `esb`, revert only Milestone 3 commit(s) and re-run validation.

## Revision Notes

- 2026-02-20: Initial execution plan created.
