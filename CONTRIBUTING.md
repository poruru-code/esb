# 開発者ガイド (Development Guide)

Edge Serverless Box (ESB) の開発へようこそ！
ここでは、開発環境のセットアップ、コード品質ツール、テストの実行方法について説明します。

## 1. 開発環境セットアップ

本プロジェクトでは、開発ツール（Go, Python/uv, etc.）のバージョン管理とタスク実行に [mise](https://mise.jdx.dev/) を使用することを推奨しています。

### セットアップ手順

1.  **mise のインストール(for Ubuntu)**:
    まだインストールしていない場合は、公式の手順に従ってインストールしてください。
    ```bash
    sudo apt update -y && sudo apt install -y curl
    sudo install -dm 755 /etc/apt/keyrings
    curl -fSs https://mise.jdx.dev/gpg-key.pub | sudo tee /etc/apt/keyrings/mise-archive-keyring.pub 1> /dev/null
    echo "deb [signed-by=/etc/apt/keyrings/mise-archive-keyring.pub arch=amd64] https://mise.jdx.dev/deb stable main" | sudo tee /etc/apt/sources.list.d/mise.list
    sudo apt update
    sudo apt install -y mise
    ```

2.  **シェルの設定 (Activate)**:
    `mise` が管理するツールにパスを通すため、シェル設定ファイル（`~/.bashrc`, `~/.zshrc` 等）に以下を追加してください。
    `mise` がインストールされていない環境でもエラーにならないよう、条件分岐を含めることを推奨します。

    ```bash
    # mise (もしインストールされていれば) を有効化
    if command -v mise >/dev/null 2>&1; then
      eval "$(mise activate bash)"
      # zshの場合は: eval "$(mise activate zsh)"
    fi
    ```
    *設定後はシェルを再起動するか、設定ファイルを読み込んでください。*

3.  **依存ツールのインストール**:
    リポジトリのルートディレクトリで以下を実行します。初回に設定ファイルを信頼（Trust）するか聞かれる場合があります。
    ```bash
    mise trust
    mise install
    ```
    これにより `go` や `uv` が自動的にインストールされ、パスが通ります。

4.  **プロジェクトの初期化**:
    依存関係のインストール（Pythonパッケージ）とGitフックの有効化、CLIツールのビルドを一括で行います。
    ```bash
    mise run setup
    ```

### 必須ツール（miseで自動インストールされます）
*   **Go**: 1.25.1
*   **Python**: 3.12+
*   **uv**: Pythonパッケージマネージャ
*   **mkcert**: ローカル開発用TLS証明書生成ツール
*   **lefthook**: Gitフック管理
*   Docker & Docker Compose (これらは別途システムのインストールが必要です)

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
