# Split `pkg/deployops/prepare_images.go` Responsibilities Without Behavior Change

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

After this change, image-prepare logic remains functionally identical, but the code is organized by responsibility so future proxy/base-image/runtime changes can be made safely in narrow files. A user can validate equivalence by running existing deployops tests and observing no regression while `prepare_images.go` becomes smaller and easier to navigate.

## Progress

- [x] (2026-02-24 12:21Z) Captured current-state evidence: `pkg/deployops/prepare_images.go` is 775 lines and mixes orchestration, Dockerfile rewriting, Maven shim build, command assembly, and filesystem helpers.
- [x] (2026-02-24 12:21Z) Defined refactor boundary as internal file split only (same package, same function signatures where used by tests).
- [x] (2026-02-24 12:24Z) Completed file split into responsibility-focused files with no intended behavior change.
- [x] (2026-02-24 12:24Z) Ran deployops test suite and targeted validations: both targeted and full package tests passed.
- [x] (2026-02-24 12:33Z) Split `prepare_images_test.go` into responsibility-focused test files and reran full deployops tests.
- [x] (2026-02-24 12:24Z) Recorded outcomes and residual risks.

## Surprises & Discoveries

- Observation: Existing tests in `pkg/deployops/prepare_images_test.go` directly exercise non-exported helpers such as `rewriteDockerfileForMavenShim` and `buildxBuildCommand`.
  Evidence: `rg` hits in test file reference these symbols directly.

- Observation: `lambda_base.go` depends on helper `fromImageTokenIndex`, so moving that helper must keep package-level accessibility.
  Evidence: function `readLambdaBaseRef` calls `fromImageTokenIndex`.

- Observation: Pure file split was sufficient; no helper signature change was required to keep all existing deployops tests green.
  Evidence: `go test ./pkg/deployops -count=1` passed after split.

- Observation: Test-file split required extracting shared fixtures and runner spy into a dedicated helper test file to avoid duplicate declarations.
  Evidence: Introduced `pkg/deployops/prepare_images_test_helpers_test.go` and removed single monolithic `prepare_images_test.go`.

## Decision Log

- Decision: Perform a pure structural refactor (file split) without introducing new abstractions or changing existing function contracts.
  Rationale: User concern is file bloat and responsibility clarity; behavior changes would add avoidable risk.
  Date/Author: 2026-02-24 / Codex

- Decision: Keep the package boundary as `pkg/deployops` and split by concern into multiple files in the same package.
  Rationale: This improves readability while minimizing test impact and avoiding cross-package churn.
  Date/Author: 2026-02-24 / Codex

- Decision: Keep `prepare_images.go` as orchestration entrypoint and move helpers to four focused sibling files (`*_dockerfile.go`, `*_maven.go`, `*_cmd.go`, `*_fs.go`).
  Rationale: This preserves discoverability from the existing entrypoint while reducing cognitive load per file.
  Date/Author: 2026-02-24 / Codex

- Decision: Mirror production-file split in tests by concern (`core`, `dockerfile`, `maven`, `cmd`, `base`) and centralize shared test helpers.
  Rationale: Keeping test structure aligned with production responsibilities reduces review and maintenance cost.
  Date/Author: 2026-02-24 / Codex

## Outcomes & Retrospective

The refactor completed as planned. `prepare_images.go` was reduced from 775 lines to 237 lines while preserving behavior and tests. Responsibilities are now separated into focused files within the same package:

- `pkg/deployops/prepare_images_dockerfile.go`
- `pkg/deployops/prepare_images_maven.go`
- `pkg/deployops/prepare_images_cmd.go`
- `pkg/deployops/prepare_images_fs.go`

No public API changes were introduced. Residual risk is low and limited to future drift across files; existing tests currently cover the moved helpers sufficiently.

In follow-up refinement, tests were also split by responsibility and validated. This removed the single 977-line test file and aligned test ownership with the new code layout.

## Context and Orientation

`pkg/deployops/prepare_images.go` is the deploy-time function image preparation path used by artifact-driven deployment flows. Before this refactor it contained five distinct concerns:

1. Orchestration: iterate artifact manifests, gather function targets, call build/push.
2. Dockerfile transformation: registry alias rewrite, lambda base tag rewrite, Maven shim rewrite.
3. Maven shim lifecycle: proxy validation, shim image tag derivation, shim image build/push.
4. Build command composition: `docker buildx build` arguments, proxy build args, push target rewrite.
5. Filesystem utilities: temporary workspace copy, YAML loader, sorting helpers.

