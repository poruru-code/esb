# Deploy prewarm 分岐削除と image-import 廃止

この ExecPlan は living document です。`Progress`、`Surprises & Discoveries`、`Decision Log`、`Outcomes & Retrospective` を作業に合わせて更新し続けます。

この計画は `.agent/PLANS.md` の運用ルールに従います。また、artifact-first の全体方針は `.agent/execplan-artifact-first-deploy.md` を正本として参照し、本計画はその中の残課題「deploy の prewarm 複雑性整理」を具体化します。

## Purpose / Big Picture

この変更後、`PackageType: Image` の関数は常に「Dockerfile を使った独自ビルド」で runtime hooks（Python `sitecustomize`、Java `javaagent`）を注入して配備されます。`pull/tag/push` の prewarm 分岐は撤去され、`--image-prewarm` のような無効な選択肢で失敗する経路がなくなります。

利用者が確認できる結果は次の2点です。第一に `esb deploy` と `esb artifact generate` から `--image-prewarm` が消えること。第二に deploy/apply 実行時の設定マージ対象から `image-import.json` がなくなり、設定面は `functions.yml` / `routing.yml` / `resources.yml` に単純化されることです。

## Progress

- [x] (2026-02-18 17:35Z) 現行実装を調査し、`prewarm` 分岐が `deploy_runtime_provision.go` と `image_prewarm.go` に集中し、`image-import.json` はその分岐の入力としてのみ実質利用されていることを確認。
- [x] (2026-02-18 17:35Z) `tools/artifactctl prepare-images` が `functions/*/Dockerfile` を用いて image 関数も再ビルドし、runtime hooks 注入済みイメージを push できることを確認。
- [x] (2026-02-18 17:36Z) 計画レビュー1回目を実施し、親ExecPlan同期、残骸検知、fixture清掃の不足を指摘として確定。
- [x] (2026-02-18 17:37Z) 計画レビュー2回目を実施し、指摘ゼロを確認して実装開始条件を満たした。
- [x] (2026-02-18 17:47Z) prewarm 系フラグ・分岐・コード・UT を削除し、deploy/apply を artifact apply + provision のみに統一。
- [x] (2026-02-18 17:47Z) `image-import.json` の生成・merge・runtime sync を削除し、contract と docs を更新。
- [x] (2026-02-18 17:47Z) CLI/engine/e2e の UT を更新し、関連テストを通した。
- [x] (2026-02-18 17:47Z) 実装後レビューを記録し、計画と実装の整合を確認した。

## Surprises & Discoveries

- Observation: 現行の `runImagePrewarm` は `pull/tag/push` で外部イメージを内部レジストリへそのまま再公開する。
  Evidence: `cli/internal/usecase/deploy/image_prewarm.go` の `docker pull -> docker tag -> docker push`。

- Observation: `templategen` は image 関数でも `functions/<name>/Dockerfile` を必ず生成し、`FROM <ImageSource>` でラップ build する。
  Evidence: `cli/internal/infra/templategen/generate.go` と `cli/internal/infra/templategen/generate_test.go` の `TestGenerateFilesImageFunctionWritesImportManifest`。

- Observation: `prepare-images` は `functions.yml` の `functions.*.image` と Dockerfile 存在を基準に build/push するため、`image-import.json` を読まない。
  Evidence: `tools/artifactctl/pkg/engine/prepare_images.go`。

- Observation: `e2e/artifacts/*` には既存 `config/image-import.json` が含まれており、コード削除だけでは契約残骸が残る。
  Evidence: `e2e/artifacts/e2e-docker/template.e2e/e2e-docker/config/image-import.json`、`e2e/artifacts/e2e-containerd/template.e2e/e2e-containerd/config/image-import.json`。

- Observation: `e2e/runner/tests` を直接実行すると `e2e/conftest.py` が `X_API_KEY` 等の環境変数を要求する。
  Evidence: `RuntimeError: X_API_KEY is required for E2E tests`。`X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy` を付与して再実行し成功。

## Decision Log

- Decision: `--image-prewarm` フラグ（deploy/artifact generate）を完全削除する。
  Rationale: image 関数の正しい経路は hooks 注入を伴う再ビルドのみであり、`off/all` 選択は仕様として無意味かつ誤用余地を増やす。
  Date/Author: 2026-02-18 / Codex

- Decision: `image-import.json` は JSON->YAML 変換せず、機能ごと廃止する。
  Rationale: 利用箇所が prewarm 系のみで、prewarm 廃止後は dead artifact になるため、形式統一より削除が責務分離と単純化に一致する。
  Date/Author: 2026-02-18 / Codex

