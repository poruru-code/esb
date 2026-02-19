# Artifactcore Refactor: API Surface Simplification and Caller Contract Alignment

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This plan follows `.agent/PLANS.md` and must be maintained accordingly. This plan is a focused follow-up under the artifact-first direction described in `.agent/execplan-artifact-first-deploy.md`.

## Purpose / Big Picture

After this refactor, `pkg/artifactcore` will have a clearer public API and a less fragile call contract from `cli` and `tools/artifactctl`. The package will keep artifact-first behavior unchanged, but remove ambiguous or redundant entrypoints and reduce coupling that currently depends on string-matching and duplicated request shaping.

The user-visible behavior must remain the same for operators: `esb artifact apply` and `artifactctl deploy` keep working, E2E deploy still uses artifact-driven apply, and runtime config merge semantics remain unchanged. The gain is maintainability and safer future changes before further CLI separation.

## Progress

- [x] (2026-02-19 02:10Z) Completed repository-wide inventory of `pkg/artifactcore` public APIs and all call sites from `cli`, `tools/artifactctl`, `e2e`, tests, and docs.
- [x] (2026-02-19 02:16Z) Drafted initial refactor plan (v1) with proposed API simplification and caller alignment.
- [x] (2026-02-19 02:19Z) Review cycle 1 completed; identified plan gaps in boundary clarity, error-contract migration, and rollback detail.
- [x] (2026-02-19 02:23Z) Updated to v2; added explicit non-goals, phased migration details, and file-level acceptance tests.
- [x] (2026-02-19 02:27Z) Review cycle 2 completed; identified gaps in deterministic validation commands and backward-compatibility stance for exported APIs.
- [x] (2026-02-19 02:31Z) Updated to v3; added strict command matrix, explicit breaking-change policy for internal consumers, and success criteria per milestone.
- [x] (2026-02-19 02:35Z) Review cycle 3 completed.
- [x] (2026-02-19 02:42Z) Review cycle 4 completed; found blocking gap in typed-error migration detail.
- [x] (2026-02-19 02:47Z) Updated to v4; added explicit typed-error taxonomy and boundary-check validation command.
- [x] (2026-02-19 02:54Z) Review cycle 5 completed; found blocking gap in `MissingReferencedPathError` emission points.
- [x] (2026-02-19 02:58Z) Updated to v5; specified exact branches/functions for `MissingReferencedPathError`, required `Error()` implementations, and dedicated tests.
- [x] (2026-02-19 03:06Z) Review cycle 6 completed; found blocking gap in explicit error-to-hint mapping and deterministic fallback strings.
- [x] (2026-02-19 03:10Z) Updated to v6; added exact error-to-hint matrix, explicit `Error()` message contracts, and execution prerequisites.
- [x] (2026-02-19 03:16Z) Review cycle 7 completed; no blocking findings remain.
- [x] (2026-02-19 02:36Z) Execute Milestone 1 (safety-net tests + contract snapshots).
- [x] (2026-02-19 02:36Z) Milestone 1 self-review completed; no blocking findings. Added contract snapshots for hint text, apply-request defaults, and missing runtime-config failure path.
- [x] (2026-02-19 02:44Z) Execute Milestone 2 (artifactcore API surface cleanup): removed `MergeRequest` / `MergeRuntimeConfig` from public surface and removed always-true merge helper parameters.
- [x] (2026-02-19 02:44Z) Milestone 2 self-review completed; no blocking findings. Merge behavior remains validated through `Apply` path tests.
- [x] (2026-02-19 02:50Z) Execute Milestone 3 (caller request-shaping alignment): removed `cli` type-alias leakage for apply path and introduced explicit request conversion helper.
- [x] (2026-02-19 02:50Z) Milestone 3 self-review completed; no blocking findings. Runtime provision path now declares defaulted apply fields intentionally.
- [x] (2026-02-19 03:05Z) Execute Milestone 4 (typed error contract migration): added `errors.go`, migrated artifactcore emission points, and replaced `artifactctl` hint routing with `errors.Is`/`errors.As`.
- [x] (2026-02-19 03:05Z) Milestone 4 self-review completed with one corrective fix: runtime-base sentinel now applies only to `os.IsNotExist`; non-notfound stat failures keep native error context.
- [x] (2026-02-19 03:47Z) Execute Milestone 5 (docs sync + full validation): synced docs command example and ran full UT/boundary/Python/E2E validation set.
- [x] (2026-02-19 03:47Z) Milestone 5 self-review completed; no blocking findings remained after full-matrix verification.
- [x] (2026-02-19 11:20Z) Post-review follow-up plan defined for residual contract drift (`ApplyRequest` shaping duplication, runtime-meta constant duplication, dead `ValidateIDs` API).
- [x] (2026-02-19 11:35Z) Post-review follow-up implementation completed and validated with focused Go tests (`pkg/artifactcore`, `cli/internal/{usecase/deploy,command}`, `tools/artifactctl`).
- [x] (2026-02-19 12:05Z) TOCTOU hardening follow-up completed: removed duplicate pre-check path in merge flow and unified missing required-file errors to `MissingReferencedPathError` from merge YAML loaders.
- [x] (2026-02-19 12:05Z) Compatibility concern re-evaluated: call sites remain unified on `NewApplyRequest(...)` and tooling must resolve artifactcore via repo `go.work`.

