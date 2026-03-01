<!--
Where: README.md
What: Project overview and quick links.
Why: Provide a concise entry point and delegate details to docs.
-->
# オンプレミス向けサーバーレス実行基盤

**オンプレミス環境のための、自己完結型サーバーレス実行基盤**
*(A self-contained, serverless-compatible environment for on-premises and local development)*

## クイックリンク
- [System-level docs](docs/README.md)
- [Gateway docs](services/gateway/docs/README.md)
- [Agent docs](services/agent/docs/README.md)
- [runtime-node docs](services/runtime-node/docs/README.md)
- [Provisioner docs](services/provisioner/docs/README.md)

## できること（概要）
- `Gateway` + `Agent` + `runtime-node` の分離構成で Lambda 互換の実行基盤を提供
- S3/Dynamo/ログ基盤を同梱し、Compose だけで Control + Compute が起動
- 生成済み artifact（`artifact.yml` + runtime-config）を `esb-ctl` で適用できる

## クイックスタート（最小）
```bash
docker compose -f docker-compose.containerd.yml up -d
```
詳細な手順・構成は docs に移譲しています。

## Artifact デプロイ（esb-ctl）
`esb-ctl` の利用手順（コマンド一覧、標準フロー、環境変数、トラブルシュート）の正本は `tools/cli/README.md` です。
ここでは最小の入口のみ記載します。

```bash
esb-ctl stack deploy
```

特定 artifact を指定する場合:

```bash
esb-ctl stack deploy --artifact artifacts/<name>/artifact.yml
```

補足:
- `stack deploy` は `artifacts/**/artifact.yml` を自動検出し、複数ある場合は選択できます。
- 固定名 registry コンテナ（既定: `esb-infra-registry`）が別 compose project 所有で残っている場合は、解消コマンドを表示して fail-fast します。
- 詳細は `tools/cli/README.md` を参照してください。

## 開発 / コントリビュート
### 推奨ツール
- Go 1.25.1 / Python 3.12+ / `uv` / `mise` / `lefthook` / Docker & Compose

### セットアップ（推奨フロー）
```bash
mise trust
mise install
mise run setup
```
`mise run setup` は開発用ツールの検証と `esb-ctl` インストールを行います。
証明書の作成・ローテーション手順は `docs/certificate-operations.md` を参照してください。
証明書ローテーション用の補助タスク: `mise run rotate:certs:leaf:docker` / `mise run rotate:certs:leaf:containerd` / `mise run rotate:certs:all:docker` / `mise run rotate:certs:all:containerd`

### Lint / Format
```bash
uv run ruff check .
uv run ruff format .
```

### テスト
```bash
# Python unit tests (gateway)
uv run pytest services/gateway/tests -v
```
