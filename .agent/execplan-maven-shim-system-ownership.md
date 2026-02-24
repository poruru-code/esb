# Promote Maven Shim to Deploy Core Asset and Remove `tools/` Coupling

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

After this change, Maven proxy handling used during deploy-time image builds will no longer depend on `tools/maven-shim` filesystem placement or current working directory. The deploy core will own Maven shim lifecycle as a first-class system capability, and both `artifactctl deploy` and E2E fixture builds will consume the same shim contract. A user can verify success by running deploy/build flows from any working directory and observing that Maven downloads succeed through proxy without ad-hoc Dockerfile rewrites or path assumptions.

## Progress

- [x] (2026-02-24 20:30Z) Collected current-state evidence: `pkg/deployops` and E2E runner both depend on `tools/maven-shim` path conventions.
- [x] (2026-02-24 20:30Z) Confirmed layout contract and deploy contract boundaries relevant to system-vs-tools ownership.
- [x] (2026-02-24 20:35Z) Drafted zero-based refactor plan that promotes Maven shim to deploy core ownership.
- [x] (2026-02-24 20:42Z) Reconsidered E2E invocation after ownership change and fixed plan to artifactctl-mediated JSON contract with fail-fast parsing.
- [x] (2026-02-24 20:50Z) Architecture review #1: added explicit artifactctl prerequisite and fail-fast path for E2E shim resolution.
- [x] (2026-02-24 20:52Z) Architecture review #2: hardened CLI contract with versioned JSON schema and stable `--output json` mode.
- [x] (2026-02-24 20:54Z) Architecture review #3: enforced proxy credential handling through environment-only inputs and redacted logging.
- [x] (2026-02-24 20:56Z) Architecture review #4: added cross-process deduplication lock for shim image build/push.
- [x] (2026-02-24 20:58Z) Architecture review #5: added staged migration and compatibility window before deleting `tools/maven-shim`.
- [ ] Implement deploy-core Maven shim asset package and remove direct `tools/maven-shim` runtime dependency.
- [ ] Introduce single shim ensure API reachable from both `artifactctl` and E2E runner.
- [ ] Run proxy-backed acceptance matrix for artifactctl and E2E paths.

## Surprises & Discoveries

- Observation: `pkg/deployops` currently resolves Maven shim Dockerfile by walking up from `os.Getwd()` and expecting `tools/maven-shim/Dockerfile`.
  Evidence: `pkg/deployops/prepare_images.go` function `resolveMavenShimBuildPaths`.

- Observation: E2E runner independently references shim context as `PROJECT_ROOT / "tools" / "maven-shim"`.
  Evidence: `e2e/runner/deploy.py` constants `_MAVEN_SHIM_CONTEXT` and `_MAVEN_SHIM_DOCKERFILE`.

- Observation: This repository already treats deploy logic as shared core (`pkg/deployops`) and CLI adapter (`tools/artifactctl`), so system behavior ownership belongs in core packages rather than tooling directories.
  Evidence: `docs/deploy-artifact-contract.md` section "ツール責務（確定）".

- Observation: E2E runner already resolves artifactctl binary path and depends on it for deploy orchestration; shim delegation can reuse this boundary safely.
  Evidence: `e2e/runner/deploy.py` function `_artifactctl_bin`.

## Decision Log

- Decision: Reclassify Maven shim from tooling asset to deploy core asset.
  Rationale: Shim behavior is required for deploy correctness in proxy environments, not optional developer convenience.
  Date/Author: 2026-02-24 / Codex

- Decision: Keep one canonical shim build implementation and make E2E consume it, instead of maintaining a separate Python-specific shim builder path.
  Rationale: Duplicate implementations drift and cause proxy regressions across paths.
  Date/Author: 2026-02-24 / Codex

- Decision: Eliminate current-working-directory assumptions from shim resolution.
  Rationale: `artifactctl` may be executed outside repository root and still must function when artifacts are valid.
  Date/Author: 2026-02-24 / Codex