## Surprises & Discoveries

- Observation: `MergeRuntimeConfig` is exported but not used by production call paths; only tests call it directly.
  Evidence: `rg -n "MergeRuntimeConfig\\("` shows use in `pkg/artifactcore/merge_test.go` and no runtime callers.

- Observation: `cli` has two apply paths with different request shaping behavior.
  Evidence: `cli/internal/command/artifact.go` forwards `SecretEnvPath` and `Strict`, while `cli/internal/usecase/deploy/deploy_runtime_provision.go` forwards only `ArtifactPath` and `OutputDir`.

- Observation: `artifactctl` hint routing depends on string matching against artifactcore error text.
  Evidence: `tools/artifactctl/cmd/artifactctl/main.go` `hintForDeployError` uses `strings.Contains` on message fragments emitted by `pkg/artifactcore/*`.

- Observation: merge helper signatures include a `required bool` parameter that is always passed as `true` by current callers.
  Evidence: `pkg/artifactcore/merge_yaml.go` signatures vs `pkg/artifactcore/merge.go` call sites.

- Observation: helper functions like `asMap`, `asSlice`, and `routeKey` exist in multiple packages with near-identical behavior.
  Evidence: `pkg/artifactcore/values.go`, `pkg/artifactcore/merge_yaml.go`, `cli/internal/usecase/deploy/config_diff.go`.

- Observation: Reviewer feedback showed that typed error migration was not executable without an explicit error taxonomy.
  Evidence: review cycle 4 reported missing sentinel/type names and missing mapping between error category and `artifactctl` hints.

- Observation: Even with taxonomy defined, migration remained blocked until concrete emission points were mapped to existing branches.
  Evidence: review cycle 5 requested exact function/branch targets for `MissingReferencedPathError`.

- Observation: Final blocker was not code-path ambiguity but contract ambiguity between error categories and user-facing hint text.
  Evidence: review cycle 6 requested explicit mapping from each typed error to `artifactctl` hint output and deterministic fallback strings.

- Observation: Final pass surfaced one non-blocking determinism concern for `MissingSecretKeysError` key ordering.
  Evidence: review cycle 7 recommended explicitly sorting keys before rendering `Error()` text.

- Observation: Runtime-base missing sentinel should not capture all stat failures.
  Evidence: Milestone 4 self-review identified that broad wrapping would misclassify permission errors; implementation narrowed sentinel emission to `os.IsNotExist`.

## Decision Log

- Decision: Keep artifact-first runtime behavior unchanged while allowing internal API cleanup in `pkg/artifactcore`.
  Rationale: This work is maintainability-focused; behavior regressions would invalidate current E2E and operational expectations.
  Date/Author: 2026-02-19 / Codex

- Decision: Treat `pkg/artifactcore` exported surface as mutable within this repository, because no external compatibility contract is currently enforced.
  Rationale: User policy is no active external users; minimizing dead public surface now reduces long-term maintenance burden.
  Date/Author: 2026-02-19 / Codex

- Decision: Preserve responsibility boundary `services/*` non-dependence on `pkg/artifactcore`; refactor scope is limited to tooling side (`cli`, `tools/artifactctl`, `e2e`).
  Rationale: This boundary is already guarded by CI policy and must not be weakened.
  Date/Author: 2026-02-19 / Codex

