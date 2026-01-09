# 開発者ガイド (Development Guide)

Edge Serverless Box (ESB) の開発へようこそ！
ここでは、開発環境のセットアップ、コード品質ツール、テストの実行方法について説明します。

## 1. 開発環境セットアップ

本プロジェクトでは、パッケージ管理に `uv`、Git フック管理に `lefthook` を使用しています。

### 必須ツール
*   Python 3.12+
*   [uv](https://github.com/astral-sh/uv) (高速なPythonパッケージマネージャ)
*   Docker & Docker Compose
*   [mise](https://mise.jdx.dev/) (任意: 依存ツールの一括管理)

### 初期セットアップ手順

リポジトリをクローンした後、以下のコマンドを実行して依存関係をインストールし、Gitフックを有効化してください。

```bash
# 依存関係のインストール (devグループを含む)
uv sync --all-extras

# Gitフックのインストール (pre-commit等)
lefthook install
```

これにより、`.venv` ディレクトリに仮想環境が作成され、コミット時に自動的に Lint と型チェックが実行されるようになります。

### mise を使う場合 (推奨)

`.mise.toml` を使ってツール類をまとめて導入できます。

```bash
mise trust
mise install
mise run setup
```

`mise run setup` は `uv sync --all-extras` と `lefthook install` を実行します。

## 2. コーディング規約とツール

コードの品質を保つために、以下のツールを使用しています。これらは `uv sync` で自動的にインストールされます。

### Lint & Formatting (Ruff)

Lint（静的解析）とフォーマットには、高速な [Ruff](https://docs.astral.sh/ruff/) を使用しています。
Black, Isort, Flake8 などの機能はすべて Ruff に統合されています。

```bash
# Lint の実行（修正可能なものは --fix で自動修正）
uv run ruff check . --fix

# フォーマットの実行
uv run ruff format .
```

### Type Checking (Ty)

型チェックには [Ty](https://github.com/bwhyman/ty) (Type-checker wrapper) を使用しています。
これは `mypy` や `pyright` のような体験を提供しますが、設定が統合されています。

```bash
# 型チェックの実行
uv run ty .
```

特に `tools/` および `services/gateway/` ディレクトリ配下では厳密な型チェックを行っています。

## 3. 推奨開発環境 (VS Code)

VS Code を使用する場合、プロジェクトルートの推奨設定 (`.vscode/extensions.json`, `.vscode/settings.json.recommended`) を利用することを強く推奨します。

### 推奨拡張機能
*   **Ruff** (`charliermarsh.ruff`): 保存時に自動フォーマットとimport整理が行われます。
*   **Ty** (`astral-sh.ty`): エディタ上でリアルタイムに型エラーを表示します。

### 設定の注意点
`ty` 拡張機能を使用するため、標準の `Pylance` による型チェックは無効化 (`"python.languageServer": "None"`) されています。これは `ty` (LSP) との競合を防ぐためです。

## 4. テストの実行

### E2E テスト

統合テストランナー `e2e/run_tests.py` を使用します。

```bash
# 全テストスイートの実行
python e2e/run_tests.py

# 特定のプロファイル（例: Containerdモード）のみ実行
python e2e/run_tests.py --profile e2e-containerd
```

### ユニットテスト

```bash
# ユニットテストのみ実行
python e2e/run_tests.py --unit-only
```