- Decision: E2E must consume shim ensure through artifactctl adapter command with machine-readable stdout contract.
  Rationale: Shared core behavior is preserved while Python runner remains orchestration-only and deterministic.
  Date/Author: 2026-02-24 / Codex

- Decision: Adapter command contract must be versioned JSON with explicit output mode instead of implicit plain-text.
  Rationale: Prevent parser breakage and allow backward-compatible evolution.
  Date/Author: 2026-02-24 / Codex

- Decision: Shim ensure command must accept proxy configuration via environment variables only, not CLI flags.
  Rationale: Avoid credential exposure through process list and command logs.
  Date/Author: 2026-02-24 / Codex

- Decision: Shim build/push must be guarded by a deterministic per-tag lock.
  Rationale: Prevent duplicate concurrent builds in parallel deploy/E2E executions.
  Date/Author: 2026-02-24 / Codex

- Decision: Remove `tools/maven-shim` in two phases with explicit compatibility period and CI enforcement.
  Rationale: Reduce migration risk for existing scripts while still converging to the new ownership boundary.
  Date/Author: 2026-02-24 / Codex

## Outcomes & Retrospective

This plan is currently at design stage. No implementation outcomes yet. The expected retrospective at completion is that Maven proxy behavior is governed by a single deploy-core mechanism with explicit ownership, no `tools/` hard dependency, and proven behavior in both `artifactctl` and E2E paths.

## Context and Orientation

This repository has two deploy-time places where Maven may run during image builds.

The first is `artifactctl deploy`, implemented through `pkg/deployops/prepare_images.go`. This path rewrites function Dockerfiles, ensures base images, and builds/pushes function images. Maven shim image preparation currently lives in this code path but depends on filesystem discovery of `tools/maven-shim`.

The second is E2E local fixture preparation in `e2e/runner/deploy.py`. When Java fixture image `esb-e2e-image-java` is detected, the runner currently builds a Maven shim image from `tools/maven-shim` and injects it into fixture builds via `MAVEN_IMAGE` build argument.

A "Maven shim image" means a container image where `mvn` is replaced by a wrapper that generates Maven `settings.xml` from proxy environment variables and then invokes real Maven. This avoids mutating `RUN mvn ...` instructions and keeps proxy behavior deterministic.

The architectural problem is ownership and coupling: a deploy-critical system behavior is physically placed under `tools/`, while both Go and Python execution paths rely on hard-coded path assumptions. This plan removes that mismatch.

## Plan of Work

Milestone 1 establishes canonical ownership and repository location. Create a deploy-core asset package under `pkg/deployops/mavenshim/` with subdirectory `assets/` containing `Dockerfile` and `mvn-wrapper.sh`. This location is intentionally under deploy core ownership and is not a developer-tool path. During this milestone, keep `tools/maven-shim` as compatibility mirror with clear deprecation notes, so existing scripts do not break mid-migration. Update documentation (`docs/maven-proxy-contract-v2.md`, `docs/repo-layout-contract.md`) to classify Maven shim as deploy-core asset and document the temporary compatibility window.

Milestone 2 creates a single Maven shim ensure API in Go. Add package-level API such as `pkg/deployops/mavenshim.EnsureImage(...)` that takes base image ref, host registry, proxy env map, and cache flags, then builds/pushes the shim image with deterministic tag derivation. This API is responsible for proxy env validation using `pkg/proxy/maven`, command assembly, and credential-safe diagnostics. Add per-tag locking (file lock or equivalent process-safe lock) so concurrent invocations do not race building the same shim image.

Milestone 3 integrates `artifactctl deploy` path with the shared API. Replace `resolveMavenShimBuildPaths` and any path-walking logic in `pkg/deployops/prepare_images.go` with `mavenshim.EnsureImage(...)`. This removes `os.Getwd()`-based repository discovery from shim behavior.

