# Milestone 1 Plan: Guard Hardening First

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

This milestone prevents reintroduction of CLI-split blockers. After completion, CI must fail immediately when adapter modules (`cli`, `tools/artifactctl`) add forbidden `replace github.com/poruru-code/esb/pkg/*` directives or use internal `pkg/*` with `v0.0.0` placeholder versions.

## Progress

- [x] (2026-02-20 20:55Z) Implemented full adapter `replace` ban in `tools/ci/check_tooling_boundaries.sh` (`pkg/*` wildcard).
- [x] (2026-02-20 20:55Z) Implemented adapter `v0.0.0` detection with frozen allowlist file `tools/ci/adapter_pkg_v0_allowlist.txt`.
- [x] (2026-02-20 20:55Z) Wired `tools/ci/check_repo_layout.sh` into `.github/workflows/quality-gates.yml`.
- [x] (2026-02-20 20:55Z) Added guard fixture file (`adapter_pkg_v0_allowlist.txt`) and validated diff-based failure behavior.
- [x] (2026-02-20 20:56Z) Validated local + CI-equivalent commands and intentional-fail scenarios; captured evidence below.

## Surprises & Discoveries

- Observation: current `replace` guard only targets `artifactcore|composeprovision`.
  Evidence: regex in `tools/ci/check_tooling_boundaries.sh` is scoped to two names.

- Observation: strict `v0.0.0` fail cannot be enabled immediately because current baseline intentionally still contains adapter `v0.0.0` until later milestones.
  Evidence: `cli/go.mod` and `tools/artifactctl/go.mod` both contain internal `pkg/* v0.0.0` entries on current `develop`.

- Observation: guard robustness needed explicit “fail-on-diff” semantics, not binary presence check.
  Evidence: allowlist diff output clearly showed added fake dependency (`tools/artifactctl/go.mod github.com/poruru-code/esb/pkg/fake`) during intentional-fail validation.

## Decision Log

- Decision: Guard conditions will target adapters only, not `pkg/*` modules in this milestone.
  Rationale: this milestone is about ingress prevention at adapter boundary; package cleanup belongs to Milestone 2/3.
  Date/Author: 2026-02-20 / Codex

- Decision: Use a temporary frozen allowlist (`tools/ci/adapter_pkg_v0_allowlist.txt`) for adapter `v0.0.0` entries instead of immediate hard-ban.
  Rationale: enables Milestone 1 guard hardening without breaking current baseline; Milestone 3 will remove entries and collapse this transitional mechanism.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 1 completed. Adapter boundary guards are now significantly stronger and immediately detect two high-risk regressions: adapter-side `replace` of internal `pkg/*` modules and unreviewed drift in adapter `v0.0.0` usage. CI now runs both boundary and layout checks in the quality gate flow. No runtime behavior changed.

## Context and Orientation

The boundary guard script is `tools/ci/check_tooling_boundaries.sh`. It already enforces API allowlist and helper naming guard for `pkg/artifactcore`, and enforces dependency direction for `services/*`.

The quality-gates workflow currently runs `check_tooling_boundaries.sh` but not `check_repo_layout.sh`.

## Plan of Work

Update adapter dependency contract checks in `tools/ci/check_tooling_boundaries.sh`.

1. Expand forbidden `replace` regex to any `pkg/*` path under `github.com/poruru-code/esb/pkg/` (and legacy path alias for safety).
2. Add check for adapter `require github.com/.../pkg/* v0.0.0` and fail with explicit guidance.
3. Keep existing artifactcore guards unchanged.
4. Add a dedicated workflow step in `.github/workflows/quality-gates.yml` to run `./tools/ci/check_repo_layout.sh` in `go-boundary-guard` job.

## Concrete Steps

Run from `/home/akira/esb`.

    ./tools/ci/check_tooling_boundaries.sh
    ./tools/ci/check_repo_layout.sh
    go -C tools/artifactctl test ./... -run '^$'
    if [ -f cli/go.mod ]; then GOWORK="$(pwd)/go.work.cli" go -C cli test ./... -run '^$'; fi

## Validation and Acceptance

Acceptance conditions:

- Adding `replace github.com/poruru-code/esb/pkg/runtimeimage => ...` in `cli/go.mod` must fail boundary guard.
- Keeping/removing non-forbidden lines must still pass guard.
- Adding `require github.com/poruru-code/esb/pkg/runtimeimage v0.0.0` in adapter module must fail guard.
- Quality gate job includes both `check_tooling_boundaries.sh` and `check_repo_layout.sh`.

## Idempotence and Recovery

Script edits are deterministic and repeatable. If regex over-matches, rollback only script/workflow hunks and rerun both checks.

## Artifacts and Notes

Capture command output snippets showing both pass and intentional-fail behaviors.

    PASS: ./tools/ci/check_tooling_boundaries.sh
    PASS: ./tools/ci/check_repo_layout.sh
    PASS: go -C tools/artifactctl test ./... -run '^$'
    PASS: GOWORK="$(pwd)/go.work.cli" go -C cli test ./... -run '^$'

    INTENTIONAL FAIL (replace):
    cli/go.mod:86:replace github.com/poruru-code/esb/pkg/fake => ../pkg/fake
    [error] do not add pkg/* replace directives to adapter go.mod files

    INTENTIONAL FAIL (v0 drift):
    +tools/artifactctl/go.mod github.com/poruru-code/esb/pkg/fake
    [error] adapter pkg/* v0.0.0 set changed; update allowlist with separation rationale

## Interfaces and Dependencies

No runtime interface changes. This milestone modifies only static guard behavior and CI wiring.

## Revision Notes

- 2026-02-20: Initial detailed milestone plan created from master plan.
