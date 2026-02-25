# Replan: Generated `<brand>-ctl` Migration with Callsite Commonization First

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

After this replan, command naming will move to generation-time branding: the executable name will be `<BRAND_SLUG>-ctl`, produced by `esb-branding-tool`. Runtime execution paths will remain non-dependent on runtime `BRAND_SLUG` environment variables.

The first phase is an explicit refactor-only phase that commonizes all command callsites behind shared helpers before any rename behavior change. This reduces churn and prevents broad string-level replacements across E2E, scripts, and docs.

## Progress

- [x] (2026-02-25 15:48Z) Initial two-phase plan (`artifactctl` -> `branding-ctl`) was drafted.
- [x] (2026-02-25 17:05Z) Phase 1 branding-surface minimization work was merged and revalidated in targeted tests.
- [x] (2026-02-25 17:20Z) Real deploy-phase dry-run evidence captured with `uv run e2e/run_tests.py --build-only --profile e2e-docker --no-live --no-color --no-emoji --no-cache` (compose/deploy passed, test phase skipped by design).
- [x] (2026-02-25 18:05Z) User decision recorded: discard old downstream plan that targeted fixed `branding-ctl` naming.
- [x] (2026-02-25 18:55Z) Completed Phase A: added shared E2E ctl contract helper, rewired `run_tests.py` + `deploy.py`, centralized CLI self-name hints in Go main, and variableized shell diagnostics command token.
- [x] (2026-02-25 18:58Z) Validated Phase A with `go test ./tools/artifactctl/...`, `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest -q e2e/runner/tests/test_run_tests_cli_requirement.py e2e/runner/tests/test_deploy_command.py`, and targeted `uv run ruff check`.
- [x] (2026-02-26 11:20Z) Completed Phase B: `esb-branding-tool` now generates ctl command naming, including `.mise.toml` template output (`<BRAND_SLUG>-ctl`) and related tests/docs.
- [x] (2026-02-26 11:35Z) Completed Phase C core migration: `esb3` switched runtime/test/docs/tooling surfaces to generated `<brand>-ctl` defaults (`CTL_BIN` family, `build-ctl`, `esb-ctl` command text), and merged.
- [x] (2026-02-26 12:05Z) Final doc sweep completed for service-local docs (`services/agent/docs`): command text migrated to `esb-ctl`, with `tools/artifactctl` module-path references intentionally retained.
- [ ] Validate end-to-end behavior and publish migration note for ESB-CLI and operators.

## Surprises & Discoveries

- Observation: command-name literals are spread across E2E launcher, runner deploy flow, shell diagnostics, CI checks, and docs.
  Evidence: `rg -n "artifactctl|ARTIFACTCTL" e2e tools docs .mise.toml README.md` returns broad multi-surface matches.

- Observation: this repository has multiple Go modules, so package moves/renames are higher risk than in a single-module repo.
  Evidence: previous attempt to add cross-module shared package required rollback to generated file placement under existing module boundaries.

- Observation: command-name change and behavior refactor mixed together previously increased review risk.
  Evidence: historical review comments clustered around naming churn rather than runtime behavior.

- Observation: focused pytest execution in this repository still imports `e2e/conftest.py` and requires auth env variables even for unit-like runner tests.
  Evidence: `uv run pytest -q e2e/runner/tests/...` failed until `X_API_KEY`, `AUTH_USER`, and `AUTH_PASS` were provided.

## Decision Log

- Decision: The previous downstream plan (fixed `branding-ctl` rename phase) is canceled.
  Rationale: user requested final direction change to generated `<brand>-ctl`.
  Date/Author: 2026-02-25 / Codex

- Decision: Introduce a dedicated refactor-only phase first, with zero command-name behavior change.
  Rationale: commonized callsites allow deterministic migration and small reviewable diffs.
  Date/Author: 2026-02-25 / Codex

