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
5. E2E 実行経路は artifact-only（`deploy_driver` / `artifact_generate` は matrix から撤去済み）。
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
| F Cleanup | 旧 descriptor/冗長経路/未使用コード整理 | 追加是正が必要 | In Progress |

## UX Specification（現行）

- Composite:
  - `esb deploy` = Generate + Apply
- Generate only:
  - `esb artifact generate ...`（render-only、`.esb/staging/**` merge は行わない）
  - `esb artifact generate --build-images ...`（render + image build、`.esb/staging/**` merge は行わない）
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

1. firecracker は開発中のため gate 対象から除外中（再開時に Track C で復帰）。
   - `e2e/environments/test_matrix.yaml`

## Completion Criteria 再判定（今回）

| Criteria | 判定 | 根拠 |
|---|---|---|
| `artifact.yml` 単一正本で apply できる | Pass | `tools/artifactctl/pkg/engine/manifest.go` |
| `esb deploy` が Generate/Apply 分離で動く | Pass | `cli/internal/command/deploy_entry.go` |
| E2E artifact-only で docker/containerd が成立 | Pass | `e2e/runner/config.py`, `e2e/runner/deploy.py`, `e2e/runner/warmup.py` |
| `uv run e2e/run_tests.py --parallel --verbose` で docker/containerd が完走する | Pass | 現行 gate 条件 |
| firecracker を含む full gate（docker/containerd/firecracker） | Deferred | firecracker 開発中のため対象外 |
| 非 CLI 実行が成果物だけに依存（build 時も repo 非依存） | Pass | `tools/artifactctl/pkg/engine/prepare_images.go`（`artifact_root/runtime-base/**` のみ参照） |
| strict runtime metadata 検証が artifact ローカル前提で固定 | Pass | `tools/artifactctl/pkg/engine/runtime_meta_validation.go` |

現時点の総合判定: **主目的は達成。未完了は firecracker 復帰トラックのみ**

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

### Track G: artifactctl UX 見直し（Deferred）

- 背景:
  - 現行の非CLI経路は `prepare-images -> apply -> provisioner -> compose up` の手順理解コストが高い。
- 目的:
  - 運用者が `artifact-first` 契約を維持しつつ、最小コマンドで安全に適用できるUXへ整理する。
- 検討観点:
  - `tools/artifactctl` サブコマンドの統合/ラッパー導入可否（ロジック重複は禁止）。
  - 失敗時メッセージと実行順ガイドの改善。
  - `docker compose up` 単体との差分を明確化するドキュメント導線。
- 受け入れ条件:
  - 初見運用者が docs を見て迷わず apply 完了まで到達できる。
  - `artifactctl` の判定ロジック正本化（Go実装）を崩さない。

### Track D: E2E runtime cleanup（完了）

#### D-1: Template 非依存化（Runner）
- 目的:
  - E2E runtime 実行を `artifact.yml` のみで成立させる。
- 変更対象:
  - `e2e/environments/test_matrix.yaml`
  - `e2e/runner/config.py`
  - `e2e/runner/models.py`
  - `e2e/runner/planner.py`
  - `e2e/runner/context.py`
  - `e2e/runner/tests/*`
- 受け入れ条件:
  - runner が template path を要求せず、artifact manifest のみで deploy phase に到達する。

#### D-2: Java warmup 経路撤去（Runner 実行時）
- 目的:
  - E2E 実行中の Java fixture ビルドを廃止し、fixture 更新時専用フローへ限定する。
- 変更対象:
  - `e2e/runner/warmup.py`
  - `e2e/runner/tests/test_warmup_*`
  - `docs/artifact-operations.md`
- 受け入れ条件:
  - `uv run e2e/run_tests.py ...` 実行中に `docker run ... mvn` が呼ばれない。

#### D-3: Deploy API 残骸除去 + null manifest 統一
- 目的:
  - artifact-only API に合わせて runner/deploy の引数と null 取り扱いを整理する。
- 変更対象:
  - `e2e/runner/deploy.py`
  - `e2e/runner/runner.py`
  - `e2e/runner/tests/test_deploy_command.py`
- 受け入れ条件:
  - `deploy_artifacts` API が未使用引数を持たない。
  - `artifact_manifest: null` / blank は default fallback に統一される。

### Track E: 責務境界補正（generate 時 merge 副作用除去）（完了）

- 背景:
  - `artifact-first` 契約では runtime-config の merge/apply は `artifactctl`（Apply phase）の責務。
  - しかし GoBuilder の generate/build 経路に `.esb/staging/**` への merge 副作用が残っていた。
- 実施内容:
  - `esb deploy` / `esb artifact generate` の generate フェーズを常時 artifact-only に固定。
  - `skipStagingMerge` 拡張ポイント自体を撤去し、bypass 設定経路を削除。
  - `generate/build` 実装から runtime-config merge 呼び出しを除去し、Apply phase（`artifactctl apply`）に一本化。
  - `Workflow.Apply` の staging path 解決から `TemplatePath` 依存を外し、一時 workspace で apply 可能にした。
  - generate フェーズの no-op 残骸（`prepareGenerate`/`prepareBuildPhase` の staging 前提）を整理し、build→summary→apply の実フローに一致させた。
  - 旧 `cli/internal/infra/build/merge_config_*` 実装を撤去し、merge 判定ロジックの重複正本を解消。
  - 期待動作を UT へ反映（generate/build で staging config を生成しないことを検証）。
