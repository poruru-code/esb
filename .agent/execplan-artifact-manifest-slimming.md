# Remove Artifact Entry ID and Make source_template Optional

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document follows `.agent/PLANS.md` from the repository root. It must be maintained in accordance with that file.

## Purpose / Big Picture

After this change, a manually authored `artifact.yml` can be applied without defining `artifacts[].id`, and `source_template` is treated as optional metadata instead of required deploy input. This removes strict template-coupled validation from artifact apply, reduces code volume in `artifactcore` and `artifactctl`, and aligns behavior with an artifact-first contract where runtime config files under `artifact_root` are the source of truth.

The user-visible proof is that `artifactctl deploy --artifact <artifact.yml> --out <dir>` succeeds for a manifest entry that only contains `artifact_root` and `runtime_config_dir` (plus manifest-level required fields), while existing runtime file/secret/runtime-stack validations still behave the same.

## Progress

- [x] (2026-02-23 15:12Z) Read `.agent/PLANS.md` and mapped required sections/process for this ExecPlan.
- [x] (2026-02-23 15:12Z) Identified all current `id`/`source_template` dependencies in `pkg/artifactcore`, `tools/artifactctl`, tests, docs, and CI export allowlist.
- [x] (2026-02-23 15:19Z) Implemented manifest schema simplification in `pkg/artifactcore`: removed `id`, removed deterministic ID helpers, and made `source_template` optional with conditional validation.
- [x] (2026-02-23 15:20Z) Removed obsolete ID sync logic and CLI surface (`manifest sync-ids`) from `tools/artifactctl`.
- [x] (2026-02-23 15:22Z) Updated tests across `pkg/artifactcore`, `pkg/deployops`, and `tools/artifactctl` for the new contract.
- [x] (2026-02-23 15:24Z) Updated docs and checked-in artifact fixtures to reflect minimal schema without `id`.
- [x] (2026-02-23 15:25Z) Ran focused test suites and confirmed acceptance behavior for Go and Python targets.
- [x] (2026-02-23 15:37Z) Applied strict follow-up validation for explicit empty `source_template.path` / `source_template.sha256`, clarified lowercase-hex docs, and re-ran targeted tests.

## Surprises & Discoveries

- Observation: Current `id` validation is purely a manifest self-consistency check, not artifact payload authenticity.
  Evidence: `pkg/artifactcore/manifest.go` recomputes `id` from `source_template` fields, while merge/apply paths only read runtime config files under `artifact_root`.

- Observation: Removing exported helpers (`ComputeArtifactID`, `SyncArtifactIDs`) requires updating `tools/ci/artifactcore_exports_allowlist.txt` to avoid stale API expectations.
  Evidence: `tools/ci/artifactcore_exports_allowlist.txt` currently lists both symbols.

- Observation: `pytest` collection for `e2e/runner` tests requires environment bootstrap values and `PYTHONPATH`.
  Evidence: initial run failed with `X_API_KEY is required`; rerun succeeded with `PYTHONPATH=. X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy`.

- Observation: Distinguishing “field omitted” from “field provided as empty string” required YAML presence tracking, not plain zero-value checks.
  Evidence: Added `ArtifactSourceTemplate.UnmarshalYAML` flags to detect explicit `path: ""` / `sha256: ""` and reject them.

## Decision Log

- Decision: Remove `artifacts[].id` from the manifest contract instead of keeping it optional.
  Rationale: The requested direction is artifact-first with reduced validation coupling and code volume reduction; optional `id` preserves dead pathways and complexity.
  Date/Author: 2026-02-23 / Codex

- Decision: Keep `source_template` as optional metadata, but validate format only when fields are present.
  Rationale: Preserves manual metadata utility without making apply correctness depend on template provenance.
  Date/Author: 2026-02-23 / Codex

- Decision: Represent `source_template` as a pointer field (`*ArtifactSourceTemplate`) in `ArtifactEntry`.
  Rationale: Distinguishes omission from presence so “validate only when set” can be implemented cleanly with `omitempty`.
  Date/Author: 2026-02-23 / Codex

- Decision: Keep `source_template.path` optional, but reject explicit empty string when provided in YAML.
  Rationale: Matches user requirement (`path: ""` must fail) while preserving optional metadata behavior when the key is omitted.
  Date/Author: 2026-02-23 / Codex

- Decision: Keep `sha256` format strict to 64 lowercase hex characters (uppercase rejected).
  Rationale: User explicitly required no uppercase acceptance; docs were updated to match implementation.
  Date/Author: 2026-02-23 / Codex

## Outcomes & Retrospective

Completed. Artifact manifest validation is now artifact-first: `id` is fully removed from schema, validation, write-path normalization, CLI maintenance commands, tests, and docs. `source_template` is optional metadata with conditional validation only when provided. This reduced code volume by deleting deterministic ID generation/synchronization pathways and their test surface while preserving deploy/runtime checks that matter for apply behavior (`artifact_root`, runtime config paths/files, secrets, runtime stack compatibility).

Remaining gap: broad `go test ./...` cannot run from repo root because workspace configuration excludes that pattern in this environment. Focused package tests that cover changed behavior passed.

Follow-up hardening completed: explicit empty metadata values (`source_template.path: ""`, `source_template.sha256: ""`) now fail validation at manifest read time, with dedicated tests and docs alignment.

## Context and Orientation

The deploy path is split across three areas:

- `pkg/artifactcore`: canonical manifest schema and apply behavior. `ArtifactManifest.Validate()` enforces schema/path/runtime payload requirements; `source_template` is optional metadata validated only when present.
- `pkg/deployops`: orchestration for runtime probing, image preparation, and call into `artifactcore.ExecuteApply`; it depends on manifest validity but not on entry IDs directly.
- `tools/artifactctl/cmd/artifactctl`: CLI adapter exposing `deploy` and `provision` commands.

The runtime apply behavior merges files from each `<artifact_root>/<runtime_config_dir>` entry. Required files are `functions.yml` and `routing.yml`. Optional files include `resources.yml`. Secret and runtime compatibility checks are separate concerns and should remain untouched.

## Plan of Work

First, simplify `pkg/artifactcore/manifest.go` by deleting entry `ID` storage and deterministic ID helper code. Update `ArtifactEntry.Validate` to require only deploy-relevant fields (`artifact_root`, `runtime_config_dir`) and treat `source_template` as optional metadata. Add conditional validation for metadata quality: when `source_template.path` exists it must be non-empty, when `source_template.sha256` exists it must be a 64-character lowercase hex string, and `source_template.parameters` keys must be non-empty if provided.

Second, remove ID synchronization behavior from write/read paths and CLI. In `WriteArtifactManifest`, remove forced ID synchronization. In `tools/artifactctl/cmd/artifactctl/main.go`, delete `manifest sync-ids` command wiring, dependency injection slots, and associated help strings.

Third, update tests to remove deterministic ID assumptions. Replace tests that assert ID recomputation with tests that assert the new optional behavior for `source_template` and unchanged hard failures for missing runtime fields/files. Update deploy and CLI tests so command parsing/dispatch reflects only `deploy` and `provision` surfaces.

Fourth, update user-facing docs and artifact fixture YAML files so minimal schema examples no longer require `id`; preserve mention that `source_template` is optional metadata. Update CI export allowlist to remove deleted exported symbols.

Finally, run targeted Go/Python tests that cover manifest validation, deploy orchestration, and CLI command parsing to prove behavior is intact and codebase compiles/tests cleanly.

## Concrete Steps

Run all commands from `/home/akira/esb3`.

1. Edit core manifest schema and validation:
   - `pkg/artifactcore/manifest.go`
   - `pkg/artifactcore/manifest_test.go`
   - `pkg/artifactcore/fixture_manifest_test.go`
   - `pkg/artifactcore/execute_test.go`
   - `pkg/artifactcore/merge_test.go`

2. Remove obsolete CLI command and sync plumbing:
   - `tools/artifactctl/cmd/artifactctl/main.go`
   - `tools/artifactctl/cmd/artifactctl/main_test.go`

3. Update dependent tests and API export allowlist:
   - `pkg/deployops/prepare_images_test.go`
   - `tools/ci/artifactcore_exports_allowlist.txt`

4. Update docs and fixture manifests:
   - `docs/deploy-artifact-contract.md`
   - `docs/artifact-operations.md`
   - `e2e/artifacts/e2e-docker/artifact.yml`
   - `e2e/artifacts/e2e-containerd/artifact.yml`
   - `e2e/runner/tests/test_deploy_command.py` (fixture example)

5. Validate:
   - `go test ./pkg/artifactcore ./pkg/deployops ./tools/artifactctl/cmd/artifactctl`
   - `pytest -q e2e/runner/tests/test_deploy_command.py`

Expected transcript highlights:

  go test ...
  ok  github.com/poruru-code/esb/pkg/artifactcore ...
  ok  github.com/poruru-code/esb/pkg/deployops ...
  ok  github.com/poruru-code/esb/tools/artifactctl/cmd/artifactctl ...

  pytest ...
  ... passed

## Validation and Acceptance

Acceptance is met when all the following are true:

- A manifest entry without `id` validates and can be applied if runtime config files exist.
- A manifest entry without `source_template` also validates and can be applied.
- If `source_template` is present but malformed (empty `path`, invalid `sha256`, empty parameter key), validation fails with clear field-specific errors.
- `artifactctl` no longer exposes `manifest sync-ids` and still supports `deploy`/`provision`.
- Targeted Go/Python tests pass.

## Idempotence and Recovery

Edits are source-only and idempotent. Re-running format/test commands is safe. If a step fails midway, fix the failing file and rerun only the affected test package before rerunning the full targeted set. No destructive migrations or data rewrites are involved beyond updating checked-in fixture YAML content.

## Artifacts and Notes

Important implementation evidence will be captured as:

- Diff excerpts showing removal of `ID` field and deterministic ID helpers.
- CLI diff removing `manifest sync-ids` command.
- Test output snippets from the targeted Go and Python commands.

## Interfaces and Dependencies

At completion, these public interfaces must exist:

- `pkg/artifactcore.ArtifactEntry` without an `ID` field.
- `pkg/artifactcore.ArtifactSourceTemplate` retained as metadata type, not required for apply.
- `pkg/artifactcore.ArtifactManifest.Validate()` enforcing only deploy-required fields and conditional `source_template` metadata validation.
- `tools/artifactctl` CLI exposing `deploy` and `provision` commands.

Symbols planned for removal from exported API:

- `artifactcore.ComputeArtifactID`
- `artifactcore.SyncArtifactIDs`

Revision note: Initial ExecPlan created to implement user-requested manifest simplification, remove obsolete ID pathways, and reduce code volume while preserving deploy behavior.

Revision note: Updated the living plan after implementation to record completed milestones, added runtime discoveries from test execution, and captured final outcomes for restartability.
