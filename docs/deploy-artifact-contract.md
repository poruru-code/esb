<!--
Where: docs/deploy-artifact-contract.md
What: Contract for deploy artifacts that can be consumed without in-repo producer tooling.
Why: Define a stable boundary between artifact producer (external/manual) and runtime consumer.
-->
# Deploy Artifact Contract

## 目的
この契約は、生成済み成果物だけで compose 起動・更新を行えるようにするためのものです。
この契約は「生成手段」と「適用手段」を分離し、生成ツールの実装に依存しません。

## 設計原則（Single Manifest）
- 適用対象の正本は `artifact.yml` のみです。
- 単一/複数テンプレートは `artifact.yml` の `artifacts[]` で表現します。
- 複数テンプレート時の deploy 順と merge 順は `artifacts[]` 配列順を唯一の真実にします。
- `.esb` 探索や `ARTIFACT_ROOTS` 手動列挙は非推奨ではなく禁止とします。

## 契約固定ルール（ブレ防止）
- `runtime-base/**` は Deploy Artifact Contract の対象外です。
- `artifactctl deploy` は artifact 生成を行いません。必要な image build は許可しますが、artifact 作成時の `runtime-base/**` を base ソースとして使用しません。
- `artifactctl deploy` が参照する lambda base は、実行時環境（現在の registry/tag/stack）を正とします。
- `artifactctl deploy` は関数イメージの build 対象有無に関係なく、deploy 前提条件として lambda base を target registry へ確保する必要があります。
- `artifactctl deploy` は deploy で build/push する function image の local registry alias（例: `127.0.0.1:5010`, `registry:5010`）を deploy 実行時の `CONTAINER_REGISTRY` に正規化して扱います。
- 互換性判定は artifact 内ファイルではなく、実行時スタック観測結果に基づいて行います。

## 用語
- Artifact Manifest: 適用の正本となる `artifact.yml`
- Artifact Entry: `artifacts[]` の各要素（テンプレート単位の成果物情報）
- Artifact Root: 各 entry が指す成果物ディレクトリ
- Runtime Config: `runtime-config/` 配下の実行設定

## レイアウト（v1）
```text
<artifact-manifest-dir>/
  artifact.yml
  compose.env                  # 非機密のみ
  compose.secrets.env.example  # キー名テンプレートのみ

<artifact-root-a>/
  runtime-config/
    functions.yml
    routing.yml
    resources.yml              # 条件付き

<artifact-root-b>/
  ...
```

## 典型的な出力先
- 生成ツールは `artifact.yml` を次へ出力します。
  `<repo_root>/.esb/artifacts/<project>/<env>/artifact.yml`
- `<project>` と `<env>` は path segment 化して保存します（`/` と `\` は `-` へ置換）。
- 各 entry の `artifact_root` は、既定では `artifact.yml` からの relative path で出力します。

## 必須 / 条件付き必須
### Manifest 必須
- `artifact.yml`
- `schema_version`
- `project`
- `env`
- `mode`
- `artifacts[]`（1件以上）

### Entry 必須
- `artifact_root`
- `runtime_config_dir`
- `<artifact_root>/<runtime_config_dir>/functions.yml`
- `<artifact_root>/<runtime_config_dir>/routing.yml`

### Entry 条件付き必須
- `<artifact_root>/<runtime_config_dir>/resources.yml`: resource 定義を使う場合

## 手動作成で成立する最小必須セット（固定）
このセクションは「producer を使わず、手動で artifact を作る」場合の最小要件です。
`artifactctl deploy` で成立させるため、以下だけを必須とします。

- Manifest 必須:
  - `schema_version`
  - `project`
  - `env`
  - `mode`
  - `artifacts[]`（1件以上）
- Entry 必須:
  - `artifact_root`
  - `runtime_config_dir`
- ファイル必須:
  - `<artifact_root>/<runtime_config_dir>/functions.yml`
  - `<artifact_root>/<runtime_config_dir>/routing.yml`

最小セットでは以下は任意です（必要時のみ追加）:
- `resources.yml`
- `source_template`
- `generated_at` / `generator`

注意:
- `source_template` は deploy 適用の正本ではなく、参照用メタデータです。
- `source_template` を設定する場合のみ形式検証されます（`path` がある場合は空白禁止、`sha256` がある場合は 64 桁の小文字 hex）。

## パス規約
### 実行パス（厳格）
- `runtime_config_dir` は `artifact_root` 基準の相対パスのみ許可
- `..` による `artifact_root` 外への脱出は禁止

### 参照パス（管理用）
- `artifact_root` は absolute/relative の両方を許可
- relative の `artifact_root` は `artifact.yml` の所在ディレクトリ基準で解決
- `source_template.path` は管理情報なので absolute/relative の両方を許可（設定時のみ検証対象）

## Manifest（`artifact.yml`）最小スキーマ
```yaml
schema_version: "1"
project: esb-dev
env: dev
mode: docker
artifacts:
  - artifact_root: ../service-a/.esb/template-a/dev
    runtime_config_dir: runtime-config
