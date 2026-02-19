# Artifactctl Refactor (Test-First Safety Net)

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan follows `.agent/PLANS.md` and is written as a standalone execution spec.

## Purpose / Big Picture

The next change is to refactor `artifactctl` without breaking artifact-first deploy behavior. Right now the risk is high because coverage is uneven: `tools/artifactctl/cmd/artifactctl/main.go` has no unit tests while `pkg/artifactcore` has partial test coverage. After this plan, we can safely refactor command parsing and core behavior because critical paths are protected by tests that fail on regression.

The user-visible outcome is simple: operators still run the same commands (`validate-id`, `merge`, `prepare-images`, `apply`), but code becomes safer to evolve and future CLI split work is less risky.

## Progress

- [x] (2026-02-19 00:39Z) Established baseline coverage and hotspot map for `tools/artifactctl` and `pkg/artifactcore`.
- [x] (2026-02-19 00:44Z) Implemented Milestone 1: command adapter test seam + `main_test.go` for routing/flags/error propagation.
- [x] (2026-02-19 00:46Z) Implemented Milestone 2: added `manifest`/`merge` branch tests (`ResolveBundleManifest`, write-dir failure, lock owner PID parsing).
- [x] (2026-02-19 00:47Z) Implemented Milestone 3: added `prepare_images` tests for required path, non-map functions payload, no-target skip, default runner empty command.
- [x] (2026-02-19 00:48Z) Completed minimal behavior-preserving refactor and re-ran validation/coverage.

## Surprises & Discoveries

- Observation: `go -C pkg/artifactcore test ./...` fails under workspace mode because `go.work` module roots do not include `pkg/artifactcore` as a direct root for that invocation style.
  Evidence: command output included `directory /home/akira/esb/pkg/artifactcore is outside module roots (...)`.

- Observation: Test seam extraction in `tools/artifactctl/cmd/artifactctl/main.go` can preserve command UX while removing `os.Exit` dependence from tests.
  Evidence: `tools/artifactctl/cmd/artifactctl/main_test.go` now validates unknown command, required flags, and backend errors without invoking process termination.

## Decision Log

- Decision: Run test-first and hold functional behavior constant during refactor.
  Rationale: `artifactctl` is operational tooling; regressions would break deploy/apply flows across E2E and operators.
  Date/Author: 2026-02-19 / Codex

- Decision: Keep scope to `tools/artifactctl` and `pkg/artifactcore` only; no new product behavior.
  Rationale: This work is a safety-net and structure improvement step before broader changes.
  Date/Author: 2026-02-19 / Codex

- Decision: Replace direct `main -> run* -> exitf` flow with `run(args,deps) error` and keep `main` as a thin `exitf` wrapper.
  Rationale: Allows deterministic command adapter unit tests while preserving CLI surface and error text.
  Date/Author: 2026-02-19 / Codex

- Decision: Use `flag.ContinueOnError` with flag output discarded for adapter tests.
  Rationale: Keeps parser behavior while avoiding noisy/stable-assertion-unfriendly stderr output in unit tests.
  Date/Author: 2026-02-19 / Codex

- Decision: Use `GOWORK=off` for direct `pkg/artifactcore` coverage commands in this plan’s validation.
  Rationale: Avoid workspace-root ambiguity and make package-local coverage runs reliable.
  Date/Author: 2026-02-19 / Codex

## Outcomes & Retrospective

The plan achieved its immediate goal: refactor safety for `artifactctl` and key `artifactcore` branches improved without changing operational behavior.

Measured outcomes:
1. `tools/artifactctl` coverage increased from 0.0% to 87.9% for the command adapter package.
2. `pkg/artifactcore` coverage increased from 72.9% to 76.2%.
3. Previously blind critical branch `ResolveBundleManifest` moved from 0.0% to 77.8%; `prepare_images.Run` moved from 0.0% to 33.3%; `values.go` conversion helpers are now fully covered.

Remaining gaps:
1. `isProcessAlive` remains partial (50.0%) due OS-behavior-dependent branches.
2. `WriteArtifactManifest` remains moderate (51.5%); deeper encoder/chmod/rename failure simulation is still open for future hardening.
3. `prepare_images.Run` remains low (33.3%) because most behavior is delegated to `PrepareImages`; additional CLI-style invocation branch tests would be needed for higher coverage.

## Context and Orientation

`tools/artifactctl/cmd/artifactctl/main.go` is the command adapter. It parses subcommands and flags and calls `pkg/artifactcore`. It currently has no tests, so malformed arguments and command dispatch behavior are not protected.

`pkg/artifactcore` is the core implementation for manifest validation, runtime-config merge/apply, and image preparation. This module has tests, but coverage gaps remain in high-risk areas:

1. `manifest.go` bundle path resolution and write error branches.
2. `merge.go` lock recovery and PID-related branches.
3. `prepare_images.go` entry path (`Run`) and selected command-building/error branches.

This plan does not change runtime contract or command surface. It only adds tests and refactors internals in behavior-preserving increments.

## Plan of Work

Milestone 1 builds a thin, testable seam in `tools/artifactctl/cmd/artifactctl/main.go`. We will extract dispatch into a function that accepts args and a dependency interface (or function set) so tests can assert subcommand routing and required-flag failures without invoking real `os.Exit`.