Milestone 4 integrates E2E path with the same API through CLI adapter extension, but with strict adapter semantics so E2E is deterministic and parse-safe. Add a subcommand to `tools/artifactctl/cmd/artifactctl` (for example `artifactctl internal maven-shim ensure --output json`) that calls the shared ensure API and prints versioned machine-readable payload only on stdout, for example `{ "schema_version": 1, "shim_image": "..." }`. Proxy parameters are inherited from environment variables only; no proxy CLI flags are introduced. Update `e2e/runner/deploy.py` to call this subcommand through the already-resolved artifactctl binary path (`_artifactctl_bin(ctx)`), parse JSON, and use returned `shim_image` in fixture build args. E2E must fail fast on non-zero exit, malformed output, or unsupported artifactctl version; it must not fallback to local filesystem shim build logic. This keeps one behavioral source of truth for shim creation while preserving E2E orchestration ownership.

Milestone 5 removes obsolete paths and adds contract guards in two phases. Phase A keeps `tools/maven-shim` as a compatibility mirror with warnings and CI checks that block new runtime references from core paths. Phase B deletes `tools/maven-shim/` after adapters are migrated and CI confirms no runtime dependency. Extend CI checks so reintroduction of shim assets under `tools/` fails, and ensure docs and tests reference only new ownership path.

## Concrete Steps

Run all commands from `/home/akira/esb3` unless otherwise noted.

1. Add deploy-core shim package scaffold and tests.

    go test ./pkg/deployops -count=1
    go test ./pkg/proxy/maven -count=1

Expected: build passes with shim ensure tests including concurrent-invocation deduplication lock behavior.

2. Add artifactctl adapter command and tests.

    go test ./tools/artifactctl/cmd/artifactctl -count=1

Expected: command parsing/invocation tests pass for `internal maven-shim ensure --output json`, including schema-versioned JSON stdout contract and non-zero failure cases.

2.1 Verify adapter command from non-repository working directory.

    ARTIFACTCTL_BIN="${ARTIFACTCTL_BIN:-artifactctl}"
    tmpdir="$(mktemp -d)"
    (
      cd "$tmpdir"
      "$ARTIFACTCTL_BIN" internal maven-shim ensure \
        --base-image public.ecr.aws/sam/build-java21 \
        --host-registry 127.0.0.1:5010 \
        --output json
    )

Expected: command returns valid JSON with `schema_version` and `shim_image` even when `cwd` is outside repository root.

3. Rewire E2E runner to adapter command.

    X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy \
      uv run pytest e2e/runner/tests/test_deploy_command.py -q

Expected: tests assert artifactctl-mediated shim resolution, JSON parsing, and fail-fast behavior; no direct `tools/maven-shim` path assumptions remain.

4. Run static contract checks.

    bash tools/ci/check_repo_layout.sh
    bash tools/ci/check_maven_proxy_contract.sh

Expected: both checks pass and enforce staged migration rules (no new runtime references to `tools/maven-shim` during compatibility phase).

5. Run proxy-backed acceptance checks.

    uv run python tools/e2e_proxy/run_with_tinyproxy.py -- \
      uv run python e2e/run_tests.py --profile e2e-containerd --build-only --verbose --no-live

    uv run python tools/e2e_proxy/run_with_tinyproxy.py -- \
      artifactctl deploy --artifact <artifact.yml> --out <config_dir> --secret-env <secrets.env>

Expected: Maven-dependent image builds progress through proxy without `Network is unreachable` for Maven repository access.

## Validation and Acceptance

Acceptance is satisfied only when all points are true.

`artifactctl deploy` can execute from a non-repository working directory while still preparing Maven shim image correctly for Maven-based function Dockerfiles. This is verified by a unit/integration test that changes `cwd` before execution and still passes shim preparation.

