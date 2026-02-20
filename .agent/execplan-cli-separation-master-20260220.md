# CLI Repo Separation Master Plan (Post-PR183)

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

After this plan is complete, this repository will remain healthy even when `cli/` is absent, and `tools/artifactctl` plus shared packages (`pkg/*`) will build and test without relying on workspace-only tricks such as `v0.0.0` placeholder dependencies or adapter-side `replace` directives. The user-visible outcome is that CLI extraction can proceed as a routine repository split, not as a high-risk migration that breaks CI or developer workflows.

You will be able to demonstrate success by running the quality gates and E2E matrix in this repository with the new boundary guards enabled, and by running dedicated CI checks that simulate a "CLI missing" layout.

## Progress

- [x] (2026-02-20 20:00Z) Synced local `develop` to `origin/develop` after PR #183 merge (`d6d9b1e`) and confirmed clean baseline.
- [x] (2026-02-20 20:00Z) Revalidated separation baseline and identified remaining blockers from current tree (`v0.0.0` dependencies, partial guard coverage, no CLI-missing CI job).
- [x] (2026-02-20 20:56Z) Implemented PR-1: boundary guard hardening (full adapter `replace` ban, adapter `v0.0.0` drift detection via allowlist freeze, layout check wiring in quality gates).
- [x] (2026-02-20 21:28Z) Implemented PR-2: package module versionability foundation (replaced plain placeholder `v0.0.0` in package modules with pseudo-version pins and verified all `pkg/*` tests with `GOWORK=off`).
- [x] (2026-02-20 21:38Z) Implemented PR-3: adapter dependency normalization (`cli` / `artifactctl` removed direct `pkg/* v0.0.0`, updated to pseudo-versions, and validated `GOWORK=off` compile checks).
- [x] (2026-02-20 21:49Z) Implemented PR-4: CLI-absent rehearsal CI job and proof run.
- [x] (2026-02-20 22:18Z) Implemented PR-5: extraction handoff checklist documented and final verification executed (boundary/layout, module tests, full E2E).

## Surprises & Discoveries

- Observation: The current boundary guard is already active in CI but only bans adapter `replace` for two package names.
  Evidence: `tools/ci/check_tooling_boundaries.sh` currently matches only `pkg/(artifactcore|composeprovision)`.

- Observation: Module path normalization is already complete for the new repository name.
  Evidence: `cli/go.mod`, `tools/artifactctl/go.mod`, and all `pkg/*/go.mod` use `github.com/poruru-code/esb/...`.

- Observation: `v0.0.0` remains in adapters and shared packages, which still couples builds to workspace behavior.
  Evidence: `cli/go.mod` and `tools/artifactctl/go.mod` require `github.com/poruru-code/esb/pkg/* v0.0.0`; `pkg/deployops/go.mod` and `pkg/artifactcore/go.mod` also use `v0.0.0` and local `replace`.

- Observation: `go.work` intentionally excludes `cli`, while `go.work.cli` includes it; this is already a good directional split for staged extraction.
  Evidence: `go.work` uses only `services/agent` and `tools/artifactctl`; `go.work.cli` adds `./cli`.

- Observation: immediate hard-fail on adapter `v0.0.0` cannot be enabled before Milestone 3, otherwise current baseline fails by design.
  Evidence: adapter modules still contain `pkg/* v0.0.0` on current `develop`; Milestone 1 implemented a frozen allowlist to block unreviewed drift.

- Observation: `@latest` is not currently a usable query for internal `pkg/*` modules under current tag layout; `@develop` resolves and yields pseudo-versions.
  Evidence: `go list -m ...@latest` failed with no matching versions; `@develop` returned `v0.0.0-20260220113651-d6d9b1efaded`.

- Observation: package-local `replace` removal fails in non-authenticated environments because this repository is private.
  Evidence: removing `replace` from `pkg/artifactcore/go.mod` triggered `go mod tidy` failure (`could not read Username for https://github.com`).

- Observation: adapters cannot complete `v0.0.0` normalization until Milestone 2 package-module go.mod changes are remotely resolvable revisions.
  Evidence: `go mod tidy` in `cli` and `tools/artifactctl` fails with `unknown revision pkg/<module>/v0.0.0`, and temporary pseudo-version adapter edits still fail via transitive `v0.0.0` from remote `pkg/deployops`/`pkg/artifactcore`.

