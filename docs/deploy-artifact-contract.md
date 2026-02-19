<!--
Where: docs/deploy-artifact-contract.md
What: Contract for deploy artifacts that can be consumed without esb CLI.
Why: Define a stable boundary between artifact producer (CLI/manual) and runtime consumer.
-->
# Deploy Artifact Contract

## 目的
`esb` CLI がなくても、生成済み成果物だけで compose 起動・更新を行えるようにする契約です。
この契約は「生成手段」と「適用手段」を分離し、CLI は補助ツールとして扱います。

## 設計原則（Single Manifest）
- 適用対象の正本は `artifact.yml` のみです。
- 単一/複数テンプレートは `artifact.yml` の `artifacts[]` で表現します。
- 複数テンプレート時の deploy 順と merge 順は `artifacts[]` 配列順を唯一の真実にします。
- `.esb` 探索や `ARTIFACT_ROOTS` 手動列挙は非推奨ではなく禁止とします。

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
  runtime-base/               # 条件付き（python base build を行う場合）
    runtime-hooks/
      python/
        docker/
          Dockerfile
        sitecustomize/
          site-packages/
            sitecustomize.py
        trace-bridge/
          layer/
            trace_bridge.py
  bundle/
    manifest.json              # 条件付き

<artifact-root-b>/
  ...
