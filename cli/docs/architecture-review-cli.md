# CLI Architecture Review (厳しめ評価)

## 目的
初回依頼「`cli`配下のディレクトリ構造・アーキテクチャを、現実的な保守性と責務分離の観点で厳しくレビュー」に対する、
客観評価と修正結果の証跡を 1 ファイルで追跡可能にする。

## 対象と評価基準
- 対象: `cli/internal/**`, `cli/docs/**`, `e2e/runner/tests/**`
- 評価基準:
  - 公開CLI契約（コマンド/フラグ/挙動）を壊していないか
  - レイヤ責務（`command/usecase/domain/infra`）の逆流を防げるか
  - 巨大ファイルの責務混在を実務的な粒度まで縮小できているか
  - UT先行 + 節目E2E運用で退行を検知できるか

## 今回の未解消対象（PR-A0 baseline）
1. `cli/internal/infra/env/env_defaults.go`
2. `cli/internal/infra/build/go_builder.go`
3. `cli/internal/infra/build/go_builder_helpers.go`
4. `cli/internal/usecase/deploy/deploy.go`
5. `cli/internal/infra/sam/template_functions.go`

## 実施ログ（2026-02-11）
### 必須UTゲート
1. `cd cli && go test ./internal/infra/env ./internal/infra/build ./internal/usecase/deploy ./internal/infra/sam ./internal/command ./internal/infra/runtime ./internal/infra/templategen ./internal/architecture`
- 結果: PASS

### CLI契約UT
1. `X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest -q e2e/runner/tests`
- 結果: PASS（23 passed）

### 節目/最終E2E
1. `uv run python e2e/run_tests.py --profile e2e-docker --test-target e2e/scenarios/smoke/test_smoke.py --verbose`
- 結果: PASS（8 passed）
2. `uv run python e2e/run_tests.py --profile e2e-containerd --test-target e2e/scenarios/smoke/test_smoke.py --verbose`
- 結果: PASS（8 passed）

## 総評（現時点）
- 判定: **達成（初回依頼に対する主要未達を解消）**
- 根拠:
  - 既存の未解消項目（`bake/merge_config/deploy_inputs/deploy_template`）に加えて、今回対象の大型5ファイルを分割・再構成。
  - 公開CLI契約（`--template --env --mode --compose-file --no-save-defaults --image-prewarm`）は非変更。
  - UT + runner契約UT + docker/containerd smoke を通過。

## Findings（Severity別、現状ステータス）

### Critical
1. レイヤ依存のガード不在（再発リスク）
- ステータス: **Resolved**
- 対応: `cli/internal/architecture/layering_test.go` で禁止依存を自動検知。

### High
1. `env_defaults.go` の責務集中（branding/proxy/network/registry/configdir 混在）
- ステータス: **Resolved**
- 対応: `env_defaults.go` を entry化し、`env_defaults_*.go` へ責務分割。

2. `GoBuilder` の責務混在（orchestration と補助実装が同居）
- ステータス: **Resolved**
- 対応: `go_builder_generate_stage.go`, `go_builder_base_images.go`, `go_builder_registry_config.go`, `go_builder_functions.go`, `go_builder_ca.go`, `go_builder_lock.go` へ分離。

3. `usecase/deploy.go` のフェーズ混在（registry/build/summary/provision）
- ステータス: **Resolved**
- 対応: `deploy_run.go`, `deploy_registry_wait.go`, `deploy_build_phase.go`, `deploy_postbuild_summary.go`, `deploy_runtime_provision.go` に分割。

4. `sam/template_functions.go` の解析責務混在
- ステータス: **Resolved**
- 対応: `template_functions_parse/serverless/lambda/events_scaling/layers_runtime` に分割。

### Medium
1. `infra/build` の未分割大物（`bake.go`, `merge_config.go`）
- ステータス: **Resolved**
- 対応:
  - `bake.go` を `bake_exec.go`, `bake_hcl.go`, `bake_outputs.go`, `bake_builder.go`, `bake_args.go` に分割。
  - `merge_config.go` を `merge_config_entry.go`, `merge_config_yaml.go`, `merge_config_image_import.go`, `merge_config_lock_io.go`, `merge_config_helpers.go` に分割。

2. `command` 側の未分割大物（`deploy_inputs.go`, `deploy_template_params.go`）
- ステータス: **Resolved**
- 対応:
  - `deploy_inputs.go` を `deploy_inputs_resolve.go`, `deploy_inputs_env_mode.go`, `deploy_inputs_compose.go`, `deploy_inputs_output.go` に分割。
  - `deploy_template_params.go` を `deploy_template_resolve.go`, `deploy_template_discovery.go`, `deploy_template_prompt.go` に分割。

3. `templategen/sam` の未分割大物（`bundle_manifest.go`, `generate.go`, `intrinsics_resolver.go`）
- ステータス: **Resolved**
- 対応:
  - `bundle_manifest.go` を schema/type と helper に分離（`bundle_manifest_types.go`, `bundle_manifest_helpers.go`）。
  - `generate.go` を path/parameter helper に分離（`generate_paths.go`, `generate_params.go`）。
  - `intrinsics_resolver.go` を resolve dispatch / condition / warnings に分離（`intrinsics_resolve_dispatch.go`, `intrinsics_conditions.go`, `intrinsics_warnings.go`）。

4. `infra/deploy` と `templategen/stage` の責務集約（変更影響が広い）
- ステータス: **Resolved**
- 対応:
  - `compose_provisioner.go` を実行本体 / status 判定 / compose file 解決へ分離。
  - `stage.go` を core / layer staging / path 解決へ分離（`stage_layers.go`, `stage_paths.go`）。

### Low
1. `file_ops` の二重実装による将来ドリフト
- ステータス: **Resolved**
- 対応: `cli/internal/infra/fileops/file_ops.go` を新設し、`infra/build` と `infra/templategen` はラッパー経由で共通実装を利用。

2. 依存ルール検証の粗さ（循環依存の説明不足）
- ステータス: **Resolved**
- 対応: `cli/internal/architecture/layering_cycles_test.go` を追加し、`cli/internal/**` の import cycle を明示的に検知。

## 対応前後比較（今回対象5件）
| 項目 | 対応前 | 対応後 |
| --- | ---: | --- |
| `cli/internal/infra/env/env_defaults.go` | 400 LOC | 90 LOC（詳細は `env_defaults_*.go` へ分離） |
| `cli/internal/infra/build/go_builder.go` | 408 LOC | 289 LOC（詳細は `go_builder_*.go` へ分離） |
| `cli/internal/infra/build/go_builder_helpers.go` | 373 LOC | 廃止（`go_builder_*.go` へ再配置） |
| `cli/internal/usecase/deploy/deploy.go` | 376 LOC | 99 LOC（実行フェーズは `deploy_*.go` へ分離） |
| `cli/internal/infra/sam/template_functions.go` | 367 LOC | 廃止（`template_functions_*.go` へ分離） |

## 残課題と次の1手
1. 今後の変更では `layering_test`/`layering_cycles_test` と `e2e/runner/tests` を最優先ゲートとして維持する。
2. 追加の責務は既存分割単位（`*_phase`, `*_resolve`, `*_lock_io`）へ寄せ、巨大ファイル再発を防ぐ。
