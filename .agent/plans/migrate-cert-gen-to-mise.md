# 実装計画: 証明書生成のPythonスクリプト化 (mise連携)

## 1. 目的
証明書生成ロジックをアプリケーションから切り離し、設定ファイルベースで管理可能なPythonスクリプト (`tools/cert-gen/generate.py`) に移行します。これにより、マルチOS対応の `mkcert` を活用しつつ、設定の柔軟性と保守性を確保します。

## 2. 構成

### 2.1. 設定ファイル (`tools/cert-gen/config.toml`)
証明書の出力先や対象ドメイン/IPを定義します。
- `[certificate]`: 出力ディレクトリ、ファイル名
- `[hosts]`: 固定ドメイン、固定IP、ローカルIP自動追加フラグ

### 2.2. 生成スクリプト (`tools/cert-gen/generate.py`)
Python (3.12, mise管理) で記述し、以下の処理を行います。
1.  `mkcert` コマンドの存在確認
2.  `config.toml` の読み込み
3.  ルートCAのインストール (`mkcert -install`)
4.  設定に基づいた証明書生成コマンドの実行

### 2.3. Mise連携 (`.mise.toml`)
- **[tasks.setup:certs]**: Pythonスクリプトを実行するタスクを定義。
- **依存**: `python`, `mkcert` (miseで管理)

### 2.4. アプリケーション (`cli/internal/app/auth.go`)
- 既存の証明書生成ロジックを全削除。
- ユーザーには証明書がない場合のエラーメッセージやドキュメントで、`mise run setup:certs` の実行を案内する（コード変更としては削除のみ）。

## 3. 手順

1.  `tools/cert-gen/` ディレクトリの作成。
2.  `config.toml` と `generate.py` の作成 (上記構成案に基づき既に作成済み)。
3.  `.mise.toml` にタスク定義を追加。
4.  `cli/internal/app/auth.go` のクリーンアップ。
5.  動作確認 (`mise run setup:certs`)。

## 4. 補足
- `mkcert` はGo製ツールであり、マルチOSバイナリが提供されているため、mise経由でのインストールでクロスプラットフォーム対応可能です。
- Pythonスクリプトは標準ライブラリ + `toml` ライブラリを使用します。 `pyproject.toml` や `uv` で依存管理する場合は別途設定しますが、簡易的には標準機能または `pip install toml` で対応します。
  - ※ ユーザー環境では `uv` が使われているため、`uv run` を使うのがベストプラクティスです。
