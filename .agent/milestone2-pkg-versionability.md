# Milestone 2 Plan: Package Module Versionability Foundation

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

This milestone makes `pkg/*` modules consumable outside workspace-only development. After completion, package modules no longer depend on `v0.0.0` placeholders or mandatory local `replace` assumptions for normal resolution.

## Progress

- [x] (2026-02-20 21:19Z) Inventory all `pkg/*` module dependencies and local `replace` rules.
- [x] (2026-02-20 21:25Z) Define versioning approach (pseudo-version from `develop`) for internal `pkg/*` references.
- [x] (2026-02-20 21:27Z) Update `pkg/artifactcore/go.mod` and `pkg/deployops/go.mod` first.
- [x] (2026-02-20 21:27Z) Apply same cleanup to any remaining `pkg/*` modules if needed (`composeprovision`, `runtimeimage`, `yamlshape` had no placeholder deps).
- [x] (2026-02-20 21:28Z) Validate all `pkg/*` with `GOWORK=off` test runs.

## Surprises & Discoveries

- Observation: `pkg/artifactcore` and `pkg/deployops` currently contain local `replace` and `v0.0.0` references.
  Evidence: `pkg/artifactcore/go.mod`, `pkg/deployops/go.mod`.

- Observation: `@latest` does not resolve internal `pkg/*` modules in current repository/tag state, but `@develop` resolves to valid pseudo-versions.
  Evidence: `go list -m github.com/poruru-code/esb/pkg/*@latest` failed; `@develop` returned `v0.0.0-20260220113651-d6d9b1efaded`.

- Observation: Removing package-module-local `replace` causes immediate auth failures in this environment when resolving private GitHub module URLs.
  Evidence: `go mod tidy` failed with `fatal: could not read Username for 'https://github.com'`.

## Decision Log

- Decision: Keep each `pkg/*` as independent module; do not collapse to single package module.
  Rationale: existing boundary and reuse model already assumes module-level isolation.
  Date/Author: 2026-02-20 / Codex

- Decision: Replace plain `v0.0.0` placeholders with explicit pseudo-versions (`...-d6d9b1efaded`) in package-module requirements, while retaining package-local `replace` for local/offline test execution.
  Rationale: pseudo-version pinning gives external consumers concrete versions; `replace` in dependency modules is not transitive and only affects when that module is the main module.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone completed with concrete pseudo-version pinning in `pkg/artifactcore` and `pkg/deployops`; plain placeholder `v0.0.0` requirements were removed from package modules. Package-local `replace` was intentionally kept because this repository is private and local `GOWORK=off` test execution otherwise fails in non-authenticated environments. All package modules pass with `GOWORK=off`.

## Context and Orientation

`pkg/*` contains shared logic consumed by `cli` and `tools/artifactctl`. Separation requires that these modules can be resolved via real versions from outside this repository.

## Plan of Work

1. Remove placeholder versions in package modules.
2. Minimize local replace usage to developer-only workspace; not required for dependency contract.
3. Re-run tests per module with `GOWORK=off` to prove isolation.

## Concrete Steps

Run from `/home/akira/esb`.

    for m in artifactcore composeprovision deployops runtimeimage yamlshape; do
      GOWORK=off go -C pkg/$m test ./...
    done

    ./tools/ci/check_tooling_boundaries.sh

## Validation and Acceptance

Acceptance conditions:

- No plain placeholder `v0.0.0` remains in `pkg/*/go.mod`.
- Required dependencies are resolvable without workspace injection.
- All package module tests pass with `GOWORK=off`.

## Idempotence and Recovery

Apply dependency changes module-by-module. If one module fails resolution, revert that module’s `go.mod/go.sum` and retry.

## Artifacts and Notes

Record `go.mod` before/after snippets for each touched package module and test transcripts.

## Interfaces and Dependencies

No public behavior changes expected; this is dependency-contract hardening only.

## Revision Notes

- 2026-02-20: Initial detailed milestone plan created from master plan.
- 2026-02-20: Completed milestone implementation with pseudo-version pinning and `GOWORK=off` verification.
