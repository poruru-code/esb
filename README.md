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
- [CLI docs](cli/docs/architecture.md)

## できること（概要）
- `Gateway` + `Agent` + `runtime-node` の分離構成で Lambda 互換の実行基盤を提供
- S3/Dynamo/ログ基盤を同梱し、Compose だけで Control + Compute が起動
- CLI (`esb`) で SAM テンプレートを解析し、設定・Dockerfile・関数イメージを生成

## クイックスタート（最小）
```bash
esb build
docker compose -f docker-compose.containerd.yml up -d
```
詳細な手順・構成は docs に移譲しています。

## CLI（概要）
- `esb deploy`: SAM テンプレート解析、設定/Dockerfile/関数イメージ生成、provisioner 実行（`--build-only`でビルドのみ）
- `esb version`: ビルド情報表示

## 開発 / コントリビュート
### 推奨ツール
- Go 1.25.1 / Python 3.12+ / `uv` / `mise` / `lefthook` / Docker & Compose

### セットアップ（推奨フロー）
```bash
mise trust
mise install
mise run setup
```

### Lint / Format
```bash
uv run ruff check .
uv run ruff format .
```

### テスト
```bash
# Python unit tests (gateway)
python e2e/run_tests.py --unit-only
```