- Decision: Final CLI name contract is generation-time `<BRAND_SLUG>-ctl`, not runtime-derived and not fixed global `branding-ctl`.
  Rationale: satisfies runtime non-dependence while allowing branded command identity.
  Date/Author: 2026-02-25 / Codex

- Decision: Backward compatibility aliases are out of scope unless explicitly reintroduced later.
  Rationale: user stated compatibility is not required.
  Date/Author: 2026-02-25 / Codex

- Decision: Keep Phase A runtime env variable names (`ARTIFACTCTL_BIN`, `ARTIFACTCTL_BIN_RESOLVED`) unchanged while centralizing references.
  Rationale: this preserves behavior and avoids coupling refactor-only phase to naming migration semantics.
  Date/Author: 2026-02-25 / Codex

## Outcomes & Retrospective

The original downstream naming phase is intentionally retired. Existing completed branding-surface work remains valid and merged. Phase A is now complete and validated; remaining work is Phase B generator output contract and Phase C command-surface switch.

## Context and Orientation

Current deploy orchestration flow in this repo is:

- `e2e/run_tests.py` checks command availability and capability contracts.
- `e2e/runner/deploy.py` executes deploy/provision/internal fixture operations by command invocation.
- `tools/artifactctl/cmd/artifactctl/main.go` exposes deploy/provision/internal subcommands and user hints.
- `.mise.toml` builds/install command binary used in developer and CI workflows.

Today these areas still include hard-coded command-name literals. “Callsite commonization” means all these literals are first routed through a narrow shared interface (single constants/helpers), and only then switched to the new generated value.

## ESB-CLI Impact Statement

`pkg/*` sharing does not absorb command-name changes when external tools shell out by binary name. Under the new plan, ESB-CLI must consume the generated ctl command name contract (directly or via generated config) once Phase C lands.

## Plan of Work

### Phase A: Callsite Commonization (No Behavior Change)

This phase keeps runtime behavior identical. The default command remains the current value; only reference topology changes.

- Add a shared E2E command contract helper module (single source for command env var keys, required subcommands, capability schema/contract checks, and default binary name).
- Update `e2e/run_tests.py` to consume shared helper functions for:
  - binary resolution
  - capability probe parsing and validation
  - user-facing install/help messages
- Update `e2e/runner/deploy.py` to consume the same helper for binary resolution and cache-key identity.
- Update E2E unit tests to assert through shared contract constants instead of raw command string literals.
- In `tools/artifactctl/cmd/artifactctl/main.go`, centralize command self-name for parser name and help hint strings into one internal constant/helper.
- Update shell diagnostics scripts to use one variable-based command token instead of repeated literals.

Acceptance for Phase A:

- No functional rename yet.
- One place per language defines default command token.
- Existing test behavior remains green.

### Phase B: Branding-Tool Output Contract for `<brand>-ctl`

This phase updates `~/esb-branding-tool` and its templates/contracts.

- Define generated ctl command name as `CtlBinName = "<BRAND_SLUG>-ctl"`.
- Emit this contract in generated artifacts consumed by `esb3` (at minimum Go branding constants; plus whichever shared callsite contract file is selected in Phase A for non-Go consumers).
- Update branding-tool tests/snapshots to assert command-name generation from slug.
- Document exactly which files are generator-owned for command naming to avoid hand-edits.

Acceptance for Phase B:

- `BRAND_SLUG` single input deterministically yields ctl command name.
- No runtime env lookup for slug is needed to decide command name.

### Phase C: Switch `esb3` to Generated `<brand>-ctl`

This phase flips the commonized callsites to the generated contract from Phase B.

- Replace legacy command token defaults in E2E/shared helpers with generated contract value.
- Rename override variables/messages from legacy command-specific naming to neutral naming (`CTL_BIN` family) where needed.
- Update `.mise.toml` build/install tasks to produce the generated command binary name.
- Update docs, E2E runner docs, and operational runbooks to use generated command name.
- Keep Go module/directory path (`tools/artifactctl`) unchanged unless separately planned; this phase is command surface migration, not module-path migration.

