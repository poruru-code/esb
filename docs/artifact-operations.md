<!--
Where: docs/artifact-operations.md
What: Operational guide for artifact-first deploy flows with and without CLI.
Why: Make generate/apply responsibilities and commands explicit for operators.
-->
# Artifact Operations Guide

## Scope
This document defines operational flows for artifact-first deployment.

- Producer responsibility: generate artifacts (`artifact.yml` + runtime-config outputs)
- Applier responsibility: apply generated artifacts to `CONFIG_DIR` and run provisioner
- Runtime responsibility: consume prepared runtime-config only
- Payload contract responsibility: verify artifact input integrity (schema/path/id/runtime payload)
- Runtime stack compatibility responsibility: verify live stack version/capability compatibility at deploy time

Contract freeze:
- `runtime-base/**` is out of deploy artifact contract scope.
- `artifactctl deploy` may execute image build/pull when needed, but must not use artifact-time `runtime-base/**` as base source.
- lambda base selection follows current runtime environment (registry/tag/stack), not artifact creation-time assets.

The contract details live in `docs/deploy-artifact-contract.md`.

## Phase Model
0. Runtime compatibility phase: validate compatibility against live gateway/agent/provisioner/runtime-node before apply (phased implementation)
1. Generate phase: parse templates and render artifact outputs (`artifact.yml`, runtime-config, Dockerfiles)
2. Image build phase: optional operation outside deploy artifact contract
3. Apply phase: validate payload integrity and merge artifact outputs into `CONFIG_DIR`, then provision
4. Runtime phase: run compose services and execute tests/invocations

## CLI Flow
### Generate only
Default is render-only (`--build-images` is false by default):

```bash
esb artifact generate \
  --template e2e/fixtures/template.e2e.yaml \
  --env dev \
  --mode docker \
  --no-save-defaults
```

Notes:
- `esb artifact generate` does not merge outputs into `.esb/staging/**`.
- Generate writes template outputs + `artifact.yml`; apply responsibility is `artifact apply` (or `artifactctl deploy`).

### Generate with image build
```bash
esb artifact generate \
  --template e2e/fixtures/template.e2e.yaml \
  --env dev \
  --mode docker \
  --build-images \
  --no-save-defaults
```

### Apply only
```bash
esb artifact apply \
  --artifact .esb/artifacts/<project>/<env>/artifact.yml \
  --out /path/to/config-dir \
  --secret-env /path/to/secrets.env
```

### Composite flow
`esb deploy` is a composite command:
- run generate for all templates
- write `artifact.yml`
- run apply once

Notes:
- `esb deploy` / `esb artifact generate` が生成する `artifact.yml` には `runtime_stack` が既定で含まれます。

Note:
- image build/pull may happen in deploy operations, but base image selection must follow current runtime environment.

## Non-CLI Apply Flow
Use `artifactctl` as the canonical apply implementation.

```bash
artifactctl deploy \
  --artifact /path/to/artifact.yml \
  --out /path/to/config-dir \
  --secret-env /path/to/secrets.env

docker compose --profile deploy run --rm --no-deps provisioner
```

Notes:
- `artifactctl deploy` runs payload/runtime compatibility validation and artifact apply.
- `artifactctl deploy` does not treat `runtime-base/**` as contract input.
- `artifactctl deploy` may run image build/pull, but lambda base must be resolved from current runtime environment.
- `runtime_stack` requirement validation exists in shared core; `artifactctl deploy` preflight performs runtime observation probe before apply.
- `esb deploy` 経路でも runtime observation を apply 前に取得して `artifactcore` へ渡す。
- `artifactctl deploy` must treat `<artifact_root>` as read-only. Temporary build files are created only in ephemeral workspace outside artifact directories.
- merge/apply は `artifactctl` 直実行のみを運用経路とする（shell wrapper は廃止）。

Manual artifact minimum:
- `artifact.yml` with `schema_version/project/env/mode/artifacts[]`
- each entry with `id/artifact_root/runtime_config_dir/source_template.path`
- files: `<artifact_root>/<runtime_config_dir>/functions.yml` and `routing.yml`
- manual ID sync helper:
```bash
artifactctl manifest sync-ids --artifact /path/to/artifact.yml
artifactctl manifest sync-ids --artifact /path/to/artifact.yml --check
```

## Module Contract (artifactcore)
- `cli/go.mod` と `tools/artifactctl/go.mod` には `pkg/artifactcore` の `replace` を置かない。
- ローカル開発の依存解決は repo ルート `go.work` のみで行う。
- `services/*` は `tools/*` / `pkg/artifactcore` を直接 import しない。

Boundary ownership map:
- `cli` owns producer orchestration only: template iteration, output root resolution, source template path/sha extraction.
- `pkg/artifactcore` owns manifest contract semantics: deterministic artifact ID normalization on write and required ID/schema/path validation on read/apply.
- `cli` and `tools/artifactctl` are adapters for `artifactcore.ExecuteApply`; apply correctness logic must stay in `pkg/artifactcore`.

## E2E Contract (Current)
`e2e/environments/test_matrix.yaml` is artifact-only:
- legacy driver switches (`deploy_driver`, `artifact_generate`) are no longer allowed
- `config_dir` is mandatory per environment; runner does not calculate staging paths implicitly
- test execution consumes committed fixtures under `e2e/artifacts/*`
- firecracker profile is currently disabled in matrix (docker/containerd are active gates)
- deploy phases require `artifactctl` on PATH (or `ARTIFACTCTL_BIN` override)
- runtime network defaults (`SUBNET_EXTERNAL`, `RUNTIME_NET_SUBNET`, `RUNTIME_NODE_IP`, `LAMBDA_NETWORK`) は `e2e/contracts/runtime_env_contract.yaml` を正本として runner が注入する
- matrix `env_vars` では上記 runtime network keys を上書きしない（契約値との二重管理を禁止）

Fixture refresh is a separate developer operation (outside E2E runtime):
- regenerate fixtures with `e2e/scripts/regenerate_artifacts.sh`
- this script uses `esb artifact generate` and commits raw output
- E2E runner may build/push local fixture images from `tools/e2e-lambda-fixtures/*` when `image_uri_overrides` points to local fixture repos

## Failure Policy
- Missing `artifact.yml`, required runtime config files, invalid ID, missing required secrets: hard fail
- Presence of legacy matrix fields (`deploy_driver`, `artifact_generate`): hard fail
- Apply phase must not silently fall back to template-based sync paths
- Runtime stack compatibility major mismatch is hard fail (when compatibility preflight is enabled)
- Removed runtime digests: `java_agent_digest`, `java_wrapper_digest`, `template_digest`