```

任意フィールドを使う場合の追加例:
```yaml
generated_at: "2026-02-20T00:00:00Z"
generator:
  name: producer
  version: v0.0.0
artifacts:
  - artifact_root: ../service-a/.esb/template-a/dev
    runtime_config_dir: runtime-config
    source_template:
      path: /path/to/template-a.yaml
      sha256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

## 契約境界（確定）
本契約は以下の 2 層を明確に分離します。

- Artifact Apply Contract（Payload 契約）
  - `artifact.yml` と `artifact_root` 配下のファイルから、設定マージ・provision 前段を実行できることを定義します。
  - ここで扱うのは「適用入力の完全性」です。

## 推奨フィールド（Payload 契約）
- Manifest:
  - `generated_at`
  - `generator`（name/version）
  - `merge_policy`（例: `last_write_wins_v1`）

## Payload 整合性ポリシー（現行実装）
- schema/path 充足を必須条件とします。
- `source_template` は任意で、設定された場合のみ形式検証を行います。
- artifact 内 runtime hook の digest 一致は互換性判定条件に含めません。

## 移行互換ポリシー
- 本契約の正本は `artifact.yml` 単一です。
- producer は `artifact.yml` 単一形式のみを出力します。
- apply は `artifact.yml` 以外を受け付けません。

## Secret ポリシー
- `compose.env` には非機密値のみを含めます。
- 機密値は成果物外（run-time 用 env ファイルなど）で注入します。
- ログに機密値を出力してはいけません（キー名のみ許可）。

## 失敗分類
- Hard fail:
  - 必須ファイル欠落
  - schema major 非互換
  - `artifact_root` または entry 内パス解決失敗
  - `source_template` が設定されている場合の形式不正（空白 path / 不正 sha256）
  - 複数テンプレート時に merge 規約どおりの runtime-config を生成できない

## 実装責務
- Producer（外部ツール / 手動生成）:
  - `artifact.yml` を出力
  - `artifact.yml` を atomic write
- Applier（adapter / 手動適用）:
  - `artifact.yml` を検証
  - `artifacts[]` 配列順で runtime-config をマージし `esb-runtime-config` volume へ反映
  - `artifactctl deploy` と provision を実行
- Runtime Consumer（Gateway/Provisioner/Agent）:
  - 反映済み設定を読み込むのみ
  - 生成系ツールへの依存を持たない

## ツール責務（確定）
- `tools/artifactctl`（Go 実装）:
  - `deploy` の正本実装を提供する（検証 + apply を実行。必要時の image build/pull を含む）
  - schema/path/merge 規約の判定を一元化する
  - image build 時の lambda base 選択は実行時環境（registry/tag/stack）に従い、artifact 内 `runtime-base/**` を根拠にしない
  - `tools/artifactctl/cmd/artifactctl` は command adapter、実ロジック正本は `pkg/deployops` + `pkg/artifactcore` とする
- producer 側 apply adapter:
  - `artifactctl deploy` と同じ Go 実装を呼ぶ薄いアダプタとして振る舞う

repo 分離後の依存方向:
- core repo が `pkg/artifactcore` を保有する
- producer repo は core 側モジュールを参照して同一 core ロジックをリンクする（または同一バイナリを呼び出す）
- core <- producer の逆依存は作らない

artifactcore 配布/開発ルール:
- producer adapter module と `tools/artifactctl/go.mod` に `pkg/artifactcore` の `replace` を置かない。
- CI は `go.mod` 側の `replace` 混入と `services/* -> tools/*|pkg/artifactcore` 逆依存を拒否する。