```

## CLI 既定出力先
- `esb deploy` は `artifact.yml` を次へ出力します。
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
- `id`
- `artifact_root`
- `runtime_config_dir`
- `source_template.path`
- `<artifact_root>/<runtime_config_dir>/functions.yml`
- `<artifact_root>/<runtime_config_dir>/routing.yml`

### Entry 条件付き必須
- `<artifact_root>/<runtime_config_dir>/resources.yml`: resource 定義を使う場合
- `<artifact_root>/<bundle_manifest>`: bundle/import ワークフローを使う場合
- `<artifact_root>/runtime-base/runtime-hooks/python/docker/Dockerfile`: `artifactctl deploy` の prepare phase で `esb-lambda-base:*` を build/push する場合
- `<artifact_root>/runtime-base/runtime-hooks/python/sitecustomize/site-packages/sitecustomize.py`: `runtime_meta.runtime_hooks.python_sitecustomize_digest` を検証する場合

## パス規約
### 実行パス（厳格）
- `runtime_config_dir`, `bundle_manifest` は `artifact_root` 基準の相対パスのみ許可
- `..` による `artifact_root` 外への脱出は禁止

### 参照パス（管理用）
- `artifact_root` は absolute/relative の両方を許可
- relative の `artifact_root` は `artifact.yml` の所在ディレクトリ基準で解決
- `source_template.path` は管理情報なので absolute/relative の両方を許可

## Entry ID ルール
- `id` は必須で、`artifact.yml` 内で一意でなければなりません。
- 採番は連番禁止。決定的 ID を使用します。
- 形式: `<template_slug>-<h8>`
- `template_slug`: `source_template.path` のファイル名（拡張子除去）を `[a-z0-9-]` へ正規化
- `h8`: `sha256(canonical_template_ref + \"\\n\" + canonical_parameters + \"\\n\" + canonical_source_sha256)` の先頭8桁
- `canonical_template_ref`: `source_template.path` の文字列を正規化した参照値（`/` 区切り、絶対/相対の区別は保持）
- `canonical_source_sha256`: `source_template.sha256`（未指定時は空文字）
- `canonical_parameters`: key 昇順で `key=value` を `\\n` 連結した文字列（末尾改行なし）
- 絶対化や symlink 解決などのファイルシステム依存処理は ID 算出に使いません（`source_template.path` の記録文字列のみを正規化して使用）。
- `id` は表示/追跡用途であり、適用順序は常に `artifacts[]` 配列順で決定します。
- Applier は `id` を再計算し、manifest 記載値と不一致なら hard fail とします。

### `canonical_template_ref` 正規化アルゴリズム（確定）
1. 入力: `source_template.path` の記録文字列（manifest 上の値）
2. 先頭/末尾の空白を除去する
3. `\` を `/` へ置換する
4. `.` / `..` を lexical に正規化する（`path.Clean` 相当、filesystem 参照なし）
5. 連続する `/` は 1 つに畳み込む
6. 絶対/相対の区別は保持する（先頭 `/` の有無を維持）
7. 文字大小は変更しない

例:
- `./svc/../template.yaml` -> `template.yaml`
- `C:\\work\\sam\\template.yaml` -> `C:/work/sam/template.yaml`
- `/repo//templates/./a.yaml` -> `/repo/templates/a.yaml`

## Manifest（`artifact.yml`）最小スキーマ
```yaml
schema_version: "1"
project: esb-dev
env: dev
mode: docker
artifacts:
  - id: template-a-2b4f1a9c
    artifact_root: ../service-a/.esb/template-a/dev
    runtime_config_dir: runtime-config
    bundle_manifest: bundle/manifest.json
    required_secret_env: []
    source_template:
      path: /path/to/template-a.yaml
      sha256: 2b4f...
      parameters:
        Stage: dev

  - id: template-b-43ad77f0
    artifact_root: ../service-b/.esb/template-b/dev
    runtime_config_dir: runtime-config
    required_secret_env: []
    source_template:
      path: /path/to/template-b.yaml
```

## 推奨フィールド
- Entry:
  - `runtime_meta.runtime_hooks`（`api_version` + `python_sitecustomize_digest`）
  - `runtime_meta.template_renderer`（`name`, `api_version`）
- Manifest:
  - `generated_at`
  - `generator`（name/version）
  - `merge_policy`（例: `last_write_wins_v1`）

## 互換性ポリシー
- 互換判定の主軸は `api_version`（`major.minor`）です。
- `major` 不一致は hard fail。
- `minor` 不一致は warning（strict モードでは hard fail）。
- runtime digest 検証対象は `runtime_meta.runtime_hooks.python_sitecustomize_digest` のみです。
- `runtime_meta.runtime_hooks.java_agent_digest` / `java_wrapper_digest` / `runtime_meta.template_renderer.template_digest` は削除済みです（契約外）。
- digest/checksum は既定で監査用途（warning）。strict で hard fail 化します。
- runtime digest/checksum の検証元は `artifact_root/runtime-base/runtime-hooks/python/**` に固定します（repo root 推定は禁止）。

## 移行互換ポリシー
- 本契約の正本は `artifact.yml` 単一です。
- producer は `artifact.yml` 単一形式のみを出力します。
- apply は `artifact.yml` 以外を受け付けません。

## Secret ポリシー
- `compose.env` には非機密値のみを含めます。
- 機密値は成果物外（例: `--secret-env`）で注入します。
- `required_secret_env` の未充足は hard fail。
- ログに機密値を出力してはいけません（キー名のみ許可）。

## 失敗分類
- Hard fail:
  - 必須ファイル欠落
  - schema major 非互換
  - required secret 未設定
  - strict 時の `python_sitecustomize_digest` 不一致
  - strict 時に runtime digest 検証元（`artifact_root/runtime-base/runtime-hooks/python/**`）が不足・読取不能
  - `artifact_root` または entry 内パス解決失敗
  - `prepare-images` 実行時に必要な `runtime-base` コンテキストが不足
  - `id` 欠落、重複、または再計算値不一致
  - 複数テンプレート時に merge 規約どおりの `CONFIG_DIR` を生成できない
- Warning:
  - strict でない時の `python_sitecustomize_digest` 不一致
  - strict でない時に runtime digest 検証元（`artifact_root/runtime-base/runtime-hooks/python/**`）が不足・読取不能
  - minor 非互換（strict でない時）

## 実装責務
- Producer（CLI / 手動生成）:
  - `artifact.yml` を出力
  - `artifact.yml` を atomic write
- Applier（CLI / 手動適用）:
  - `artifact.yml` を検証
  - `artifacts[]` 配列順で runtime-config をマージし `CONFIG_DIR` へ反映
  - `artifactctl deploy` と provision を実行
- Runtime Consumer（Gateway/Provisioner/Agent）:
  - 反映済み設定を読み込むのみ
  - CLI バイナリへの依存を持たない

## ツール責務（確定）
- `tools/artifactctl`（Go 実装）:
  - `deploy` の正本実装を提供する（内部で image prepare + apply を実行）
  - schema/path/id/secret/merge 規約の判定を一元化する
  - `tools/artifactctl/cmd/artifactctl` は command adapter、実ロジック正本は `pkg/artifactcore` とする
- `esb artifact apply`:
  - `artifactctl deploy` と同じ Go 実装を呼ぶ薄いアダプタとして振る舞う

repo 分離後の依存方向:
- core repo が `pkg/artifactcore` を保有する
- CLI repo は core 側モジュールを参照して同一 core ロジックをリンクする（または同一バイナリを呼び出す）
- core <- CLI の逆依存は作らない

artifactcore 配布/開発ルール:
- `cli/go.mod` と `tools/artifactctl/go.mod` に `replace ../pkg/artifactcore` を置かない。
- ローカル開発時の解決は repo ルート `go.work` の `replace` に限定する。
- CI は `go.mod` 側の `replace` 混入と `services/* -> tools/*|pkg/artifactcore` 逆依存を拒否する。

## フェーズ別ユースケース整理（CLI あり / CLI なし）
この契約では「生成」と「適用」を分離します。
CLI なし運用でも、生成済み成果物を入力に **Phase 3 以降は手動実行可能** とします。

注記:
- ここでいう「手動」は「オペレータがコマンドを直接実行する運用」を意味します。
- 「手動」は「shell にロジックを実装すること」を意味しません。
- 判定・適用ロジックの正本は常に Go 実装（`tools/artifactctl`）です。

| フェーズ | CLI あり（esb 利用） | CLI なし（esb 非依存） |
|---|---|---|
| 1. テンプレート解析 | `esb deploy` / `esb artifact generate` が SAM を解析 | 実行しない（生成済み成果物を受領） |
| 2. 生成（Dockerfile / config） | `artifact.yml` を出力（`artifacts[]` に全テンプレートを記録） | 実行しない |
| 3. Artifact 適用（検証 + 画像準備 + 設定反映） | `esb deploy` が apply phase を実行 | `artifactctl deploy --artifact ... --out ...` を実行 |
| 4. Provision | provisioner を実行 | `docker compose --profile deploy run --rm provisioner` |
| 5. Runtime 起動 | `docker compose up` | `docker compose up` |

補足:
- `artifactctl deploy` の prepare phase は `artifact_root/runtime-base/**` を唯一入力として base image を build します（repo root の `runtime-hooks/**` は参照しません）。
- `apply --strict` の runtime digest 検証は `artifact_root/runtime-base/runtime-hooks/python/**` を唯一入力とします（repo root の `runtime-hooks/**` は参照しません）。

## CLI コマンド責務（明示）
- `esb artifact generate`
  - Generate フェーズ専用（既定は render-only、`--build-images` 指定時のみ image build）
  - `.esb/staging/**` への merge は実行しない（merge/apply は apply フェーズ責務）
  - Apply は実行しない
- `esb artifact apply`
  - Apply フェーズ専用（manifest 入力で merge/apply + provision 前段）
- `esb deploy`
  - `generate -> apply` の合成コマンド
  - 内部実装が分離されていても外部 UX は単一コマンドを維持

## 補足（外部テンプレート）
- テンプレートが EBS repo 外でも、出力はテンプレート基準の `.esb/...` を維持します。
- 相対 `CodeUri` / Layer 解決はテンプレート配置基準を維持し、既存 Lambda コード変更を要求しません。
- 複数テンプレート時の適用順と対象は `artifact.yml` の `artifacts[]` が正本です。

## 手動ランブック（CLI なし、Phase 3-5）
前提: `docker`, `docker compose`, `artifactctl` が利用可能であること。

### 0) 変数
```bash
ARTIFACT="/path/to/artifact.yml"
COMPOSE_FILE="/path/to/esb/docker-compose.docker.yml"
SECRETS_ENV="/path/to/secrets.env"   # 成果物外で管理
CONFIG_DIR="/path/to/merged-runtime-config"
RUN_ENV="/path/to/run.env"
```

### 1) Artifact 適用（検証 + 画像準備 + 設定反映）
```bash
test -f "${ARTIFACT}"
artifactctl deploy \
  --artifact "${ARTIFACT}" \
  --out "${CONFIG_DIR}" \
  --secret-env "${SECRETS_ENV}"

cat "${SECRETS_ENV}" > "${RUN_ENV}"
{
  echo "CONFIG_DIR=${CONFIG_DIR}"
} >> "${RUN_ENV}"
```

注記:
- `artifactctl deploy` は内部で image prepare と apply を順に実行します。
- `deploy --strict` の runtime digest 検証は `artifact_root/runtime-base/runtime-hooks/python/**` を使用します。対象 digest に対応するファイルが欠けると hard fail します。

### 2) Provision 実行
```bash
docker compose --env-file "${RUN_ENV}" -f "${COMPOSE_FILE}" --profile deploy run --rm provisioner
```

### 3) Runtime 起動
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
- fixture 更新時のみ `e2e/scripts/regenerate_artifacts.sh` により `esb artifact generate` を使用します。