- Observation: once adapters were moved to pseudo-version `v0.0.0-20260220120751-741830b8344a` and `go mod tidy` was executed, both adapter compile checks passed with `GOWORK=off`.
  Evidence: `GOWORK=off go -C tools/artifactctl test ./... -run '^$'` and `GOWORK=off go -C cli test ./... -run '^$'`.

- Observation: CLI-absent rehearsal initially failed because layout check required runtime-template files under `cli/assets`.
  Evidence: local rehearsal (`rm -rf cli`) produced missing path errors in `check_repo_layout.sh`.

- Observation: final E2E matrix passed end-to-end with cleanup, confirming no regression from Milestone 1-4 contract hardening.
  Evidence: `uv run e2e/run_tests.py --parallel --verbose --cleanup` ended with `[PASSED] ALL MATRIX ENTRIES PASSED!`.

## Decision Log

- Decision: Keep the target module namespace as `github.com/poruru-code/esb` and treat old `edge-serverless-box` paths as legacy only.
  Rationale: Repository rename is already merged and active; reverting would add churn with no migration value.
  Date/Author: 2026-02-20 / Codex

- Decision: Treat the old five-PR concept as valid in intent, but update scope and acceptance criteria to current repository reality.
  Rationale: PR #183 changed baseline behavior, and guard/layout infrastructure already exists; planning must start from current facts.
  Date/Author: 2026-02-20 / Codex

- Decision: Prioritize guard completeness before dependency version migration.
  Rationale: Without strict guardrails, `v0.0.0` or adapter-side `replace` regressions can be reintroduced during later PRs.
  Date/Author: 2026-02-20 / Codex

- Decision: In PR-1, enforce adapter `v0.0.0` as a frozen set (diff-based guard) instead of immediate zero-tolerance fail.
  Rationale: keeps `develop` green while preventing new debt; strict removal is executed in PR-3.
  Date/Author: 2026-02-20 / Codex

- Decision: In PR-2, package modules adopt explicit pseudo-version pins while retaining package-local `replace`.
  Rationale: removes placeholder ambiguity and gives external consumers explicit versions; local `replace` remains for authenticated-independent local module testing because this repo is private.
  Date/Author: 2026-02-20 / Codex

- Decision: treat Milestone 3 as a dependent phase that starts only after Milestone 2 revisions are reachable as module revisions from adapter modules.
  Rationale: otherwise adapter normalization cannot be validated under `GOWORK=off` and will fail deterministically.
  Date/Author: 2026-02-20 / Codex

- Decision: maintain adapter `v0.0.0` guard via empty allowlist baseline after Milestone 3.
  Rationale: keeps CI logic stable while enforcing the new zero-placeholder target.
  Date/Author: 2026-02-20 / Codex

- Decision: add explicit `CLI_ABSENT_MODE=1` switch for layout check and use it only in dedicated CLI-absent rehearsal job.
  Rationale: preserves strict default layout checks while enabling split-readiness rehearsal.
  Date/Author: 2026-02-20 / Codex

- Decision: keep extraction operation as separate follow-up execution step; this plan closes with readiness and verified checklist rather than moving directories in-place.
  Rationale: decouples operational migration timing from contract hardening and CI proof completion.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 1-5 complete. The repository now blocks adapter `pkg/*` replace directives comprehensively, enforces a zero-placeholder adapter `v0.0.0` baseline, uses pseudo-version pins in shared package modules and adapters, continuously rehearses CLI-absent operation in CI, and provides an executable extraction checklist with successful final verification (including full E2E matrix).

## Context and Orientation

This repository currently hosts three categories of Go code that matter for CLI separation.

`cli/` contains the end-user command application and currently imports shared packages under `pkg/*`. `tools/artifactctl/` contains the artifact apply/provision command-line adapter and also imports shared packages. `pkg/*` contains split-out shared modules (`artifactcore`, `composeprovision`, `deployops`, `runtimeimage`, `yamlshape`) that are meant to become reusable across repositories.

