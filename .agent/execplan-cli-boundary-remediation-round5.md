# CLI Boundary Remediation Round 5 (Contract Cleanup)

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

Round 4 removed large boundary leaks, but strict re-review found remaining contract inconsistencies that can still cause drift: manual pre-parse flag logic in CLI, duplicated route-key logic across packages, duplicated output-dir ownership in deploy context/request, and an overly broad public surface in `pkg/artifactcore`.

After this round, `esb` and `artifactctl` behavior remains unchanged for users, while internal boundaries become clearer and less drift-prone. The result should be observable by passing unit tests and full E2E scenario execution.

## Progress

- [x] (2026-02-19 09:52Z) Created Round 5 plan with fixed scope and acceptance gates.
- [ ] Implement P0: remove manual value-flag list coupling from CLI repo-scope precheck and make command detection robust without hardcoded per-flag value semantics.
- [ ] Implement P1: centralize route key generation in `pkg/yamlshape` and delete duplicated local implementations.
- [ ] Implement P2: remove `state.Context.OutputDir` duplication and keep `deploy.Request.OutputDir` as the single owner in deploy usecase path.
- [ ] Implement P3: reduce `pkg/artifactcore` public API by internalizing image-prepare entrypoint not consumed cross-module.
- [ ] Run regression gates (`go test` for cli/tools/pkg + targeted e2e runner tests).
- [ ] Run full E2E and confirm all scenarios pass.
- [ ] Perform strict post-implementation architecture review; if findings remain, open Round 6 plan and repeat.

## Surprises & Discoveries

- Observation: previous CLI helper relied on `commandFlagExpectsValue` and missed `--compose-file`, proving manual value-flag bookkeeping is brittle.
  Evidence: `cli/internal/command/app.go` had no `--compose-file` in value-flag list while `DeployCmd` defines it.

## Decision Log

- Decision: Treat this as a fixed-scope remediation round before any further feature work.
  Rationale: Re-review churn is reduced when acceptance scope is explicit and finite.
  Date/Author: 2026-02-19 / Codex

## Outcomes & Retrospective

Pending completion.

## Context and Orientation

Relevant modules and why they matter:

- `cli/internal/command/app.go`: parses raw args and decides whether command must run inside repo root.
- `cli/internal/usecase/deploy/config_diff.go`: computes config merge diffs and currently defines local route key logic.
- `pkg/artifactcore/merge_yaml.go`: merges runtime YAML and currently defines the same route key logic.
- `pkg/yamlshape/shape.go`: shared YAML shape helper package suitable for low-level route-key normalization utility.
- `cli/internal/domain/state/context.go` and `cli/internal/usecase/deploy/deploy.go`: request/context contracts where output directory ownership must be unambiguous.
- `pkg/artifactcore/prepare_images.go` and `pkg/artifactcore/execute.go`: deploy execution path where prewarm image preparation currently has extra exported API.

## Plan of Work

First, rework CLI repo-scope precheck logic so command detection is robust without a list of "flags that take values". This removes a drift class where adding a flag silently breaks help/version gating.

Second, move route-key construction to `pkg/yamlshape` and replace both local implementations in `cli` and `artifactcore`.

Third, delete `OutputDir` from `state.Context` and keep it only in `deploy.Request` for deploy usecase ownership.

Fourth, make prewarm image preparation an internal artifactcore detail and keep `ExecuteDeploy` as the external orchestration entrypoint.

Finally, run regression tests and full E2E. Then run a strict architecture re-review against the same concerns.

## Concrete Steps

Run from `/home/akira/esb`.

1) Implement contract cleanup edits in the files listed above.

2) Run focused tests:

    go -C cli test ./internal/command ./internal/usecase/deploy ./internal/infra/deploy
    go -C tools/artifactctl test ./...
    GOWORK=off go -C pkg/artifactcore test ./...
    X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest -q e2e/runner/tests/test_run_tests_cli_requirement.py e2e/runner/tests/test_deploy_command.py

3) Run full E2E:

    uv run e2e/run_tests.py --parallel --verbose

## Validation and Acceptance

Acceptance criteria:

- CLI repo-scope decision no longer depends on hardcoded per-flag value list.
- `routeKey` implementation exists in one shared location and is reused.
- deploy contracts no longer duplicate output-dir ownership between context and request.
- `pkg/artifactcore` exposes only necessary APIs for cross-module callers for deploy image-prepare path.
- `go test` gates pass for `cli`, `tools/artifactctl`, `pkg/artifactcore`.
- Full E2E (`uv run e2e/run_tests.py --parallel --verbose`) passes.

## Idempotence and Recovery

All edits are source-level and idempotent. If a step fails:

- Re-run the exact test command to reproduce.
- Fix only the failing scope.
- Re-run focused tests before full E2E.

No destructive git operations are required.

## Artifacts and Notes

Implementation evidence and command summaries will be appended after execution.

## Interfaces and Dependencies

Expected end-state:

- `cli` uses stable deploy contracts without duplicate ownership fields.
- `pkg/yamlshape` provides shared route-key shaping utility used by both `cli` and `artifactcore`.
- `artifactcore.ExecuteDeploy` remains the public deploy orchestration API; internal image-prep internals are not exported.

## Plan Revision Note

Created on 2026-02-19 for the next strict remediation cycle following Round 4, with explicit requirement to pass full E2E at cycle end.