Acceptance for Phase C:

- E2E deploy/provision paths invoke generated `<brand>-ctl` only.
- Command capability probing works via shared helper.
- `rg -n "artifactctl|ARTIFACTCTL_BIN" e2e tools docs README.md .mise.toml` returns only intentionally historical references (or zero runtime references).

## Concrete Steps

Workdir: `/home/akira/esb3`.

Phase A implementation loop:

    git switch -c refactor/ctl-callsite-commonization
    rg -n "artifactctl|ARTIFACTCTL" e2e/run_tests.py e2e/runner/deploy.py e2e/runner/tests tools/e2e_proxy/collect_failure_logs.sh tools/artifactctl/cmd/artifactctl/main.go
    # introduce shared helper + rewire callsites (no rename yet)
    go test ./tools/artifactctl/...
    uv run pytest -q e2e/runner/tests/test_run_tests_cli_requirement.py e2e/runner/tests/test_deploy_command.py

Phase B (tool repo) implementation loop:

    cd ~/esb-branding-tool
    git switch -c feat/generated-brand-ctl-name
    # extend templates/contracts to emit CtlBinName from BRAND_SLUG
    uv run pytest -q

Phase C implementation loop:

    cd /home/akira/esb3
    git switch -c feat/use-generated-brand-ctl
    # consume generated command contract in commonized helper
    go test ./tools/artifactctl/... ./pkg/deployops ./pkg/artifactcore ./pkg/composeprovision
    uv run pytest -q e2e/runner/tests
    uv run e2e/run_tests.py --build-only --profile e2e-docker --no-live --no-color --no-emoji --no-cache

## Validation and Acceptance

Phase A validation:

- E2E unit tests still pass with unchanged command behavior.
- Go command tests still pass.
- Grep shows command literals are concentrated in shared helper(s), not scattered.

Phase B validation:

- Branding-tool tests prove `BRAND_SLUG -> <brand>-ctl` output determinism.
- Generated outputs clearly indicate ownership and are reproducible.

Phase C validation:

- Real deploy-phase dry-run succeeds with generated command name.
- Setup/build task installs expected binary name and runner resolves it without runtime brand env.
- Docs and examples match actual command.

## Idempotence and Recovery

Each phase is independently reversible.

- If Phase A fails, revert helper/callsite refactor branch only; behavior remains unchanged.
- If Phase B fails, keep Phase A merged; generator contract can be retried without runtime impact.
- If Phase C fails, revert command-surface switch while keeping Phase A foundation and Phase B generator output for next attempt.

No data migration is involved.

## Artifacts and Notes

Capture in PR descriptions:

- before/after grep snapshots showing callsite literal reduction in Phase A.
- branding-tool test evidence for generated command naming in Phase B.
- end-to-end deploy dry-run command/output summary in Phase C.

## Interfaces and Dependencies

Required end-state interfaces:

- Shared E2E ctl contract helper exposing:
  - default ctl command token
  - override env var keys
  - required subcommand/capability contract versions
  - binary resolve + contract assert helpers
- Generated branding contract exposing ctl binary name from slug.
- CLI main helper constant for self-name used by parser/help hints.

External dependency note:

- `~/esb-branding-tool` must be updated before Phase C can complete.

## Revision Note

Updated on 2026-02-25 to replace the previous downstream plan. The fixed `branding-ctl` rename phase was removed and replaced by a new three-phase plan with refactor-first callsite commonization and final generated `<brand>-ctl` migration.

Updated on 2026-02-25 (Phase A implementation): marked callsite commonization complete, recorded test evidence and env-related test discovery, and logged the decision to keep legacy env var names unchanged during refactor-only phase.

Updated on 2026-02-26 after PR merges #220/#221/#222 and branding-tool #40: marked Phase B and Phase C core migration complete, with residual service-local docs sweep and final deploy-phase validation remaining.