The most important control points are `tools/ci/check_tooling_boundaries.sh` for dependency direction and API-surface rules, `.github/workflows/quality-gates.yml` for CI enforcement, and `go.work` plus `go.work.cli` for workspace resolution behavior. The current risk is not runtime correctness; it is extraction fragility caused by unresolved module version contracts.

In plain language, `v0.0.0` in `go.mod` means "this dependency is not pinned to a real published version." That is acceptable only while local workspace replacement is guaranteed. Repository split removes that guarantee.

## Milestone Detail Plans

- Milestone 1 detail: `.agent/milestone1-guard-hardening.md`
- Milestone 2 detail: `.agent/milestone2-pkg-versionability.md`
- Milestone 3 detail: `.agent/milestone3-adapter-version-normalization.md`
- Milestone 4 detail: `.agent/milestone4-cli-absent-ci-rehearsal.md`
- Milestone 5 detail: `.agent/milestone5-extraction-readiness-signoff.md`

## Plan of Work

### Milestone 1 (PR-1): Guard hardening first

This milestone prevents new separation debt. Update `tools/ci/check_tooling_boundaries.sh` so it rejects any `replace github.com/poruru-code/esb/pkg/*` in adapter modules (`cli/go.mod`, `tools/artifactctl/go.mod`) and rejects adapter `require github.com/poruru-code/esb/pkg/* v0.0.0`. Keep existing artifactcore API guard and naming guard intact.

Then wire `tools/ci/check_repo_layout.sh` into `.github/workflows/quality-gates.yml` so layout boundaries and dependency boundaries are both enforced in the same gate.

Acceptance for this milestone is behavioral: intentionally adding a forbidden adapter `replace` or adapter `v0.0.0` must fail CI with a clear message.

### Milestone 2 (PR-2): Shared package versionability foundation

This milestone makes `pkg/*` modules consumable without hidden local assumptions. Remove local-only dependency shortcuts inside `pkg/*/go.mod` and replace `v0.0.0` usage with resolvable versions (tag or pseudo-version strategy chosen and documented in the PR). Keep dependency graph minimal and explicit.

Because this repository is still in active development, this milestone must also define how maintainers refresh package versions safely when `pkg/*` changes. The rule must be explicit and automatable, not tribal knowledge.

Acceptance is that each package module passes `GOWORK=off go -C <pkg-module> test ./...` and no package module depends on `v0.0.0` placeholders.

### Milestone 3 (PR-3): Adapter dependency normalization

This milestone updates `cli/go.mod` and `tools/artifactctl/go.mod` to consume real package versions and removes adapter-side `v0.0.0` entirely. The adapters must compile and run tests with `GOWORK=off` in CI so split readiness is continuously verified.

At the end of this milestone, `go.work.cli` may still exist for local convenience, but it must no longer be required for dependency resolution correctness.

Acceptance is twofold: adapter modules have no `v0.0.0` entries for internal `pkg/*`, and CI proves compile/test with `GOWORK=off`.

### Milestone 4 (PR-4): CLI-absent rehearsal CI

This milestone proves the repository can function without `cli/`. Add a dedicated CI job that checks out or simulates a tree without `cli/` and then runs repository checks that should still hold (`check_tooling_boundaries.sh`, `check_repo_layout.sh`, `tools/artifactctl` tests, and relevant package tests).

Acceptance is a green CI job whose explicit purpose is "CLI missing rehearsal".

### Milestone 5 (PR-5): Extraction-ready sign-off

This final milestone does not copy `cli/` yet; it locks the handoff contract. Update docs that describe ownership boundaries and developer workflows so that extraction can be executed in a separate operational step with minimal ambiguity.

Acceptance is an extraction checklist in docs that is executable by a new contributor and validated by rerunning quality gates and full E2E from a clean environment.

## Concrete Steps

The following commands are the baseline commands to run in each PR. Run from repository root `/home/akira/esb` unless a different directory is explicitly stated.

1. Baseline checks before edits.

    git status -sb
    ./tools/ci/check_tooling_boundaries.sh
    ./tools/ci/check_repo_layout.sh