- Decision: Migrate deploy hint logic away from fragile string matching by introducing stable typed/sentinel error signals from artifactcore.
  Rationale: Caller guidance should depend on error categories, not message wording.
  Date/Author: 2026-02-19 / Codex

- Decision: Introduce typed/sentinel errors in a dedicated artifactcore file (`pkg/artifactcore/errors.go`) before changing caller hint logic.
  Rationale: Tests and migration steps need a stable contract to target; without this, Milestone 4 remains ambiguous.
  Date/Author: 2026-02-19 / Codex

- Decision: Specify exact emission points for `MissingReferencedPathError` in merge/runtime-meta/secret-file paths before implementation starts.
  Rationale: Without branch-level mapping, typed hint migration cannot be implemented deterministically.
  Date/Author: 2026-02-19 / Codex

- Decision: Keep `functions.yml` and `routing.yml` separation unchanged in this refactor.
  Rationale: Merge semantics and gateway loaders are split by design; this task targets API/caller cleanup, not runtime config schema redesign.
  Date/Author: 2026-02-19 / Codex

- Decision: Keep typed hint mapping purely category-based in `artifactctl` and remove string matching fallback for covered cases.
  Rationale: This stabilizes operator guidance against message wording changes and enforces one-to-one mapping from error category to recovery hint.
  Date/Author: 2026-02-19 / Codex

## Outcomes & Retrospective

This refactor completed the intended API and caller-contract cleanup while preserving artifact-first runtime behavior.

- API changes:
  - Removed `pkg/artifactcore` public dead surface: `MergeRequest` and `MergeRuntimeConfig`.
  - Removed dead public helper `ValidateIDs`; manifest validation now uses `ReadArtifactManifest` directly.
  - Removed redundant always-true merge helper parameters from `mergeFunctionsYML` / `mergeRoutingYML`.
  - Added typed/sentinel error contract in `pkg/artifactcore/errors.go`.
  - Added `NewApplyRequest(...)` as the single request-shaping constructor for `artifactcore.ApplyRequest`.
  - Centralized runtime metadata contract constants in `pkg/artifactcore` (`RuntimeHooksAPIVersion`, `TemplateRendererName`, `TemplateRendererAPIVersion`).
  - Removed `merge.go` pre-merge required-file pre-checks and made required-file failures originate from merge loaders with typed `MissingReferencedPathError`.

- Caller contract unification:
  - `cli/internal/usecase/deploy` apply path no longer leaks `artifactcore.ApplyRequest` via type alias.
  - `ArtifactApplyRequest -> artifactcore.ApplyRequest` conversion is explicit and test-backed.
  - Runtime provision apply defaults are explicit and tested.
  - `esb artifact apply`, `cli usecase adapter`, and `artifactctl deploy` all use `NewApplyRequest(...)` for single-source request shaping.

- Error hint robustness:
  - `artifactctl` now maps deploy hints by `errors.Is`/`errors.As` against typed categories.
  - Hint behavior no longer depends on fragile free-form string matching for covered categories.

- Validation evidence:
  - `go -C tools/artifactctl test ./... -count=1`: pass
  - `GOWORK=off go -C pkg/artifactcore test ./... -count=1`: pass
  - `go -C cli test ./... -count=1`: pass
  - `go -C services/agent test ./... -count=1`: pass
  - `bash tools/ci/check_tooling_boundaries.sh`: pass
  - `DISABLE_VICTORIALOGS=1 uv run pytest services/gateway/tests -v`: 149 passed
  - `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner -q`: 66 passed
  - `ARTIFACTCTL_BIN=/tmp/artifactctl-deploy-kong uv run e2e/run_tests.py --parallel --verbose`: all matrix entries passed (`e2e-containerd` 45 passed, `e2e-docker` 53 passed)

- Residual risks intentionally deferred:
  - Helper duplication (`asMap`/`asSlice`/`routeKey`) across packages remains for a separate targeted refactor.
  - Error taxonomy currently covers operator-hintable categories defined in this plan; additional categories can be added incrementally as new recoverable flows emerge.

## Context and Orientation

`pkg/artifactcore` is the shared Go core used by both `cli` and `tools/artifactctl`.

Core responsibilities are split across:

