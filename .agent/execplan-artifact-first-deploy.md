# Deploy Artifacts First: 実装同期版 ExecPlan

この文書は `artifact-first deploy` 移行の現状を、実装に同期した状態で管理するための実行計画です。
最終更新: 2026-02-18

## Purpose / Big Picture

主目的は以下です。

1. `esb` CLI 依存を実行経路から外し、成果物 (`artifact.yml` + runtime-config) を正本にする。
2. CLI は「生成/適用を簡単にする補助ツール」に限定し、唯一の実行主体にしない。
3. 将来のリポジトリ分離を可能にする責務境界を固定する。

## Responsibility Boundary (固定)

- Artifact Producer
  - `artifact.yml` と各 artifact root (`runtime-config`, `functions/*/Dockerfile` など) を生成する責務。
  - 実体: `esb artifact generate`（既定 render-only、`--build-images` で build 実行）。

- Artifact Applier
  - `artifact.yml` を検証し、`CONFIG_DIR` へ merge/apply する責務。
  - 実体: `tools/artifactctl` (`validate-id`, `merge`, `prepare-images`, `apply`)。

- Runtime Consumer
  - 反映済み `CONFIG_DIR` を読むだけの責務。
  - 実体: `services/gateway`, `services/provisioner`, `services/agent`, `services/runtime-node`。

## 実装スナップショット（コード正本ベース）

1. CLI 名称は `esb` 固定、repo 外では `version/help` 以外を拒否。
   - `cli/internal/command/app.go`
2. `esb deploy` は内部で Generate/Apply を分離（Generate 全件 -> manifest 出力 -> Apply 1 回）。
   - `cli/internal/command/deploy_entry.go`
   - `cli/internal/usecase/deploy/deploy_run.go`
3. `esb artifact generate` は render-only 既定、`--build-images` 指定時のみ build。
   - `cli/internal/command/artifact.go`
   - `cli/internal/command/app.go`
   - `cli/internal/infra/build/go_builder.go`
4. Apply/merge/ID 検証の判定ロジック正本は Go 実装 (`tools/artifactctl`) に一本化。
   - `tools/artifactctl/cmd/artifactctl/main.go`
   - `tools/artifactctl/pkg/engine/*`
5. E2E 実行経路は artifact-only（`deploy_driver=artifact`, `artifact_generate=none`）。
   - `e2e/runner/config.py`
   - `e2e/runner/deploy.py`
   - `e2e/environments/test_matrix.yaml`
6. E2E fixture はコミット済み成果物を消費し、更新時だけ CLI を使う。
   - `e2e/artifacts/*`
   - `e2e/scripts/regenerate_artifacts.sh`
7. 共有 `meta` モジュールは撤去済み。
   - `meta/` ディレクトリなし

## Phase Status (A-F)

| Phase | 目的 | 状態 | 判定 |
|---|---|---|---|
| A Contract Freeze | `artifact.yml` 単一正本 | 実装済み | Done |
| B Artifact Engine | merge/apply/validate の Go 正本化 | 実装済み | Done |
| C Adapter 分離 | CLI adapter と non-CLI adapter 分離 | 実装済み | Done |
| D Runtime Hardening | フォールバック抑制と hard-fail 化 | 概ね実装済み | Done (要監視) |
| E E2E Gate | artifact-only 回帰ゲートの常設 | docker/containerd は実装済み、firecracker は無効化中 | Partial |
| F Cleanup | 旧 descriptor/冗長経路/未使用コード整理 | 実装済み（継続監視） | Done |

## UX Specification（現行）

- Composite:
  - `esb deploy` = Generate + Apply
- Generate only:
  - `esb artifact generate ...`（render-only）
  - `esb artifact generate --build-images ...`（render + image build）
- Apply only:
  - `esb artifact apply --artifact <artifact.yml> --out <CONFIG_DIR> [--secret-env ...] [--strict]`
- Non-CLI apply path:
  - `tools/artifactctl validate-id`
  - `tools/artifactctl prepare-images`
  - `tools/artifactctl apply`
  - `docker compose --profile deploy run --rm --no-deps provisioner`

## E2E Contract（現行）

- テスト実行時に CLI で generate しない。
- `e2e/artifacts/*/artifact.yml` をそのまま consume する。
- deploy は `artifactctl prepare-images` -> `artifactctl apply` -> provisioner 実行。
- `image_uri_overrides` が local fixture repo を指す場合は、`tools/e2e-lambda-fixtures/*` から fixture image を build/push してから apply する。
- firecracker profile は matrix でコメントアウト中（現時点の対象外）。

## Remaining Gaps / Risks（厳格評価）

1. `prepare-images` は base image build に repo 相対パス (`runtime-hooks/python/docker/Dockerfile`) を使う。
   - 完全な「artifact directory だけで build」を満たしていない。
   - `tools/artifactctl/pkg/engine/prepare_images.go`
2. runtime metadata digest 検証は repo root 推定に依存する。
   - repo root を検出できない環境では non-strict は warning、strict は fail。
   - `tools/artifactctl/pkg/engine/runtime_meta_validation.go`
3. E2E gate は firecracker を含む full matrix まで到達していない。
   - `e2e/environments/test_matrix.yaml`

## Completion Criteria 再判定（今回）

| Criteria | 判定 | 根拠 |
|---|---|---|
| `artifact.yml` 単一正本で apply できる | Pass | `tools/artifactctl/pkg/engine/manifest.go` |
| `esb deploy` が Generate/Apply 分離で動く | Pass | `cli/internal/command/deploy_entry.go` |
| E2E artifact-only で docker/containerd が成立 | Pass | `e2e/runner/config.py`, `e2e/runner/deploy.py` |
| `uv run e2e/run_tests.py --parallel --verbose` を含む full gate（docker/containerd/firecracker） | Fail | firecracker 無効化中 |
| 非 CLI 実行が成果物だけに依存（build 時も repo 非依存） | Fail | `prepare-images` が repo assets 参照 |

現時点の総合判定: **未完了（Partial Complete）**

## Next Work (設計上の残作業)

1. `prepare-images` の base image 依存を artifact contract 側へ明示移管するか、artifact root 内へ閉じる設計に改訂する。
2. firecracker matrix を再有効化する前提条件（環境安定性、時間、CI コスト）を定義し、E2E gate に戻す。
3. runtime metadata strict 検証の前提（repo root 必須か、digest source を artifact 側へ持つか）を契約として固定する。

## Change Log

- 2026-02-18: 実装同期版へ全面更新。旧フェーズ記述（`deploy_driver=cli` 併存、`ensure_local_esb_cli` 前提、古い Current Gaps）を削除し、現行コード基準の完了判定へ切り替え。
