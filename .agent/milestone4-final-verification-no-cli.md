# Milestone 4 Plan: Final Verification in No-CLI State

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

Validate runtime/tooling behavior after CLI removal from this repository.

## Progress

- [x] (2026-02-20) Run boundary/layout checks.
- [x] (2026-02-20) Run `tools/artifactctl` and `pkg/*` tests with `GOWORK=off`.
- [x] (2026-02-20) Run full E2E with local `artifactctl` binary.
- [x] (2026-02-20) Update master plan outcomes.

## Surprises & Discoveries

- Observation: direct `pytest e2e/runner` requires env bootstrap values and fails without them.
  Evidence: `X_API_KEY is required` runtime error when env values were not set.
- Observation: full E2E matrix remains green after CLI removal from this repository.
  Evidence: docker + containerd matrix completed with `[PASSED] ALL MATRIX ENTRIES PASSED!`.

## Decision Log

- Decision: E2E full pass is required before merge.
  Rationale: split must not regress actual deploy/apply operation.
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 4 completed.

Validated commands:

- `./tools/ci/check_tooling_boundaries.sh`
- `./tools/ci/check_repo_layout.sh`
- `GOWORK=off go -C tools/artifactctl test ./...`
- `GOWORK=off go -C pkg/artifactcore test ./...`
- `GOWORK=off go -C pkg/composeprovision test ./...`
- `GOWORK=off go -C pkg/deployops test ./...`
- `GOWORK=off go -C pkg/runtimeimage test ./...`
- `GOWORK=off go -C pkg/yamlshape test ./...`
- `uv run pytest services/gateway/tests -v`
- `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner -q`
- `go -C services/agent test ./...`
- `ARTIFACTCTL_BIN=/tmp/artifactctl-local uv run e2e/run_tests.py --parallel --verbose --cleanup`

## Revision Notes

- 2026-02-20: Initial milestone detail created.