- manifest contract and ID logic in `pkg/artifactcore/manifest.go`,
- runtime metadata and secret validation in `pkg/artifactcore/runtime_meta_validation.go` and `pkg/artifactcore/apply.go`,
- runtime-config merge and file locking in `pkg/artifactcore/merge.go` and `pkg/artifactcore/merge_yaml.go`,
- image build/push orchestration in `pkg/artifactcore/prepare_images.go`.

Callers are:

- `cli artifact apply` path: `cli/internal/command/artifact.go`,
- `cli deploy apply phase` path: `cli/internal/usecase/deploy/deploy_runtime_provision.go` via aliases in `cli/internal/usecase/deploy/artifact_manifest.go`,
- `artifactctl deploy` path: `tools/artifactctl/cmd/artifactctl/main.go`,
- e2e indirect caller: `e2e/runner/deploy.py` invoking `artifactctl deploy`.

Current maintainability issues to solve:

1. Exported but effectively dead API (`MergeRuntimeConfig`, `MergeRequest`).
2. Redundant parameters in merge internals (`required bool` always true).
3. Divergent request-shaping between caller paths.
4. Caller hint behavior tightly coupled to free-form error text.
5. Repeated conversion helper logic spread across packages.

Non-goals for this plan:

- Changing runtime config schema (`functions.yml` / `routing.yml` split remains).
- Changing user-facing command flags or command names.
- Moving logic into `services/*` or creating new dependency direction from runtime services to tooling modules.
- Reintroducing `go.mod` local replace coupling between `cli` and `tools/artifactctl`.

Typed error contract introduced by this plan:

1. Add `pkg/artifactcore/errors.go` with these exported error categories.
   - `var ErrRuntimeBaseDockerfileMissing = errors.New("runtime base dockerfile missing")`
   - `var ErrSecretEnvFileRequired = errors.New("secret env file required")`
   - `type MissingSecretKeysError struct { Keys []string }`
   - `type MissingReferencedPathError struct { Path string }`
   - both struct-based errors provide deterministic `Error() string` messages:
     - `MissingSecretKeysError.Error()` sorts `Keys` lexicographically and returns `missing required secret env keys: <k1>, <k2>, ...`
     - `MissingReferencedPathError.Error()` returns `referenced path not found: <path>`

2. Artifactcore wrapping rules.
   - `prepare_images.go` wraps missing runtime-base Dockerfile failures with `ErrRuntimeBaseDockerfileMissing`.
   - `apply.go` wraps the no-secret-env-required failure with `ErrSecretEnvFileRequired`.
   - `apply.go` returns `MissingSecretKeysError` when required keys are missing.
   - `merge.go` `requireFile` `os.IsNotExist` branch returns `MissingReferencedPathError`.
   - `apply.go` `readEnvKeys` open-file `os.IsNotExist` branch returns `MissingReferencedPathError`.
   - `runtime_meta_validation.go` `resolveArtifactFileDigest` strict-mode source-not-found branch returns `MissingReferencedPathError` (wrapped with field context).
   - `manifest.go` missing manifest-file read path wraps with `MissingReferencedPathError` while preserving operation context.

3. Caller hint mapping rules.
   - `tools/artifactctl/cmd/artifactctl/main.go` `hintForDeployError` must use `errors.Is` / `errors.As` first.
   - String matching is temporary fallback only and must be removed or explicitly limited by Milestone 4 acceptance.

4. Error-to-hint matrix (must be implemented and tested exactly).
   - `ErrRuntimeBaseDockerfileMissing`:
     - hint: `run \`esb artifact generate ...\` to stage runtime-base into the artifact before deploy.`
   - `ErrSecretEnvFileRequired`:
     - hint: `set \`--secret-env <path>\` with all required secret keys listed in artifact.yml.`
   - `MissingSecretKeysError`:
     - hint: `set \`--secret-env <path>\` with all required secret keys listed in artifact.yml.`
   - `MissingReferencedPathError`:
     - hint: `confirm \`--artifact\` and referenced files exist and are readable.`
   - fallback (unknown errors):
     - hint: `run \`artifactctl deploy --help\` for required arguments.`

5. Transitional compatibility rule.
   - During migration, wrapped errors keep current human-readable context (for example `deploy failed during image preparation: ...`) while carrying typed categories.
   - String fallback branches in `hintForDeployError` may remain only until all categories above are covered by `errors.Is` / `errors.As` tests.

