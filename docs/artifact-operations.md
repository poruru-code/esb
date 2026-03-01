<!--
Where: docs/artifact-operations.md
What: Operational guide for artifact-first deploy flows.
Why: Make generate/apply responsibilities and commands explicit for operators.
-->
# Artifact 運用ガイド

## 対象範囲
このドキュメントは、artifact-first デプロイの運用フローを定義します。

- Producer の責務: artifact（`artifact.yml` + runtime-config 出力）を生成する
- Applier の責務: 生成済み artifact を runtime-config volume に適用し、provisioner を実行する
- Runtime の責務: 適用済み runtime-config のみを読み込む
- Payload 契約の責務: artifact 入力の整合性（schema/path/runtime payload）を検証する

契約の固定事項:
- `runtime-base/**` は deploy artifact 契約のスコープ外です。
- `esb-ctl deploy` は必要に応じて image build/pull を実行しますが、artifact 作成時の `runtime-base/**` を base ソースとして使ってはいけません。
- lambda base 解決は deploy 時ルールのみを正とします（artifact 作成時資産は正本ではありません）。
  - 関数 Dockerfile の build target がある場合、lambda base は各 Dockerfile の `FROM` 参照から決定します（build/push 時に registry alias を正規化）。
  - 関数 Dockerfile の build target がない場合、既定 ensure target は `<ensure-registry>/esb-lambda-base:latest` です。
- 関数 image build target が 0 件でも、`esb-ctl deploy` は target registry に lambda base を確保する必要があります。

契約の詳細は `docs/deploy-artifact-contract.md` を参照してください。

## フェーズモデル
0. 生成フェーズ: テンプレートを解析し artifact 出力（`artifact.yml`、runtime-config、Dockerfile）を生成
1. イメージビルドフェーズ: deploy artifact 契約外の任意操作
2. 適用フェーズ: payload 整合性検証と runtime-config へのマージを実施し、その後 provision
3. ランタイムフェーズ: compose サービスを起動し、テスト/呼び出しを実施

## Producer フロー（本リポジトリ管理外）
- 生成ツールは `artifact.yml` と runtime-config を出力します。
- 生成ツールの操作方法やフラグは本リポジトリでは扱いません。
- 本リポジトリでは apply/runtime 側の契約と実装のみを正本とします。

## Apply フロー
apply 実装の正本は `esb-ctl` です。  
`esb-ctl` の具体的な利用手順（コマンド一覧/引数/標準フロー）は `tools/cli/README.md` を正本とします。

```bash
esb-ctl deploy \
  --artifact /path/to/artifact.yml

docker compose up -d
```

補足:
- `esb-ctl deploy` は payload 検証と artifact apply を実行します。
- `esb-ctl deploy` は `runtime-base/**` を契約入力として扱いません。
- `esb-ctl deploy` は image build/pull を実行し得ますが、lambda base の選択は deploy 時ルールに従います。
- 関数 build target がない場合でも、`esb-ctl deploy` は既定 `esb-lambda-base:latest` を ensure/push します。
- ensure-base の registry 解決順は `HOST_REGISTRY_ADDR` -> `CONTAINER_REGISTRY` -> `REGISTRY` です。
- ensure 中に lambda-base の pull が失敗した場合、現在実装は `runtime-hooks/python/docker/Dockerfile` からのローカルビルドへフォールバックします。
- `esb-ctl deploy` は、artifact 時点の local registry alias（例: `127.0.0.1:5010`, `registry:5010`）を、実行時 `CONTAINER_REGISTRY` に正規化して build/push と出力生成を行います。
- `esb-ctl deploy` は `<artifact_root>` を読み取り専用として扱います。一時ファイルは artifact ディレクトリ外の一時ワークスペースにのみ作成されます。
- `docker compose up` では one-shot `provisioner` が自動実行され、成功後に runtime サービスが起動します。
- 明示的に再 provision したい場合は `esb-ctl provision ...` または `docker compose --profile deploy run --rm provisioner` を使えます。
- merge/apply の運用経路は `esb-ctl` 直実行のみです（shell wrapper は廃止）。

手動 artifact の最小要件:
- `schema_version/project/env/mode/artifacts[]` を含む `artifact.yml`
- 各 entry に `artifact_root/runtime_config_dir`（`source_template` は任意メタデータ）
- `<artifact_root>/<runtime_config_dir>/functions.yml` と `routing.yml`

## モジュール契約（`esb-ctl` / `tools/cli`）
- `services/*` は `tools/*` を直接 import しない。
- 外部オーケストレータは package import ではなく、`esb-ctl` 実行ファイル呼び出しで連携する。

境界ごとの責務:
- producer adapter は producer 側オーケストレーションのみを担当（テンプレート反復、出力先解決、source template path/sha 抽出）。
- `tools/cli/deploy_ops.py` は apply オーケストレーションの共有ロジック（image prepare と apply 実行順）を担当。
- `tools/cli/artifact.py` は manifest/apply の中核セマンティクス（schema/path/runtime payload 必須検証）を担当。
- producer adapter は `esb-ctl` CLI adapter として振る舞い、payload correctness logic は `tools/cli` に保持する。

## E2E 契約（現行）
`e2e/environments/test_matrix.yaml` は artifact-only です。
- 旧ドライバ切替 (`deploy_driver`, `artifact_generate`) は使用不可
- テストはコミット済み fixture（`e2e/artifacts/*`）のみを消費
- matrix の firecracker profile は現在無効（docker/containerd が有効ゲート）
- deploy を伴うフェーズでは `esb-ctl` が PATH 上に必要（または `CTL_BIN` 上書き）
- runtime network default は runner の決定論ロジックで算出し、`e2e/contracts/runtime_env_contract.yaml` で検証
- `RUNTIME_NET_SUBNET` / `RUNTIME_NODE_IP` は docker モードのみ既定注入し、containerd/firecracker では注入しない（設定しても runtime env では無効化）
- matrix からの追加 env 注入は行わず、環境変数は `e2e/environments/*/.env` を唯一の設定点とする

## 外部オーケストレータ契約
- 外部オーケストレータは `esb-ctl` 実装を package import で吸収できません。実行ファイル呼び出しで連携します。
- deploy/provision 実行前に `deploy --help` / `provision --help` の実行可否を確認してください。
- 最低限必要な subcommand は `deploy`, `provision` です。
- バイナリ上書きは `CTL_BIN` を使います（解決後実体パスは runner 内で `CTL_BIN_RESOLVED` に固定）。

fixture 更新は E2E runtime 外の開発作業として扱います（本リポジトリ管理外）。
- E2E runner は、生成済み artifact Dockerfile の `FROM` が local fixture repo を使う場合、`e2e/fixtures/images/lambda/*` からローカル fixture image を build/push します。

## 失敗ポリシー
- `artifact.yml` 欠落、必須 runtime config 欠落、manifest path 不正は hard fail
- 旧 matrix フィールド（`deploy_driver`, `artifact_generate`）の存在は hard fail
- apply フェーズは template ベース同期への暗黙フォールバックを禁止
- 廃止済み runtime digest（`java_agent_digest`, `java_wrapper_digest`, `template_digest`）は使用禁止
