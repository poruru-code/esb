<!--
Where: docs/artifact-operations.md
What: Operational guide for artifact-first deploy flows.
Why: Make generate/apply responsibilities and commands explicit for operators.
-->
# Artifact Operations Guide

## Scope
This document defines operational flows for artifact-first deployment.

- Producer responsibility: generate artifacts (`artifact.yml` + runtime-config outputs)
- Applier responsibility: apply generated artifacts to runtime-config volume and run provisioner
- Runtime responsibility: consume prepared runtime-config only
- Payload contract responsibility: verify artifact input integrity (schema/path/runtime payload)

Contract freeze:
- `runtime-base/**` is out of deploy artifact contract scope.
- `artifactctl deploy` may execute image build/pull when needed, but must not use artifact-time `runtime-base/**` as base source.
- lambda base resolution follows deploy-time rules only (artifact creation-time assets are not authoritative):
  - when function Dockerfile build targets exist, lambda base comes from those Dockerfile `FROM` references (registry alias is normalized for build/push path)
  - when no function Dockerfile build targets exist, default ensure target is `<ensure-registry>/esb-lambda-base:latest`
- `artifactctl deploy` must ensure lambda base availability in the target registry even when no function image build targets exist.

The contract details live in `docs/deploy-artifact-contract.md`.

## Phase Model
0. Generate phase: parse templates and render artifact outputs (`artifact.yml`, runtime-config, Dockerfiles)
1. Image build phase: optional operation outside deploy artifact contract
2. Apply phase: validate payload integrity and merge artifact outputs into runtime-config, then provision
3. Runtime phase: run compose services and execute tests/invocations

## Producer Flow (Out of This Repository Scope)
- 生成系ツールは `artifact.yml` と runtime-config を出力します。
- 生成系ツールの操作方法・フラグは本リポジトリでは扱いません。
- 本リポジトリでは apply/runtime 側の契約と実装のみを正本とします。

## Apply Flow
Use `artifactctl` as the canonical apply implementation.

```bash
artifactctl deploy \
  --artifact /path/to/artifact.yml

docker compose up -d
```

Notes:
- `artifactctl deploy` runs payload validation and artifact apply.
- `artifactctl deploy` does not treat `runtime-base/**` as contract input.
- `artifactctl deploy` may run image build/pull, but lambda base selection must follow deploy-time rules only.
- `artifactctl deploy` ensures/pushes lambda base required by deploy-time function builds; when function build targets are absent, it ensures default `esb-lambda-base:latest`.
- ensure-base registry resolution order is `HOST_REGISTRY_ADDR` -> `CONTAINER_REGISTRY` -> `REGISTRY`.
- when lambda-base pull fails during ensure, current implementation falls back to local build from `runtime-hooks/python/docker/Dockerfile`.
- `artifactctl deploy` normalizes deploy-built function image refs from artifact-time local registry aliases (e.g. `127.0.0.1:5010`, `registry:5010`) to the current runtime registry (`CONTAINER_REGISTRY`) before build/push and output generation.
- `artifactctl deploy` must treat `<artifact_root>` as read-only. Temporary build files are created only in ephemeral workspace outside artifact directories.
- `docker compose up` では one-shot `provisioner` が自動実行され、成功後に runtime サービスが起動します。
- 明示的に再provisionしたい場合は `artifactctl provision ...` または `docker compose --profile deploy run --rm provisioner` を使えます。
- merge/apply は `artifactctl` 直実行のみを運用経路とする（shell wrapper は廃止）。

Manual artifact minimum:
- `artifact.yml` with `schema_version/project/env/mode/artifacts[]`
- each entry with `artifact_root/runtime_config_dir` (`source_template` is optional metadata)
- files: `<artifact_root>/<runtime_config_dir>/functions.yml` and `routing.yml`

## Module Contract (artifactcore)
- adapter modules と `tools/artifactctl/go.mod` には `pkg/artifactcore` の `replace` を置かない。
- `services/*` は `tools/*` / `pkg/artifactcore` を直接 import しない。

Boundary ownership map:
- producer adapter owns producer orchestration only: template iteration, output root resolution, source template path/sha extraction.
- `pkg/deployops` owns shared apply orchestration: image prepare and apply execution order.
- `pkg/artifactcore` owns manifest/apply core semantics: required schema/path/runtime payload validation on read/apply.
- producer adapter and `tools/artifactctl` are adapters for `deployops.Execute`; payload correctness logic stays in `pkg/artifactcore`.

## E2E Contract (Current)
`e2e/environments/test_matrix.yaml` is artifact-only:
- legacy driver switches (`deploy_driver`, `artifact_generate`) are no longer allowed
- test execution consumes committed fixtures under `e2e/artifacts/*`
- firecracker profile is currently disabled in matrix (docker/containerd are active gates)
- deploy phases require `artifactctl` on PATH (or `ARTIFACTCTL_BIN` override)
- runtime network defaults は runner の決定論ロジックで算出し、`e2e/contracts/runtime_env_contract.yaml` で整合性を検証する
- `RUNTIME_NET_SUBNET` / `RUNTIME_NODE_IP` は docker モードのみ既定注入し、containerd/firecracker モードでは注入しない（設定しても runtime env では無効化）
- matrix での追加 env 注入は行わず、環境変数は `e2e/environments/*/.env` を唯一の設定点として扱う

## External Orchestrator Contract (ESB-CLI)
- ESB-CLI などの外部オーケストレータは `artifactctl` 実装を package import で吸収できません。実行ファイル呼び出しで連携します。
- deploy/provision 実行前に `artifactctl internal capabilities --output json` を実行し、schema/contracts を照合してください。
- 最低限 required な subcommand は `deploy`, `provision`, `internal fixture-image ensure`, `internal maven-shim ensure`, `internal capabilities` です。
- binary path override は `ARTIFACTCTL_BIN` を使います（解決後の実体パスは runner 内で `ARTIFACTCTL_BIN_RESOLVED` に固定されます）。

Fixture refresh is a separate developer operation (outside E2E runtime):
- regenerate fixtures with `e2e/scripts/regenerate_artifacts.sh`
- this script uses an external artifact producer command and commits raw output
- E2E runner scans generated artifact Dockerfiles and builds/pushes local fixture images from `e2e/fixtures/images/lambda/*` when `FROM` uses local fixture repos

## Failure Policy
- Missing `artifact.yml`, required runtime config files, invalid manifest paths: hard fail
- Presence of legacy matrix fields (`deploy_driver`, `artifact_generate`): hard fail
- Apply phase must not silently fall back to template-based sync paths
- Removed runtime digests: `java_agent_digest`, `java_wrapper_digest`, `template_digest`
