<!--
Where: docs/deploy-artifact-contract.md
What: Contract for deploy artifacts that can be consumed without esb CLI.
Why: Define a stable boundary between artifact producer (CLI/manual) and runtime consumer.
-->
# Deploy Artifact Contract

## 目的
`esb` CLI がなくても、生成済み成果物だけで compose 起動・更新を行えるようにするための契約です。  
この契約は「生成手段」と「適用手段」を分離し、CLI は補助ツールとして扱います。

## 用語
- Artifact Root: 成果物のルートディレクトリ
- Descriptor: Artifact Root 配下の `artifact.json`
- Runtime Config: `runtime-config/` 配下の実行設定

## レイアウト（v1）
```text
<artifact-root>/
  artifact.json
  runtime-config/
    functions.yml
    routing.yml
    resources.yml              # 条件付き
    image-import.json          # 条件付き
  bundle/
    manifest.json              # 条件付き
  compose.env                  # 非機密のみ
  compose.secrets.env.example  # キー名テンプレートのみ
```

## 必須 / 条件付き必須
### 必須
- `artifact.json`
- `runtime-config/functions.yml`
- `runtime-config/routing.yml`

### 条件付き必須
- `runtime-config/resources.yml`: resource 定義を使う場合
- `runtime-config/image-import.json`: image import を使う場合
- `bundle/manifest.json`: bundle/import ワークフローを使う場合

## パス規約
- Descriptor 内のパスは相対パスのみ許可します。
- 相対パスは「`artifact.json` の所在ディレクトリ基準」で解決します。
- 絶対パスは契約違反です。

## Descriptor（`artifact.json`）最小スキーマ
```json
{
  "schema_version": "1",
  "project": "esb-dev",
  "env": "dev",
  "mode": "docker",
  "runtime_config_dir": "runtime-config",
  "bundle_manifest": "bundle/manifest.json",
  "image_prewarm": "all",
  "required_secret_env": []
}
```

## Descriptor 推奨フィールド
- `templates[]`: 元テンプレートの `path` / `sha256` / `parameters`
- `runtime_meta.runtime_hooks`: 実行時フック契約（`api_version` + 任意 digest）
- `runtime_meta.template_renderer`: 生成器契約（`name`, `api_version`, 任意 digest）

## 互換性ポリシー
- 互換判定の主軸は `api_version`（`major.minor`）です。
- `major` 不一致は hard fail。
- `minor` 不一致は warning（strict モードでは hard fail）。
- digest/checksum は既定で監査用途（warning）。strict で hard fail 化します。

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
  - strict 時の digest/checksum 不一致
- Warning:
  - strict でない時の digest/checksum 不一致
  - minor 非互換（strict でない時）

## 運用モード
- 非 strict（既定）: version 互換中心、digest は監査用途
- strict: version + digest/checksum を完全一致で検証（CI 推奨）

## 実装責務
- Producer（CLI / 手動生成）:
  - 上記構造で成果物を出力
  - descriptor を atomic write
- Applier（CLI / 手動適用）:
  - descriptor を検証
  - runtime-config を同期
  - 必要なら prewarm/provision を実行
- Runtime Consumer（Gateway/Provisioner/Agent）:
  - 反映済み設定を読み込むのみ
  - CLI バイナリへの依存を持たない