Milestone 2 strengthens manifest and merge lock coverage in `pkg/artifactcore`. We will add table-driven tests for `ResolveBundleManifest`, `WriteArtifactManifest` error branches (invalid path/write failure), and lock helper behavior such as stale lock owner parsing and process-alive checks under invalid PID text.

Milestone 3 strengthens `prepare_images` coverage by testing entry-point behavior and key branch combinations with a fake command runner. We will focus on preserving current command composition semantics (`buildx`, `--no-cache`, target rewrite) and explicit failure behavior.

Milestone 4 applies small refactors after tests are in place. Refactors must be internal only (naming/structure extraction), keeping CLI behavior and error messages stable unless tests are intentionally updated with rationale in Decision Log.

## Concrete Steps

All commands run from repository root: `/home/akira/esb`.

1. Baseline coverage and function map.
   - `go -C tools/artifactctl test ./... -coverprofile=/tmp/cover_artifactctl.out`
   - `go tool cover -func=/tmp/cover_artifactctl.out`
   - `GOWORK=off go -C pkg/artifactcore test ./... -coverprofile=/tmp/cover_artifactcore.out`
   - `go -C pkg/artifactcore tool cover -func=/tmp/cover_artifactcore.out`

2. Add tests for command adapter.
   - Create `tools/artifactctl/cmd/artifactctl/main_test.go`.
   - Add cases for:
     - unknown command
     - missing command
     - each subcommand missing required flags
     - backend failure propagation
     - successful routing for all subcommands

3. Add tests for `manifest` and `merge` edge behavior.
   - Extend `pkg/artifactcore/manifest_test.go`.
   - Extend `pkg/artifactcore/merge_test.go`.
   - Specifically cover:
     - `ResolveBundleManifest`
     - `WriteArtifactManifest` failure paths
     - invalid lock owner and stale lock recovery branches

4. Add tests for `prepare_images` entry and branch matrix.
   - Extend `pkg/artifactcore/prepare_images_test.go`.
   - Cover `Run` and branch combinations around cache flags and rewrite behavior.

5. Refactor in small commits under green tests.
   - Keep external behavior unchanged.
   - Use one concern per commit.

6. Final verification.
   - `go -C tools/artifactctl test ./...`
   - `GOWORK=off go -C pkg/artifactcore test ./...`
   - `go -C tools/artifactctl test ./... -coverprofile=/tmp/cover_artifactctl.after.out`
   - `go tool cover -func=/tmp/cover_artifactctl.after.out`
   - `GOWORK=off go -C pkg/artifactcore test ./... -coverprofile=/tmp/cover_artifactcore.after.out`
   - `go -C pkg/artifactcore tool cover -func=/tmp/cover_artifactcore.after.out`

## Validation and Acceptance

Acceptance is based on behavior and coverage, not only compilation.

1. Command adapter behavior is fully test-covered.
   - `tools/artifactctl/cmd/artifactctl/main.go` has unit tests that verify subcommand selection, required flag validation, and error propagation.

2. Core risky branches are now guarded.
   - `ResolveBundleManifest` and `WriteArtifactManifest` failure paths are covered by tests.
   - Merge lock recovery/error branches have explicit test coverage.
   - `prepare_images` entry path and key branch logic have explicit test coverage.

3. Coverage improvement is measurable.
   - `tools/artifactctl` no longer reports 0.0% statement coverage.
   - `pkg/artifactcore` maintains or improves baseline and removes 0% critical function blind spots identified in this plan.

4. Behavior lock is preserved.
   - Existing command surface and operational flow remain unchanged (`validate-id`, `merge`, `prepare-images`, `apply`).

## Idempotence and Recovery

The steps are idempotent. Re-running tests and coverage commands is safe.

If a refactor commit introduces regression:
1. Stop additional refactors.
2. Re-run failing focused test package.
3. Fix or revert only the offending commit.
4. Update `Decision Log` with the branch that failed and the corrective action.

No destructive repository operations are required.

## Artifacts and Notes

Evidence to attach in PR description:
1. Before/after coverage snippets for `tools/artifactctl` and `pkg/artifactcore`.
2. List of newly added tests with file paths.
3. Confirmation that command surface is unchanged.

## Interfaces and Dependencies

`tools/artifactctl/cmd/artifactctl/main.go` should expose a testable dispatch seam that does not require invoking process exit in tests. The implementation may use an internal function such as:

- `run(args []string, deps commandDeps) error`

`commandDeps` should include callable hooks for:

- `ValidateIDs(path string) error`
- `MergeRuntimeConfig(req artifactcore.MergeRequest) error`
- `PrepareImages(req artifactcore.PrepareImagesRequest) error`
- `Apply(req artifactcore.ApplyRequest) error`

This keeps the adapter thin and behavior-preserving while making routing testable.

`pkg/artifactcore` public signatures must remain unchanged during this refactor cycle:

- `ValidateIDs(artifactPath string) error`
- `MergeRuntimeConfig(req MergeRequest) error`
- `PrepareImages(req PrepareImagesRequest) error`
- `Apply(req ApplyRequest) error`
- `ReadArtifactManifest(path string) (ArtifactManifest, error)`
- `WriteArtifactManifest(path string, manifest ArtifactManifest) error`

## Plan Revision Note

Created on 2026-02-19 to start artifactctl refactor with a test-first safety baseline after Track I/O completion and PR #167 merge.

Updated on 2026-02-19 after Milestones 1-4 execution with completed progress, refreshed validation evidence, and residual risk notes.