## Plan of Work

Milestone 1 creates a safety net and contract snapshot before any structural change. Add or extend tests that lock current user-visible behavior for `esb artifact apply`, `cli deploy apply phase`, and `artifactctl deploy`, including error and warning outputs where relevant. This prevents accidental regressions when removing or renaming APIs.

Milestone 2 simplifies artifactcore API surface and internal signatures. Remove exported merge-only entrypoints that have no production caller, and make merge helpers explicit and minimal by deleting always-true parameters. Keep implementation semantics identical by preserving file order, lock behavior, and merge precedence.

Milestone 3 aligns caller request shaping. Replace alias-driven loose coupling in `cli/internal/usecase/deploy/artifact_manifest.go` with explicit conversion helpers so each caller intentionally sets `ApplyRequest` fields. This makes defaults, strictness, and secret-env handling explicit and testable.

Milestone 4 migrates deploy hint coupling from text matching to typed error categories. Artifactcore should return wrapped sentinel/type errors for known recoverable operator actions (for example missing runtime-base staging, missing secret env configuration), and `artifactctl` should route hints using `errors.Is` / `errors.As`.

Milestone 5 synchronizes docs and finalizes validation. Update docs that currently imply stale API or error-contract assumptions, then run full UT and full E2E matrix to prove behavior is unchanged.

## Concrete Steps

All commands run from repository root `/home/akira/esb`.

Prerequisites for deterministic execution:

- `docker` daemon is running and accessible by current user.
- `uv sync --extra dev --frozen` has been run once in repo root.
- required E2E env vars for runner tests are set when noted (`X_API_KEY`, `AUTH_USER`, `AUTH_PASS`).
- current branch has latest changes from target base branch to avoid stale test drift.

1. Baseline and contract snapshot.

    git status --short --branch
    rg -n "MergeRuntimeConfig\\(|type MergeRequest|ApplyRequest|hintForDeployError" pkg cli tools
    go -C tools/artifactctl test ./... -count=1
    go -C services/agent test ./... -count=1
    GOWORK=off go -C pkg/artifactcore test ./... -count=1
    DISABLE_VICTORIALOGS=1 uv run pytest services/gateway/tests -v
    X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner -q

2. Milestone 1 test additions (before refactor).

    - Extend `tools/artifactctl/cmd/artifactctl/main_test.go` to lock hint behavior expectations by error category.
    - Extend `cli/internal/usecase/deploy/deploy_runtime_provision_test.go` to assert current apply-request defaults explicitly.
    - Add focused tests in `pkg/artifactcore` for error categories to be introduced (initially skipped or table-prepared if implementing in next milestone).

3. Milestone 2 artifactcore API cleanup.

    - Edit `pkg/artifactcore/merge.go`: remove exported `MergeRequest` / `MergeRuntimeConfig`; keep internal merge function used by `Apply`.
    - Edit `pkg/artifactcore/merge_yaml.go`: remove `required bool` from `mergeFunctionsYML` and `mergeRoutingYML`; update callers.
    - Update tests in `pkg/artifactcore/merge_test.go` to target behavior through remaining public API.

4. Milestone 3 caller request-shaping alignment.

    - Edit `cli/internal/usecase/deploy/artifact_manifest.go`: replace type alias pass-through with explicit request mapping helpers.
    - Edit `cli/internal/usecase/deploy/deploy_runtime_provision.go`: call mapping helper and set defaults intentionally (no implicit zero-value drift).
    - Edit `cli/internal/command/artifact.go` and tests only if mapping changes expose mismatch.

5. Milestone 4 typed error contract migration.

    - Add `pkg/artifactcore/errors.go` and define the typed/sentinel error categories listed in `Context and Orientation`.
    - Edit `pkg/artifactcore/prepare_images.go` and `pkg/artifactcore/apply.go` to return wrapped sentinel/type errors for operator-hintable failures.
    - Edit `tools/artifactctl/cmd/artifactctl/main.go` `hintForDeployError` to use `errors.Is` / `errors.As` first, with string fallback only during migration.
    - Add tests for typed error mapping in `tools/artifactctl/cmd/artifactctl/main_test.go`.
    - Add artifactcore tests that assert returned error categories for:
      - missing runtime-base Dockerfile,
      - missing required secret env file,
      - missing required secret keys,
      - missing referenced paths (manifest path, required runtime-config file, strict digest source file).