## フェーズ別ユースケース整理（Producer 経路 / Direct Apply 経路）
この契約では「生成」と「適用」を分離します。
生成済み成果物を入力に **Phase 3 以降は手動実行可能** とします。

注記:
- ここでいう「手動」は「オペレータがコマンドを直接実行する運用」を意味します。
- 「手動」は「shell にロジックを実装すること」を意味しません。
- 判定・適用ロジックの正本は常に Go 実装（`pkg/deployops` + `pkg/artifactcore`、`tools/artifactctl` は adapter）です。

| フェーズ | Producer 経路 | Direct Apply 経路 |
|---|---|---|
| 1. テンプレート解析 | producer が SAM を解析 | 実行しない（生成済み成果物を受領） |
| 2. 生成（Dockerfile / config） | `artifact.yml` を出力（`artifacts[]` に全テンプレートを記録） | 実行しない |
| 3. Artifact 適用（検証 + 設定反映） | producer 側 apply adapter が実行 | `artifactctl deploy --artifact ...` を実行 |
| 4. Provision | provisioner を実行 | `docker compose up` の起動シーケンス内で自動実行（必要時は `docker compose --profile deploy run --rm provisioner` を明示実行） |
| 5. Runtime 起動 | `docker compose up` | `docker compose up` |

補足:
- `artifactctl deploy` は `artifact_root` を読み取り専用として扱い、`artifact_root` 配下へ一時ファイルを書き込みません。

## Producer コマンド責務（外部管理）
- producer のコマンド体系は本リポジトリ管理外です。
- ただし責務境界は固定します:
  - Generate フェーズは `artifact.yml` と runtime-config の出力に限定する
  - Apply フェーズは shared core（`pkg/deployops` + `pkg/artifactcore`）を利用する
  - `.esb/staging/**` への merge は apply フェーズ責務とする

## 補足（外部テンプレート）
- テンプレートが EBS repo 外でも、出力はテンプレート基準の `.esb/...` を維持します。
- 相対 `CodeUri` / Layer 解決はテンプレート配置基準を維持し、既存 Lambda コード変更を要求しません。
- 複数テンプレート時の適用順と対象は `artifact.yml` の `artifacts[]` が正本です。

## 手動ランブック（Phase 3-5）
前提: `docker`, `docker compose`, `artifactctl` が利用可能であること。

### 0) 変数
```bash
ARTIFACT="/path/to/artifact.yml"
COMPOSE_FILE="/path/to/esb/docker-compose.docker.yml"
SECRETS_ENV="/path/to/secrets.env"   # 成果物外で管理
RUN_ENV="/path/to/run.env"
```

### 1) Artifact 適用（検証 + 設定反映）
```bash
test -f "${ARTIFACT}"
artifactctl deploy \
  --artifact "${ARTIFACT}"

cat "${SECRETS_ENV}" > "${RUN_ENV}"
```

注記:
- `artifactctl deploy` は検証と apply を実行します（必要時に image build/pull を実行し得ます）。
- image build では artifact 作成時の `runtime-base/**` ではなく、実行時環境の lambda base を使用します。
- `artifactctl deploy` は deploy 時に必要な lambda base を確保し、関数 build が 0 件のときは既定の `esb-lambda-base:<resolved-tag>` を target registry へ確保します。
- `artifactctl deploy` は最終 runtime-config（volume 内 `functions.yml`）でも、deploy で build/push した function image を実行時 registry へ正規化します。

### 2) Provision 実行（任意: 明示再実行したい場合）
```bash
docker compose --env-file "${RUN_ENV}" -f "${COMPOSE_FILE}" --profile deploy run --rm provisioner
```

### 3) Runtime 起動（起動時に Provisioner は自動実行）
```bash
docker compose --env-file "${RUN_ENV}" -f "${COMPOSE_FILE}" up -d
```

### 4) 確認
```bash
curl -k https://127.0.0.1/health
```

## E2E 契約（現行）
- E2E matrix から `deploy_driver` / `artifact_generate` は撤去済みで、artifact 実行経路のみを許可します。
- テストはコミット済み `e2e/artifacts/*` を consume し、ランタイムで generate は行いません。
- fixture 更新時のみ `e2e/scripts/regenerate_artifacts.sh` により外部の artifact 生成コマンドを使用します。
