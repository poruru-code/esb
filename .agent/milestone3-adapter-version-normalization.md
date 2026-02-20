# Milestone 3 Plan: Adapter Version Normalization

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

This milestone removes workspace-only dependency assumptions from `cli` and `tools/artifactctl`. After completion, adapters use real internal package versions and compile with `GOWORK=off`.

## Progress

- [ ] Replace adapter `pkg/* v0.0.0` requirements with resolvable versions. (blocked until Milestone 2 pseudo-versioned `pkg/*` deps are remotely available)
- [ ] Ensure no adapter-local `replace github.com/poruru-code/esb/pkg/*` is required.
- [ ] Update any supporting docs that mention workspace-only assumptions.
- [ ] Validate adapter compile/test with `GOWORK=off`.

## Surprises & Discoveries

- Observation: both adapters currently depend on `pkg/* v0.0.0`.
  Evidence: `cli/go.mod`, `tools/artifactctl/go.mod`.

- Observation: adapter `go mod tidy` with `v0.0.0` fails deterministically (`unknown revision pkg/<module>/v0.0.0`) under `GOWORK=off`.
  Evidence: `go -C tools/artifactctl mod tidy` and `go -C cli mod tidy` failures.

- Observation: even when adapter requirements are temporarily switched to pseudo-version (`...-d6d9b1efaded`), resolution still fails because remote `pkg/deployops` / `pkg/artifactcore` at that revision still require transitive `v0.0.0`.
  Evidence: temporary edit to `tools/artifactctl/go.mod` followed by `go mod tidy` failed fetching transitive `pkg/*@v0.0.0`.

## Decision Log

- Decision: `go.work.cli` remains optional convenience only, not a correctness dependency.
  Rationale: extraction readiness requires adapter correctness without workspace coupling.
  Date/Author: 2026-02-20 / Codex

- Decision: complete Milestone 2 changes must be published (or otherwise made resolvable as versioned module revisions) before Milestone 3 can be validated with `GOWORK=off`.
  Rationale: adapters consume `pkg/deployops`/`pkg/artifactcore` transitively; unresolved transitive `v0.0.0` blocks adapter normalization.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

To be filled after implementation.

## Context and Orientation

Adapters are `cli/` and `tools/artifactctl/`. They must build against shared packages as external dependencies, not by implicit local wiring.

## Plan of Work

1. Update `cli/go.mod` dependency versions.
2. Update `tools/artifactctl/go.mod` dependency versions.
3. Validate with strict `GOWORK=off` compile/test commands.
4. Ensure boundary script passes under tightened rules from Milestone 1.

## Concrete Steps

Run from `/home/akira/esb`.

    GOWORK=off go -C tools/artifactctl test ./... -run '^$'
    if [ -f cli/go.mod ]; then GOWORK=off go -C cli test ./... -run '^$'; fi
    ./tools/ci/check_tooling_boundaries.sh

## Validation and Acceptance

Acceptance conditions:

- `cli/go.mod` and `tools/artifactctl/go.mod` contain no internal `pkg/* v0.0.0`.
- Adapter compile passes with `GOWORK=off`.
- Boundary guard remains green.

## Idempotence and Recovery

If adapter compile fails after version bump, revert only that adapter’s `go.mod/go.sum` and retry with corrected version selection.

## Artifacts and Notes

Capture adapter compile command outputs and final dependency diff snippets.

## Interfaces and Dependencies

No behavioral API changes expected in this milestone.

## Revision Notes

- 2026-02-20: Initial detailed milestone plan created from master plan.
- 2026-02-20: Added execution blocker and prerequisite after preflight validation.