E2E Java fixture path no longer performs independent shim build logic in Python; it delegates shim ensure to artifactctl adapter command via `_artifactctl_bin(ctx)`, and still passes proxy-backed Java fixture build where `RUN mvn -q -DskipTests package` completes.

Adapter stdout contract is versioned JSON (`schema_version`, `shim_image`) and E2E parser accepts only this contract.

When artifactctl does not support the internal shim command, E2E fails with explicit upgrade guidance rather than silently falling back.

Proxy credentials are passed via environment variables only and never through new CLI flags.

Concurrent shim ensure requests for the same target image do not cause duplicate build/push races.

No runtime code path depends on `tools/maven-shim` after migration Phase B. During Phase A compatibility window, only transitional references are allowed.

Credential safety remains intact: command logs must not expose raw `user:password@proxy` fragments.

## Idempotence and Recovery

All migrations in this plan are additive-first and reversible. During implementation, keep a compatibility window where old and new paths can coexist behind explicit preference for the new path, then remove old path once tests pass. If migration fails midway, restoring previous behavior is possible by reverting the introducing commits because no persisted data format changes are involved.

Shim image tagging must remain deterministic (hash based) so repeated runs do not produce unbounded image churn. Running the same deploy command multiple times should reuse existing shim image unless `--no-cache` is set.

## Artifacts and Notes

The key before/after evidence should include:

- A command transcript proving shim preparation succeeds when `cwd` is not repository root.
- A command transcript showing `artifactctl internal maven-shim ensure ...` returns JSON-only stdout that E2E parser consumes.
- A diff excerpt showing removal of `resolveMavenShimBuildPaths` and `tools/maven-shim` constants.
- E2E log excerpt showing artifactctl-mediated shim resolution and successful Maven package phase.

Keep these snippets concise and focused on behavioral proof.

## Interfaces and Dependencies

Introduce or preserve these interfaces as implementation targets.

In `pkg/deployops/mavenshim`:

- `type EnsureInput struct { BaseImage string; HostRegistry string; NoCache bool; Env map[string]string; Runner CommandRunnerLike }`
- `func EnsureImage(input EnsureInput) (string, error)`
- `func ImageTag(baseImage, hostRegistry string) string`
- `func AcquireLock(tag string) (release func(), err error)` (or equivalent process-safe lock helper) used internally by `EnsureImage`.

`CommandRunnerLike` should match existing deployops command execution shape so `prepare_images.go` can use the same runner abstraction.

In `pkg/deployops/prepare_images.go`:

- Replace direct shim path resolution and build command assembly with `mavenshim.EnsureImage` call.
- Keep Maven-base detection and Dockerfile `FROM` rewrite behavior unchanged except for API integration.

In `tools/artifactctl/cmd/artifactctl/main.go`:

- Add adapter command `internal maven-shim ensure --output json` (or equivalent explicit internal namespace) that maps CLI flags to `mavenshim.EnsureImage` and emits JSON payload `{ "schema_version": 1, "shim_image": "<resolved-ref>" }`.
- Keep stdout contract machine-readable and stable (no prefix/suffix text on stdout). Human hints/errors go to stderr.
- Do not add proxy credential flags. Command must use inherited environment variables.

In `e2e/runner/deploy.py`:

- Replace `_ensure_maven_shim_image` local implementation with adapter invocation to artifactctl command via `_artifactctl_bin(ctx)`.
- Preserve in-process memoization by base image+registry key so the command is called once per unique shim target in a run.
- Keep existing fixture build flow and proxy build-arg propagation unchanged.

Revision note (2026-02-24): Initial ownership-corrective ExecPlan created from architecture review request focusing on system-vs-tools boundary.
Revision note (2026-02-24): E2E invocation model reworked so shim resolution is artifactctl-mediated with strict JSON contract and no local-build fallback.
Revision note (2026-02-24): Completed five-round architecture hardening review (dependency, contract stability, secret handling, concurrency, and migration safety).