- 主な変更対象:
  - `cli/internal/command/deploy_entry.go`
  - `cli/internal/infra/build/go_builder_generate_stage.go`
  - `cli/internal/infra/build/go_builder_test.go`
- 受け入れ条件:
  - generate/build フェーズで `.esb/staging/**` へ runtime-config merge が発生しない。
  - apply フェーズは従来どおり `tools/artifactctl` 経由で runtime-config を反映する。

### Track H: image prewarm 分岐撤去と `image-import.json` 廃止（In Progress）

- 背景:
  - `PackageType: Image` 関数は runtime hooks 注入のため Dockerfile 再ビルドが必須であり、`pull/tag/push` prewarm は正本経路ではない。
  - `image-import.json` は prewarm 分岐入力としてのみ残存しており、artifact-first 契約の複雑性要因となっている。
- 実施内容:
  - CLI から `--image-prewarm` を撤去し、deploy/apply から prewarm 分岐と関連コードを削除する。
  - `artifact.yml` の `image_prewarm` を廃止し、manifest 契約を単純化する。
  - `templategen`/`artifactctl merge`/runtime sync から `image-import.json` を撤去する。
  - e2e fixture と docs を新契約に同期する。
- 主な変更対象:
  - `cli/internal/command/app.go`
  - `cli/internal/usecase/deploy/*`
  - `cli/internal/infra/templategen/*`
  - `tools/artifactctl/pkg/engine/*`
  - `e2e/artifacts/*`
  - `docs/deploy-artifact-contract.md`
  - `cli/docs/*`
- 受け入れ条件:
  - `--image-prewarm` が CLI help/実装/UT から消えている。
  - `image-import.json` が生成・merge・runtime sync の経路から消えている。
  - image 関数の配備は `prepare-images` / function Dockerfile build に一本化されている。

### 完了条件（更新）

- 現フェーズ完了条件:
  - `uv run e2e/run_tests.py --parallel --verbose` が docker/containerd で完走
  - CLI なし apply 経路が artifact のみで成立
- 追加残余リスク管理:
  - PR ごとに `e2e/runner/tests` を必須実行
  - Track D 最終PRで full E2E (`uv run e2e/run_tests.py --parallel --verbose`) を必須実行
  - テンプレート非依存性を runner UT で検証（template fixture を直接参照しない）
- firecracker 再開時の追加完了条件（Deferred）:
  - Track C の全ステップが完了
  - `uv run e2e/run_tests.py --parallel --verbose` が docker/containerd/firecracker で完走

## Change Log

- 2026-02-18: 実装同期版へ全面更新。旧フェーズ記述（`deploy_driver=cli` 併存、`ensure_local_esb_cli` 前提、古い Current Gaps）を削除し、現行コード基準の完了判定へ切り替え。
- 2026-02-18: Track A を完了。`prepare-images` の入力契約を `artifact_root/runtime-base/**` に固定し、repo 直参照を撤去。generator/fixture/docs を同時同期した。
- 2026-02-18: Track B を完了。strict runtime metadata 検証の入力源を `artifact_root/runtime-base/**` に固定し、repo root 推定依存を撤去した。
- 2026-02-18: Track C（firecracker gate）を Deferred に変更。現フェーズ完了条件を docker/containerd gate に固定した。
- 2026-02-18: `artifact generate` 軽量化として、artifact adapter 経路では `.esb/staging/**` への merge/stage をスキップする実装へ更新した。
- 2026-02-18: Residual risk を Track D として追加。F Cleanup 判定を In Progress へ戻した。
- 2026-02-18: Track D を完了。runner の template 依存と Java warmup 経路を撤去し、artifact-only 実行契約へ統一した。
- 2026-02-18: Track E を完了。`esb deploy` generate 既定で staging merge を無効化し、GoBuilder generate/build 経路から runtime-config merge 呼び出しを除去。merge/apply 責務を `artifactctl` Apply phase へ再集約した。
- 2026-02-18: Track E を補強。`skipStagingMerge` 拡張ポイントを削除し、generate フェーズを常時 artifact-only へ固定。未使用化した `cli/internal/infra/build/merge_config_*` を撤去して責務境界を明確化した。
- 2026-02-18: Track E を追補。`Workflow.Apply` の template 結合を解除して apply workspace を内部生成へ変更。併せて generate フェーズの no-op 補助層を整理し、実行フローを単純化した。
- 2026-02-18: Track G（Deferred）を追加。`artifactctl` 運用UX（手順複雑性・ガイド導線）の見直しタスクを後続計画へ登録した。
- 2026-02-18: Track H（In Progress）を追加。`--image-prewarm` と `image-import.json` を廃止し、image 関数配備経路を Dockerfile 再ビルドへ一本化する方針を確定した。