The resulting state keeps all logic in `pkg/deployops` but distributes these concerns into dedicated files so each file has one dominant reason to change.

## Plan of Work

First, reduce `prepare_images.go` to orchestration and shared small type definitions used by orchestration tests. Then move Dockerfile-rewrite helpers into a dedicated file, keeping function names unchanged so tests remain valid. Next move Maven shim routines into another file, preserving use of `proxymaven.ResolveEndpointsFromEnv` and existing image-tag derivation. Then move command composition helpers (`buildxBuildCommandWithBuildArgs`, proxy arg append, push ref resolver, image existence probe) into a command-focused file. Finally move workspace and file-copy helpers (`withFunctionBuildWorkspace`, `copyDir`, `copyFile`, `loadYAML`, `sortedUniqueNonEmpty`) to a filesystem-focused file.

Throughout the split, keep all functions package-private and preserve call sites. No behavior logic changes are intended.

## Concrete Steps

Run all commands from `/home/akira/esb3`.

1. Implement file split and compile.

    go test ./pkg/deployops -run TestPrepareImagesBuildsAndPushesDockerRefs -count=1

Observed: pass (`ok github.com/poruru-code/esb/pkg/deployops 0.026s`).

2. Run full deployops package tests.

    go test ./pkg/deployops -count=1

Observed: pass (`ok github.com/poruru-code/esb/pkg/deployops 1.086s`, and post-test-split rerun `0.798s`).

3. Capture shape-change evidence.

    wc -l pkg/deployops/prepare_images.go

Observed: `237` lines.

## Validation and Acceptance

Acceptance criteria met.

- `pkg/deployops/prepare_images.go` is materially smaller and orchestration-focused.
- Helper logic is split into dedicated files by concern.
- `go test ./pkg/deployops -count=1` passes.
- Existing tests that directly call moved helper functions continue to pass without test rewrites.
- Tests are no longer monolithic; they are grouped by the same responsibilities as production files.

## Idempotence and Recovery

This is a non-destructive refactor. Re-running the split is safe because no stateful migration exists. If compile issues appear in follow-up edits, re-run `go test ./pkg/deployops -count=1` and move only missing helper references between same-package files.

## Artifacts and Notes

Key evidence from implementation:

- Before: `prepare_images.go` 775 lines.
- After: `prepare_images.go` 237 lines.
- New files added:
  - `pkg/deployops/prepare_images_dockerfile.go`
  - `pkg/deployops/prepare_images_maven.go`
  - `pkg/deployops/prepare_images_cmd.go`
  - `pkg/deployops/prepare_images_fs.go`
- Test files reorganized:
  - `pkg/deployops/prepare_images_core_test.go`
  - `pkg/deployops/prepare_images_dockerfile_test.go`
  - `pkg/deployops/prepare_images_maven_test.go`
  - `pkg/deployops/prepare_images_cmd_test.go`
  - `pkg/deployops/prepare_images_base_test.go`
  - `pkg/deployops/prepare_images_test_helpers_test.go`
  - removed `pkg/deployops/prepare_images_test.go`
- Validation:
  - `go test ./pkg/deployops -run TestPrepareImagesBuildsAndPushesDockerRefs -count=1` passed.
  - `go test ./pkg/deployops -count=1` passed.

## Interfaces and Dependencies

No public interfaces changed. The same package-private functions remain available inside `pkg/deployops`, including helpers referenced by tests and sibling files:

- `prepareImages`, `prepareImagesWithResult`, `collectImageBuildTargets`, `buildAndPushFunctionImage`
- `resolveFunctionBuildDockerfile`, `rewriteDockerfileForBuild`, `rewriteDockerfileForMavenShim`, `rewriteLambdaBaseTag`, `fromImageTokenIndex`
- `ensureMavenShimImage`, `validateMavenShimProxyEnv`, `resolveMavenShimBuildPaths`
- `buildxBuildCommandWithBuildArgs`, `buildxBuildCommand`, `appendProxyBuildArgs`, `resolvePushReference`, `dockerImageExists`
- `withFunctionBuildWorkspace`, `copyDir`, `copyFile`, `loadYAML`, `sortedUniqueNonEmpty`

Revision note (2026-02-24 12:21Z): Initial plan created to satisfy required design-before-implementation flow for a significant refactor.
Revision note (2026-02-24 12:24Z): Updated plan with implemented file split, validation evidence, and completion outcomes.
Revision note (2026-02-24 12:33Z): Added test-file responsibility split and validation evidence after review feedback.