2. Adapter and package compile checks.

    GOWORK=off go -C tools/artifactctl test ./... -run '^$'
    if [ -f cli/go.mod ]; then GOWORK=off go -C cli test ./... -run '^$'; fi

3. Shared package checks.

    GOWORK=off go -C pkg/artifactcore test ./...
    GOWORK=off go -C pkg/composeprovision test ./...
    GOWORK=off go -C pkg/deployops test ./...
    GOWORK=off go -C pkg/runtimeimage test ./...
    GOWORK=off go -C pkg/yamlshape test ./...

4. End-to-end proof before merging high-risk milestones.

    ARTIFACTCTL_BIN=/tmp/artifactctl-local uv run e2e/run_tests.py --verbose --cleanup

Expected result for step 4 is that both matrix entries pass and the suite summary prints `ALL MATRIX ENTRIES PASSED!`.

## Validation and Acceptance

Each milestone must satisfy both local and CI acceptance. Local acceptance is command-based and observable, not inferred from code inspection. CI acceptance requires relevant quality-gates jobs to pass.

Overall plan acceptance requires all of the following at the same time.

- Adapter modules (`cli`, `tools/artifactctl`) do not use `v0.0.0` for internal `pkg/*` dependencies.
- Adapter modules do not rely on adapter-local `replace github.com/poruru-code/esb/pkg/*` directives.
- Shared package modules resolve and test with `GOWORK=off`.
- A dedicated CLI-absent CI rehearsal job is green.
- Full E2E matrix remains green.

## Idempotence and Recovery

All plan steps are designed to be rerun safely. Guard scripts are read-only checks, and test commands can be rerun without mutating tracked source. If a milestone branch becomes inconsistent, recovery is to reset to latest `develop`, recreate the milestone branch, and replay only the milestone edits.

For dependency version edits, perform one module at a time and run `go test` immediately after each change. If resolution fails, revert only the last edited `go.mod` and `go.sum` pair, then retry with a corrected version reference.

## Artifacts and Notes

Current baseline evidence captured at plan start:

    HEAD: d6d9b1e (develop)
    PR #183 merged
    check_tooling_boundaries.sh: PASS
    check_repo_layout.sh: PASS

Current known blockers captured at plan start:

    cli/go.mod requires github.com/poruru-code/esb/pkg/* v0.0.0
    tools/artifactctl/go.mod requires github.com/poruru-code/esb/pkg/* v0.0.0
    pkg/artifactcore and pkg/deployops still use local replace/v0.0.0 in go.mod

## Interfaces and Dependencies

The stable dependency direction for this plan is:

- `pkg/*` modules are shared logic and must not depend on `cli` or `tools`.
- `tools/artifactctl` is an adapter that depends on `pkg/*`.
- `cli` is another adapter that depends on `pkg/*` while it still exists in this repository.
- `services/*` must not depend on `tools/*` or adapter-only logic.

The primary interfaces that must remain stable during separation are `deployops.Execute` for apply orchestration, `artifactcore` manifest/apply contracts, and `composeprovision` provisioning entrypoints. Guard changes in Milestone 1 must not alter runtime behavior of these interfaces; they only constrain dependency shape.

## Revision Notes

- 2026-02-20: Initial master plan created after PR #183 merge. Replaced old pre-merge assumptions with current baseline facts and redefined PR-1..PR-5 acceptance criteria for the renamed repository (`github.com/poruru-code/esb`).
- 2026-02-20: Added explicit links to per-milestone detailed plans so implementation can proceed with a strict \"plan-before-execute\" workflow.
- 2026-02-20: Updated after Milestone 1 implementation and validation; recorded transitional `v0.0.0` freeze guard decision and completion status.
- 2026-02-20: Updated after Milestone 2 implementation; package modules moved from placeholder `v0.0.0` to pseudo-version pins.
- 2026-02-20: Updated after Milestone 3 implementation; adapters moved to versioned `pkg/*` dependencies and `GOWORK=off` compile checks validated.
- 2026-02-20: Updated after Milestone 4 implementation; added CLI-absent rehearsal CI and layout-check switch.
- 2026-02-20: Updated after Milestone 5 implementation; extraction checklist added and final verification completed.