- Decision: 後方互換（旧フラグ・旧ファイル）を維持しない。
  Rationale: ユーザー前提として既存利用者互換を優先しない方針が明示されているため。
  Date/Author: 2026-02-18 / Codex

- Decision: 本ExecPlanの変更は `.agent/execplan-artifact-first-deploy.md` にも要約同期する。
  Rationale: 本タスクは artifact-first 方針の一部であり、正本計画へ反映しないと次担当者が現状を誤認するため。
  Date/Author: 2026-02-18 / Codex

- Decision: 実装完了判定に `rg` による legacy 参照ゼロ確認を追加する。
  Rationale: prewarm/image-import は横断的に散在しており、コンパイル成功だけでは残骸を取りこぼすため。
  Date/Author: 2026-02-18 / Codex

- Decision: 計画レビュー2回目で追加指摘がなかったため、本計画を実装ベースラインとして固定する。
  Rationale: これ以上の計画精緻化より実装着手の方がリスク低減に有効であり、検証手順も定義済みのため。
  Date/Author: 2026-02-18 / Codex

## Outcomes & Retrospective

目的としていた「prewarm 分岐削除」と「image-import 廃止」は完了しました。CLI 契約、usecase、templategen、artifactctl、e2e fixture、関連 docs を一貫して更新し、`PackageType: Image` の配備経路を Dockerfile 再ビルド（hooks 注入）に一本化できています。

残課題は本計画内にはありません。後続としては `.agent/execplan-artifact-first-deploy.md` の Track G（artifactctl UX 見直し）が別軸で残りますが、今回の削除により UX 改善時の設計面ノイズは減りました。

## Context and Orientation

このリポジトリでは artifact-first deploy を採用しています。意味は「apply の正本は `artifact.yml` と artifact root 配下の生成物であり、CLI は生成/適用の薄いアダプタに留める」です。

現状で問題を起こしている経路は次です。

`cli/internal/usecase/deploy/deploy_runtime_provision.go` は apply 後に `image-import.json` を読み、`ImagePrewarm` 値に応じて `runImagePrewarm` を呼びます。`runImagePrewarm` 本体は `cli/internal/usecase/deploy/image_prewarm.go` にあり、外部 image をそのまま `pull/tag/push` します。

一方で build 側は `cli/internal/infra/templategen/generate.go` が image 関数の Dockerfile を生成し、`tools/artifactctl/pkg/engine/prepare_images.go` がその Dockerfile から function image を再ビルドして push します。この build 経路だけが runtime hooks 注入を保証します。

`image-import.json` は現在 `templategen` が生成し、`tools/artifactctl/pkg/engine/merge.go` が merge し、`cli/internal/usecase/deploy/runtime_config.go` が runtime config として同期します。しかし runtime consumer がこのファイルを直接利用する経路はなく、実質 prewarm 系のためだけに残っています。

## Plan of Work

最初に CLI 契約を整理します。`cli/internal/command/app.go` から `--image-prewarm` を削除し、`DeployCmd` と `ArtifactGenerateCmd`、および `commandFlagExpectsValue` を同期させます。続いて `cli/internal/command/deploy_entry.go` と `cli/internal/command/artifact.go` の引数受け渡しから `ImagePrewarm` を除去し、artifact manifest 生成 (`cli/internal/command/deploy_artifact_manifest.go`) から `image_prewarm` 書き込みを削除します。

次に usecase 層を簡素化します。`cli/internal/usecase/deploy/deploy.go` から `Request.ImagePrewarm` を削除し、`deploy_run.go` の normalize と apply フェーズ呼び出しを prewarm 非依存に変更します。`deploy_runtime_provision.go` では `image-import.json` 読み込み・必須判定・prewarm 実行を削除して、`artifact apply -> runtime config sync -> provisioner` のみにします。`image_prewarm.go` と専用テストは削除します。

その後に generator/engine を整理します。`cli/internal/infra/templategen/generate.go` から image import 解決と `image-import.json` 出力を除去し、`cli/internal/infra/templategen/image_import.go` と関連テストを削除します。`tools/artifactctl/pkg/engine/merge.go` から image import merge 呼び出しを削除し、`merge_image_import.go` と `merge_io.go` 内の JSON 専用ロジックを撤去します。`cli/internal/usecase/deploy/runtime_config.go` の同期対象ファイルから `image-import.json` を外します。

最後に契約と検証を揃えます。`docs/deploy-artifact-contract.md`、`cli/docs/architecture.md`、`cli/docs/container-management.md`、`cli/docs/generator-architecture.md`、`docs/container-runtime-operations.md` から prewarm/image-import 前提を削除し、`prepare-images` 経路を唯一の関数イメージ準備手順として記述します。`e2e` の matrix 正規化で未使用 `image_prewarm` を除去し、fixture manifest の `image_prewarm` と fixture config の `image-import.json` を削除して新契約に揃えます。加えて `.agent/execplan-artifact-first-deploy.md` に Track 更新を反映します。

