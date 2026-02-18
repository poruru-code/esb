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
| E E2E Gate | artifact-only 回帰ゲートの常設 | docker/containerd は実装済み、firecracker は開発中のため保留 | Done (Scope-limited) |
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
- firecracker profile は matrix でコメントアウト中（開発中のため現時点では対象外）。

## Remaining Gaps / Risks（厳格評価）

1. 現フェーズの主目的（CLI依存排除 / artifact-only適用）は達成済み。
2. firecracker は開発中のため gate 対象から除外中（再開時に別トラックで復帰）。
   - `e2e/environments/test_matrix.yaml`

## Completion Criteria 再判定（今回）

| Criteria | 判定 | 根拠 |
|---|---|---|
| `artifact.yml` 単一正本で apply できる | Pass | `tools/artifactctl/pkg/engine/manifest.go` |
| `esb deploy` が Generate/Apply 分離で動く | Pass | `cli/internal/command/deploy_entry.go` |
| E2E artifact-only で docker/containerd が成立 | Pass | `e2e/runner/config.py`, `e2e/runner/deploy.py` |
| `uv run e2e/run_tests.py --parallel --verbose` で docker/containerd が完走する | Pass | 現行 gate 条件 |
| firecracker を含む full gate（docker/containerd/firecracker） | Deferred | firecracker 開発中のため対象外 |
| 非 CLI 実行が成果物だけに依存（build 時も repo 非依存） | Pass | `tools/artifactctl/pkg/engine/prepare_images.go`（`artifact_root/runtime-base/**` のみ参照） |
| strict runtime metadata 検証が artifact ローカル前提で固定 | Pass | `tools/artifactctl/pkg/engine/runtime_meta_validation.go` |

現時点の総合判定: **主目的は完了（firecracker gate は Deferred）**

## Next Work (未完了対応の分割計画)

以下は **PR分割可能な最小単位** での実行順です。各ステップは独立レビュー可能な粒度に固定します。

### Track A: `prepare-images` の artifact-only 化（完了）

- 実施内容:
  - 契約を `artifact_root/runtime-base/**` 入力へ固定した（docs 更新）。
  - `artifact generate` で runtime-base build context を成果物へステージするよう変更した。
  - `prepare-images` が repo 直参照を使わず、artifact 内の runtime-base だけで base image build するよう変更した。
  - E2E fixture を再生成し、runtime-base を含む raw output に同期した。
- 主な変更対象:
  - `cli/internal/infra/templategen/generate.go`
  - `cli/internal/infra/templategen/stage_runtime_base.go`
  - `tools/artifactctl/pkg/engine/prepare_images.go`
  - `e2e/artifacts/*`
  - `e2e/scripts/regenerate_artifacts.sh`
  - `docs/deploy-artifact-contract.md`
  - `docs/artifact-operations.md`

### Track B: runtime metadata strict 検証の前提固定（完了）

- 実施内容:
  - strict/non-strict の digest 検証入力源を `artifact_root/runtime-base/**` に固定した。
  - `runtime_meta` 検証から repo root 推定依存を撤去した。
  - strict/non-strict の失敗条件を UT と docs で同期した。
- 主な変更対象:
  - `tools/artifactctl/pkg/engine/runtime_meta_validation.go`
  - `tools/artifactctl/pkg/engine/runtime_meta_validation_test.go`
  - `docs/deploy-artifact-contract.md`
  - `docs/artifact-operations.md`

### Track C: firecracker を含む E2E gate 復帰（Deferred）

#### C-1: firecracker 再有効化の前提整備（Prep PR）
- 目的:
  - firecracker profile を matrix に戻す前に、環境前提・所要時間・失敗時の切り分けを定義する。
- 変更対象:
  - `docs/e2e-runtime-smoke.md`（または運用 docs）
  - `e2e/environments/test_matrix.yaml`（必要ならコメント整備）
- 受け入れ条件:
  - CI/ローカルで firecracker 実行可否の判定条件が明文化されている。

#### C-2: firecracker matrix 復帰（Gate PR）
- 目的:
  - `e2e-firecracker` を artifact-only gate に戻す。
- 変更対象:
  - `e2e/environments/test_matrix.yaml`
  - 必要な runner 調整
- 受け入れ条件:
  - `uv run e2e/run_tests.py --parallel --verbose` が docker/containerd/firecracker を完走する。

### 実行順（依存関係）

1. C-1（firecracker 開発再開後）
2. C-2（C-1 完了後）

### 完了条件（更新）

- 現フェーズ完了条件:
  - `uv run e2e/run_tests.py --parallel --verbose` が docker/containerd で完走
  - CLI なし apply 経路が artifact のみで成立
- firecracker 再開時の追加完了条件（Deferred）:
  - Track C の全ステップが完了
  - `uv run e2e/run_tests.py --parallel --verbose` が docker/containerd/firecracker で完走

## Change Log

- 2026-02-18: 実装同期版へ全面更新。旧フェーズ記述（`deploy_driver=cli` 併存、`ensure_local_esb_cli` 前提、古い Current Gaps）を削除し、現行コード基準の完了判定へ切り替え。
- 2026-02-18: Track A を完了。`prepare-images` の入力契約を `artifact_root/runtime-base/**` に固定し、repo 直参照を撤去。generator/fixture/docs を同時同期した。
- 2026-02-18: Track B を完了。strict runtime metadata 検証の入力源を `artifact_root/runtime-base/**` に固定し、repo root 推定依存を撤去した。
- 2026-02-18: Track C（firecracker gate）を Deferred に変更。現フェーズ完了条件を docker/containerd gate に固定した。
