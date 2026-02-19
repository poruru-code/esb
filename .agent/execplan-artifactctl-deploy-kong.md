# Artifactctl Deploy One-Command + Kong Migration

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document follows `.agent/PLANS.md` and must be maintained in accordance with it.

## Purpose / Big Picture

After this change, operators will run a single `artifactctl deploy` command to perform the full artifact apply phase: image preparation plus config apply. They will no longer need to remember separate `prepare-images` and `apply` commands for normal operation. This reduces operational steps and keeps `artifactctl` focused on artifact consumption, while artifact generation remains in `esb artifact generate`.

The user-visible result is:

- `artifactctl` uses the same parser framework as the existing CLI (`github.com/alecthomas/kong`).
- `artifactctl deploy --artifact ... --out ...` becomes the only supported command surface.
- E2E runner deploy path calls this single command.
- docs describe the new one-command apply path.

## Progress

- [x] (2026-02-19 01:17Z) Baseline analysis completed: identified current command surface, E2E call points, and docs references that must change.
- [x] (2026-02-19 01:22Z) Implemented Kong-based `artifactctl deploy` command and removed legacy subcommand surface.
- [x] (2026-02-19 01:23Z) Updated E2E deploy runner and tests to call single `artifactctl deploy` command.
- [x] (2026-02-19 01:25Z) Updated operational/contract/runtime docs to one-command apply path (`artifactctl deploy`).
- [x] (2026-02-19 01:26Z) Executed validation suite for modified Go/Python paths and confirmed pass.
- [x] (2026-02-19 01:57Z) Executed full E2E matrix with new deploy command path and confirmed all scenarios pass.
- [x] (2026-02-19 02:03Z) Executed full UT scope used in CI (`services/gateway`, `e2e/runner`, `services/agent`) plus touched Go modules and confirmed pass.

## Surprises & Discoveries

- Observation: Current docs frequently instruct `tools/artifactctl ...` command forms, but `tools/artifactctl` is a directory path, not an executable command name.
  Evidence: running `tools/artifactctl validate-id --artifact ...` returns `/bin/bash: tools/artifactctl: Is a directory`.

- Observation: The E2E runner currently assembles two separate artifactctl invocations (`prepare-images` then `apply`), which must be changed together with CLI surface changes to avoid immediate runner breakage.
  Evidence: `e2e/runner/deploy.py` currently builds two commands at lines invoking `prepare-images` and `apply`.

- Observation: Kong default help flow triggers `os.Exit(0)` directly, which breaks unit-testable `run()` wrappers unless exit behavior is intercepted.
  Evidence: `go test` initially failed with `panic: unexpected call to os.Exit(0) during test` from `github.com/alecthomas/kong/help.go`.

- Observation: Running runner unit tests directly requires E2E auth env variables due shared `e2e/conftest.py` guards.
  Evidence: direct `uv run pytest ...` failed until `X_API_KEY`, `AUTH_USER`, and `AUTH_PASS` were set.

- Observation: Local PATH may still point to a stale pre-migration `artifactctl` binary that does not support `deploy`.
  Evidence: first full E2E re-run failed with `unknown command: deploy` until run with `ARTIFACTCTL_BIN` targeting newly built binary.

## Decision Log

- Decision: Backward compatibility for old artifactctl subcommands is intentionally dropped in this change.
  Rationale: user explicitly requested “あるべき論” and allowed removing legacy subcommands; this is necessary to reduce operational complexity.
  Date/Author: 2026-02-19 / Codex

- Decision: artifact generation remains outside artifactctl and stays in `esb artifact generate`.
  Rationale: artifactctl scope is apply-phase consumption of already-generated artifacts; this preserves separation for future CLI repo split.
  Date/Author: 2026-02-19 / Codex

- Decision: adopt `kong` in artifactctl to align parser behavior with existing CLI architecture.
  Rationale: shared parser model improves consistency of help/usage behavior and long-term maintenance.
  Date/Author: 2026-02-19 / Codex

- Decision: keep `artifactctl deploy` as the only command and run `PrepareImages` then `Apply` in-process with stage-specific error prefixes.
  Rationale: one-command UX is the explicit objective; stage-prefix errors preserve diagnosability without exposing multiple user commands.
  Date/Author: 2026-02-19 / Codex

- Decision: override Kong `Exit` behavior inside `run()` to recover exit codes (notably help=0) instead of terminating process.
  Rationale: enables deterministic unit tests and preserves CLI behavior for help/parse exit semantics.
  Date/Author: 2026-02-19 / Codex

## Outcomes & Retrospective

The plan goal was achieved: artifactctl operator path is now one command (`deploy`) on a Kong parser, and E2E runner/developer docs were aligned to that new contract.

Validation outcomes:

- `go -C tools/artifactctl test ./...` passed.
- `GOWORK=off go -C pkg/artifactcore test ./...` passed.
- `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner/tests/test_deploy_command.py` passed (10 tests).
- `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner/tests/test_run_tests_cli_requirement.py` passed (8 tests).
- `ARTIFACTCTL_BIN=/tmp/artifactctl-deploy-kong uv run e2e/run_tests.py --parallel --verbose` passed full matrix:
  - `e2e-containerd`: 45 passed
  - `e2e-docker`: 53 passed
- Full UT passes:
  - `DISABLE_VICTORIALOGS=1 uv run pytest services/gateway/tests -v`: 149 passed
  - `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner -q`: 66 passed
  - `go -C services/agent test ./...`: pass
  - `go -C tools/artifactctl test ./...`: pass
  - `GOWORK=off go -C pkg/artifactcore test ./...`: pass

Residual follow-up:

- `esb artifact apply` adapter behavior is intentionally unchanged in this plan and should be reviewed separately if strict one-command parity with `artifactctl deploy` is required in CLI UX.

## Context and Orientation

`tools/artifactctl` is the operational apply-phase tool. Its command adapter lives in `tools/artifactctl/cmd/artifactctl/main.go`, while logic is in `pkg/artifactcore`.

Today, artifactctl still exposes four commands (`validate-id`, `merge`, `prepare-images`, `apply`). The E2E deploy path in `e2e/runner/deploy.py` calls two of them in sequence. The requested redesign is to make artifactctl operator UX one-command only via `deploy`, where “deploy” means:

- prepare function/base images from artifact (`PrepareImages`)
- apply artifact runtime config into `CONFIG_DIR` (`Apply`)

This plan does not move artifact generation into artifactctl. Artifact generation stays in CLI (`esb artifact generate`).

## Plan of Work

Milestone 1 rewrites the artifactctl command adapter to Kong and narrows public surface to one command. In `tools/artifactctl/cmd/artifactctl/main.go`, replace flag-based parser logic with Kong struct definitions, dispatch only `deploy`, and invoke `artifactcore.PrepareImages` then `artifactcore.Apply` internally. Preserve strict mode and secret-env behavior through `artifactcore.ApplyRequest`, and preserve no-cache behavior through `artifactcore.PrepareImagesRequest`.

Milestone 2 updates tests for the new command contract. Replace legacy subcommand tests in `tools/artifactctl/cmd/artifactctl/main_test.go` with tests that verify: required arguments, help behavior, no-cache/strict/secret-env propagation, execution order (prepare before apply), and stage-specific error propagation.

Milestone 3 updates E2E runner integration. In `e2e/runner/deploy.py`, replace separate prepare/apply command assembly with one `artifactctl deploy ...` call. Keep provisioner invocation unchanged. Update runner unit tests in `e2e/runner/tests/test_deploy_command.py` to match the new single command.

Milestone 4 synchronizes documentation. Update `docs/artifact-operations.md`, `docs/deploy-artifact-contract.md`, and `docs/container-runtime-operations.md` so operational guidance uses `artifactctl deploy` instead of removed subcommands. Also sync dependent subsystem docs (`cli/docs/container-management.md`, `services/agent/docs/*`, and fixture README) so no operator-facing document suggests removed subcommands. Where relevant, explain that `deploy` internally performs image preparation and apply.

## Concrete Steps

Run all commands from repository root `/home/akira/esb`.

1. Implement artifactctl parser and command surface rewrite.
   - Edit `tools/artifactctl/cmd/artifactctl/main.go`.
   - Add `github.com/alecthomas/kong` dependency to `tools/artifactctl/go.mod`.

2. Rewrite artifactctl unit tests.
   - Edit `tools/artifactctl/cmd/artifactctl/main_test.go`.

3. Update E2E deploy invocation and unit tests.
   - Edit `e2e/runner/deploy.py`.
   - Edit `e2e/runner/tests/test_deploy_command.py`.

4. Update docs for the new UX.
   - Edit `docs/artifact-operations.md`.
   - Edit `docs/deploy-artifact-contract.md`.
   - Edit `docs/container-runtime-operations.md`.
   - Edit `cli/docs/container-management.md`.
   - Edit `services/agent/docs/README.md`.
   - Edit `services/agent/docs/architecture.md`.
   - Edit `services/agent/docs/configuration.md`.
   - Edit `services/agent/docs/runtime-containerd.md`.
   - Edit `tools/e2e-lambda-fixtures/python/README.md`.

5. Validate with tests.
   - `go -C tools/artifactctl test ./...`
   - `GOWORK=off go -C pkg/artifactcore test ./...`
   - `uv run pytest e2e/runner/tests/test_deploy_command.py`
   - `uv run pytest e2e/runner/tests/test_run_tests_cli_requirement.py`

Expected result: all commands above pass with zero failures, and no references remain in modified docs to removed artifactctl subcommands as operational path.

## Validation and Acceptance

Acceptance criteria are behavioral.

- Running `artifactctl deploy --help` prints command usage and exits successfully.
- Running `artifactctl deploy --artifact <path> --out <dir>` triggers image preparation first, then apply.
- If prepare fails, apply is not executed.
- E2E runner deploy phase issues one artifactctl command (deploy) before provisioner.
- Docs for non-CLI apply path show one artifactctl command, not two or more.

## Idempotence and Recovery

Edits are source-level and safe to re-run. Tests are repeatable. If a milestone fails validation, revert only the touched files in that milestone and re-run the milestone-specific tests before proceeding.

No destructive data operations are included.

## Artifacts and Notes

Implementation evidence to include in final report:

- updated command help output for `artifactctl deploy --help`
- test pass summaries for Go and Python runner unit tests
- concise diff summary for command and docs updates

## Interfaces and Dependencies

At completion, `tools/artifactctl/cmd/artifactctl/main.go` must expose one public CLI operation via Kong:

- command: `deploy`
- required flags: `--artifact`, `--out`
- optional flags: `--secret-env`, `--strict`, `--no-cache`

Execution contract:

- `artifactcore.PrepareImages(PrepareImagesRequest{ArtifactPath, NoCache})`
- then `artifactcore.Apply(ApplyRequest{ArtifactPath, OutputDir, SecretEnvPath, Strict, WarningWriter})`

No new dependency from artifactctl to CLI modules is introduced.

## Plan Revision Note

Created on 2026-02-19 to execute Track G UX simplification by consolidating artifactctl apply-phase operations into a single Kong-based deploy command.

Updated on 2026-02-19 after implementation to record Kong help-exit handling, E2E test env prerequisites, completed milestones, and validation evidence.
