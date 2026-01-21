# CLI 簡素化仕様書

## 目的
`docker compose` の機能と重複するコマンドを削除し、`esb` CLI を簡素化します。ただし、Lambda イメージのビルドや SAM リソースのプロビジョニングなど、独自の機能は維持します。

## ユーザー確認事項
> [!IMPORTANT]
> **環境セットアップの変更**: 現行の `esb up` は、ネットワークサブネットや IP アドレスの動的計算（並列環境用）およびシークレットの生成を行っています。
> これに代わり、`.env` ファイルを生成する `esb env prepare` コマンドの追加を提案します。`docker compose up` を実行する前（または環境変更時）に一度実行する必要があります。

> [!WARNING]
> **ワークフローの変更**: `esb up` コマンドは削除されます。今後は以下の手順を使用します：
> 1. `esb env prepare`（ネットワーク設定やシークレットを含む `.env` を生成。これまで `up` 内部で行われていた処理を代替）
> 2. `docker compose --env-file .env up -d`
> 3. `esb sync`（SAM テンプレートに基づいて DynamoDB テーブルや S3 バケットをプロビジョニングする新コマンド）

## 提案される変更

### CLI コマンド

#### [削除] 不要なコマンド
以下のコマンドは `docker compose` で直接扱う方が適切であるため削除します：
- `esb up`
- `esb down`
- `esb logs`
- `esb stop`
- `esb prune`

#### [新規] `esb sync`
起動後のプロビジョニングを行う新しいコマンドです：
- **目的**: `template.yaml` で定義されたリソース（DynamoDB テーブル、S3 バケット）を実行中のローカル環境に同期します。
- **動作**:
    - 実行中の Docker コンテナ（Gateway, Database, S3）に接続します。
    - `template.yaml` を解析します。
    - 不足している DynamoDB テーブルや S3 バケットを作成します。
    - **[追加]** 実行中のコンテナからポート情報を検出し、`ports.json` を生成または表示します（テストランナーやユーザーが利用するため）。
    - （オプション）検出されたポートを表示します。

#### [変更] `esb env`
環境準備をサポートするために `esb env` を拡張します：
- `esb env prepare`（または `esb env init`）を追加：
    - 必要なセキュリティキー（JWT, API キー）を生成します。
    - 動的なネットワークサブネットを計算します（`dev`/`test` 環境間の衝突を防止）。
    - これらを `.env`（または指定されたファイル）に書き出し、`docker compose` で利用可能にします。

### ワークフロー比較

| アクション           | 旧ワークフロー            | 新ワークフロー                                |
| :------------------- | :------------------------ | :-------------------------------------------- |
| **セットアップ**     | （`up` コマンド内で隠蔽） | `esb env prepare`                             |
| **起動**             | `esb up`                  | `docker compose --env-file .env up -d`        |
| **プロビジョニング** | （`up` 内で自動実行）     | `esb sync`                                    |
| **停止**             | `esb down`                | `docker compose down`                         |
| **ログ**             | `esb logs`                | `docker compose logs -f`                      |
| **ビルド**           | `esb build`               | `esb build`                                   |
| **クリーンアップ**   | `esb prune`               | `docker system prune` / `docker volume prune` |

### E2E テストランナー (`e2e/run_tests.py`) の変更
現在、テストランナーは `esb up` や `esb down` を内部で呼び出しています。これを新しいワークフローに合わせて修正します。

- `e2e/runner/executor.py` の修正:
    - `esb up` の呼び出しを、`esb env prepare` -> `docker compose up` -> `esb sync` のシーケンスに置き換えます。
    - `esb down` / `stop` の呼び出しを `docker compose` コマンドに置き換えます。
- ポート情報の取得:
    - `esb sync` が生成する `ports.json` (または標準出力) を利用するように `e2e/runner/env.py` のロジックを確認・調整します。

## 詳細計画

### 1. コマンドの削除
- `cli/internal/commands/up.go`, `down.go`, `logs.go`, `stop.go`, `prune.go` を削除します。
- `cli/internal/commands/app.go` を更新し、コマンドのディスパッチ処理を削除します。

### 2. `esb sync` の実装
- `cli/internal/commands/sync.go` を作成します。
- `up.go` からロジック（プロビジョニングとポート公開）を移植します。
- `cli/internal/provisioner` を再利用します。

### 3. シークレット/Env ヘルパーの実装
- `env` コマンドを更新し、`prepare` サブコマンドをサポートします。
- `helpers/env_defaults.go` のロジックを移植し、`os.Setenv` の代わりにファイル書き出しを行うようにします。

### 4. E2E テストの修正
- `e2e/runner/executor.py` を更新し、`run_esb` 呼び出しを新コマンド群に置き換えます。

## 検証計画

### 手動検証
1.  **準備**: `esb env prepare` を実行し、`JWT_SECRET` や `NETWORK_EXTERNAL` が含まれる `.env` が作成されることを確認します。
2.  **起動**: `docker compose --env-file .env up -d` を実行し、コンテナが起動することを確認します。
3.  **同期**: `esb sync` を実行し、テーブルやバケットの作成が報告されることを確認します。
4.  **確認**: `curl` やブラウザを使用し、`esb sync` で報告されたポートを使って Gateway/S3 のヘルスチェックを行います。