6. Milestone 5 docs sync and full verification.

    - Update docs mentioning old API assumptions: `docs/artifact-operations.md`, `docs/deploy-artifact-contract.md`, `cli/docs/build.md` as needed.
    - Run full validation:

        go -C tools/artifactctl test ./... -count=1
        GOWORK=off go -C pkg/artifactcore test ./... -count=1
        go -C cli test ./... -count=1
        go -C services/agent test ./... -count=1
        bash tools/ci/check_tooling_boundaries.sh
        DISABLE_VICTORIALOGS=1 uv run pytest services/gateway/tests -v
        X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest e2e/runner -q
        ARTIFACTCTL_BIN=/tmp/artifactctl-deploy-kong uv run e2e/run_tests.py --parallel --verbose

## Validation and Acceptance

Acceptance is behavioral and must be demonstrated by tests/logs.

1. No operator-visible regression.
   - `artifactctl deploy --help` and `esb artifact apply --help` still work.
   - E2E deploy flow still succeeds in both `docker` and `containerd` matrices.

2. API simplification is complete.
   - `pkg/artifactcore` no longer exports dead merge-only API.
   - Merge helper signatures do not carry always-true parameters.

3. Caller contracts are explicit.
   - `cli` deploy apply phase and artifact apply path each map to `artifactcore.ApplyRequest` through intentional conversion, not incidental alias leakage.

4. Hint mapping is robust.
   - `artifactctl` hint selection succeeds through typed/sentinel error matching.
   - Changing human-readable artifactcore message text does not break tests for hint routing.
   - Any remaining string fallback in hint mapping is explicitly documented and covered by tests.
   - Each typed category maps to exactly one hint text defined in the Error-to-hint matrix.

5. Boundaries remain intact.
   - No new import path from `services/*` to `pkg/artifactcore` or `tools/*`.
   - Existing tooling boundary checks continue to pass.

## Idempotence and Recovery

These changes are safe to apply incrementally and rerun.

- If Milestone 2 fails, keep Milestone 1 test additions and revert only API-removal commit(s), then rerun package tests.
- If Milestone 4 fails due mixed error contracts, temporarily keep fallback string mapping in `artifactctl` until all artifactcore typed errors are migrated.
- Do not use destructive git reset operations; rollback by targeted commit revert or file-level restore.

## Artifacts and Notes

Implementation evidence to capture in PR:

- before/after symbol diff for `pkg/artifactcore` exported API (`go doc` or `rg "^func [A-Z]"` output),
- focused test outputs for:
  - `tools/artifactctl/cmd/artifactctl/main_test.go`,
  - `pkg/artifactcore/*_test.go`,
  - `cli/internal/usecase/deploy/*_test.go`,
- full E2E pass summary lines for `e2e-containerd` and `e2e-docker`.

## Interfaces and Dependencies

Target public interfaces after refactor:

- Keep:
  - `func Apply(req ApplyRequest) error`
  - `func PrepareImages(req PrepareImagesRequest) error`
  - `func ValidateIDs(path string) error`
  - `func ReadArtifactManifest(path string) (ArtifactManifest, error)`
  - `func WriteArtifactManifest(path string, manifest ArtifactManifest) error`
  - `func ComputeArtifactID(templatePath string, parameters map[string]string, sourceSHA256 string) string`

- Remove from public surface:
  - `type MergeRequest`
  - `func MergeRuntimeConfig(req MergeRequest) error`

Target caller contract:

- `tools/artifactctl` remains a thin adapter over `artifactcore` and must not import `cli`.
- `cli` may import `artifactcore` directly but must use explicit request conversion helpers for apply path wiring.
- `services/*` must remain independent from `artifactcore`.

## Plan Revision Note

Created on 2026-02-19 as a dedicated execution plan for the next `pkg/artifactcore` refactor phase requested after `artifactctl deploy` one-command migration.

Updated on 2026-02-19 through review cycle 4. v4 adds explicit typed-error taxonomy (`pkg/artifactcore/errors.go`), concrete error-to-hint mapping rules, and explicit tooling-boundary validation command so the plan is directly executable.