## Concrete Steps

作業ディレクトリは `/home/akira/esb` を前提にします。

1. 影響範囲の編集。
   - `cli/internal/command/*`
   - `cli/internal/usecase/deploy/*`
   - `cli/internal/infra/templategen/*`
   - `tools/artifactctl/pkg/engine/*`
   - `e2e/runner/*`
   - `docs/*` と `cli/docs/*`

2. 削除後のコンパイル確認。

      cd /home/akira/esb
      go test ./cli/internal/command ./cli/internal/usecase/deploy ./cli/internal/infra/templategen ./tools/artifactctl/pkg/engine -count=1

3. E2E runner の契約テスト確認。

      cd /home/akira/esb
      uv run pytest -q e2e/runner/tests/test_config.py e2e/runner/tests/test_deploy_command.py

4. 可能なら CLI 全体回帰。

      cd /home/akira/esb/cli
      go test ./... -count=1

5. legacy 残骸のゼロ確認。

      cd /home/akira/esb
      rg -n "image-prewarm|image_prewarm|image-import\\.json|runImagePrewarm|NormalizeImagePrewarmMode" cli tools e2e docs

## Validation and Acceptance

受け入れ条件は次です。

`esb deploy --help` と `esb artifact generate --help` に `--image-prewarm` が表示されないこと。

`artifact.yml` の entry に `image_prewarm` が出力されないこと。

`<artifact_root>/config` に `image-import.json` が生成されないこと。

`tools/artifactctl merge/apply` 実行後の `CONFIG_DIR` に `image-import.json` が存在しないこと。

`prepare-images` は image 関数を含む artifact で成功し、従来どおり function image を build/push できること。

関連 UT が成功し、削除対象コードに対する参照エラーがないこと。

`.agent/execplan-artifact-first-deploy.md` に今回の設計確定内容が追記されていること。

## Idempotence and Recovery

この変更は基本的に削除中心であり、再実行可能です。テストが失敗した場合は失敗箇所の契約（特に manifest の期待値と help 文言）を更新して再実行します。破壊的な外部操作は行わず、ローカルファイル編集とテスト実行のみを対象にします。

## Artifacts and Notes

主要な検証結果:

  cd /home/akira/esb
  go -C cli test ./internal/command ./internal/usecase/deploy ./internal/infra/templategen -count=1
  ok  	github.com/poruru/edge-serverless-box/cli/internal/command
  ok  	github.com/poruru/edge-serverless-box/cli/internal/usecase/deploy
  ok  	github.com/poruru/edge-serverless-box/cli/internal/infra/templategen

  cd /home/akira/esb
  go -C tools/artifactctl test ./pkg/engine -count=1
  ok  	github.com/poruru/edge-serverless-box/tools/artifactctl/pkg/engine

  cd /home/akira/esb
  X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest -q e2e/runner/tests/test_config.py e2e/runner/tests/test_deploy_command.py
  17 passed in 0.12s

  cd /home/akira/esb
  go -C cli test ./... -count=1
  go -C tools/artifactctl test ./... -count=1
  (all pass)

  cd /home/akira/esb
  rg -n "image-prewarm|image_prewarm|image-import\\.json|runImagePrewarm|NormalizeImagePrewarmMode" cli tools e2e docs --glob '!**/*_test.go'
  (no matches)

## Interfaces and Dependencies

最終的に次のインターフェースを満たす必要があります。

`cli/internal/usecase/deploy.Request` は `ImagePrewarm` フィールドを持たない。

`tools/artifactctl/pkg/engine.ArtifactEntry` は `ImagePrewarm` フィールドを持たない。

`cli/internal/usecase/deploy.Workflow.runRuntimeProvisionPhase` は prewarm 引数を取らない。

`cli/internal/infra/templategen.GenerateFiles` は `config/image-import.json` を出力しない。

`tools/artifactctl/pkg/engine.mergeOneRuntimeConfig` は `functions.yml`、`routing.yml`、`resources.yml` のみを merge 対象にする。

計画改訂履歴:
- 2026-02-18: 初版作成。prewarm 廃止と image-import 廃止を同時に進める方針を明文化。
- 2026-02-18: レビュー1回目の指摘を反映。親ExecPlan同期、legacy参照ゼロ確認、fixture内 `image-import.json` 削除を追加。
- 2026-02-18: レビュー2回目で指摘ゼロを確認し、実装ベースラインとして凍結。
- 2026-02-18: 実装完了。関連テスト結果と最終レビュー結果を反映。
