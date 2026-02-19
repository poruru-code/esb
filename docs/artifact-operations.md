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

The contract details live in `docs/deploy-artifact-contract.md`.

## Phase Model
1. Generate phase: parse templates and render artifact outputs (`artifact.yml`, runtime-config, Dockerfiles)
2. Image build phase: optional build/push of function images from rendered artifacts
3. Apply phase: validate and merge artifact outputs into `CONFIG_DIR`, then provision
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
  --secret-env /path/to/secrets.env \
  --strict
```

### Composite flow
`esb deploy` is a composite command:
- run generate for all templates (build-only internal phase, image build enabled by deploy semantics)
- write strict `artifact.yml`
- run apply once

## Non-CLI Apply Flow
Use `artifactctl` as the canonical apply implementation.

```bash
artifactctl deploy \
  --artifact /path/to/artifact.yml \
  --out /path/to/config-dir \
  --secret-env /path/to/secrets.env \
  --strict

docker compose --profile deploy run --rm --no-deps provisioner
```

Notes:
- `artifactctl deploy` internally runs image preparation and artifact apply in order.
- `artifactctl deploy` uses `<artifact_root>/runtime-base/**` as the only base-image build context. It does not read repository-local `runtime-hooks/**`.
- merge/apply は `artifactctl` 直実行のみを運用経路とする（shell wrapper は廃止）。

## Module Contract (artifactcore)
- `cli/go.mod` と `tools/artifactctl/go.mod` には `pkg/artifactcore` の `replace` を置かない。
- ローカル開発の依存解決は repo ルート `go.work` のみで行う。
- `services/*` は `tools/*` / `pkg/artifactcore` を直接 import しない。

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
- Missing runtime-base context for required base-image build in `artifactctl deploy` prepare phase: hard fail
- Apply phase must not silently fall back to template-based sync paths
- In `--strict`, runtime digest verification fails if `<artifact_root>/runtime-base/runtime-hooks/python/sitecustomize/site-packages/sitecustomize.py` is missing or unreadable
- Removed runtime digests: `java_agent_digest`, `java_wrapper_digest`, `template_digest`
