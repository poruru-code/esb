# Zero-Config アーキテクチャ移行計画書 (最終版)

## 1. 目的と概要
本ドキュメントは、ESB (Edge Serverless Box) CLI の簡素化と、Docker Native なアーキテクチャへの完全移行に関する技術仕様書です。
これまでの `esb up` によるモノリシックなオーケストレーションを廃止し、**「Build-Time Resource Generation」** と **「Runtime Python Provisioning」** を組み合わせた Zero-Config アーキテクチャを採用します。

### 主な変更点
*   **CLIの責務縮小**: `esb` CLI は「ビルド (アーティファクト生成)」に特化します。
*   **Docker Compose への委譲**: サービスの起動、依存関係管理は標準の `docker compose up` に一任します。
*   **Python Provisioner の導入**: クラウドリーソース (DynamoDB, S3) の初期化を行う専用のマイクロサービスを導入します。

---

## 2. 廃止される機能と新しいワークフロー

> [!WARNING]
> **破壊的変更**: `esb up`, `esb down`, `esb logs`, `esb sync`, `esb env prepare` は廃止されます。

### 新しい開発ワークフロー
1.  **Build**: `esb build`
    *   Functions, Routing 情報に加え、新たに `.esb/config/resources.yml` を生成します。
2.  **Run**: `docker compose up -d`
    *   `provisioner` コンテナが起動し、リソースを作成します。
    *   `gateway` コンテナは `provisioner` の完了を待ってから起動します。

---

## 3. 技術仕様 (Technical Specification)

### 3.1 リソース定義スキーマ (`resources.yml`)
Generator (`esb build`) が出力するリソース定義ファイルです。Goの構造体 `cli/internal/manifest/resources.go` と厳密に対応します。

**ファイルパス**: `.esb/config/resources.yml`

```yaml
DynamoDB:
  - TableName: "example-table"
    KeySchema:
      - AttributeName: "PK"
        KeyType: "HASH"
    AttributeDefinitions:
      - AttributeName: "PK"
        AttributeType: "S"
    BillingMode: "PAY_PER_REQUEST"
S3:
  - BucketName: "example-bucket"
```

### 3.2 Provisioner Service 詳細設計
ランタイム時にリソースを作成する Python 製マイクロサービスです。

#### ディレクトリ構成 (`services/provisioner/`)
```text
services/provisioner/
├── Dockerfile          # 後述の定義
├── pyproject.toml      # 依存: boto3, pyyaml
├── src/
│   ├── main.py         # メインロジック
│   └── sitecustomize.py # ビルド時に cli/assets からコピー
```

#### Dockerfile 仕様
ベースイメージ `esb-python-base` (Debian) には `uv` が含まれないため、明示的にインストールします。

```dockerfile
# syntax=docker/dockerfile:1.4
FROM esb-python-base:latest

# uv のインストール (公式イメージからコピー)
COPY --from=ghcr.io/astral-sh/uv:0.8.13@sha256:4de5495181a281bc744845b9579acf7b221d6791f99bcc211b9ec13f417c2853 /uv /uvx /usr/local/bin/

WORKDIR /app

# 依存関係のインストール
COPY services/provisioner/pyproject.toml .
ENV VIRTUAL_ENV=/app/.venv
RUN uv venv $VIRTUAL_ENV
ENV PATH="$VIRTUAL_ENV/bin:$PATH"
RUN uv pip install -r pyproject.toml

# ソースコードとアセットの配置
# sitecustomize.py はログ転送とエンドポイント注入に必須
COPY cli/internal/generator/assets/site-packages/sitecustomize.py /app/sitecustomize.py
COPY services/provisioner/src /app/src

ENV PYTHONPATH=/app
CMD ["python", "/app/src/main.py"]
```

#### アプリケーションロジック (`src/main.py`)
冪等性（Idempotency）を担保し、再実行可能な設計とします。
`sitecustomize.py` により、AWS SDK (`boto3`) のリクエストは自動的にローカルコンテナ (`database`, `s3-storage`) へ向き先が変更され、ログは VictoriaLogs へ転送されます。

**処理フロー**:
1. `.esb/config/resources.yml` をロード。
2. **DynamoDB**: `create_table` を実行。`ResourceInUseException` は無視（正常）。
3. **S3**: `create_bucket` を実行。`ClientError` のうち `BucketAlreadyOwnedByYou`, `BucketAlreadyExists` は無視（正常）。

### 3.3 Docker Compose 構成
`docker-compose.docker.yml` 等に以下を追加します。Gateway が Provisioner の完了を待機することが重要です。

```yaml
services:
  provisioner:
    image: ${IMAGE_PREFIX}/provisioner:${IMAGE_TAG}
    build:
      context: .
      dockerfile: services/provisioner/Dockerfile
    # Init Container として振る舞う
    depends_on:
      database: { condition: service_started }
      s3-storage: { condition: service_started }
      victorialogs: { condition: service_started }
    environment:
      # sitecustomize.py がこれらを使用して boto3 をパッチする
      - DYNAMODB_ENDPOINT=http://database:8000
      - S3_ENDPOINT=http://s3-storage:9000
      - VICTORIALOGS_URL=http://victorialogs:8428
    volumes:
      - .esb/config/resources.yml:/app/config/resources.yml:ro

  gateway:
    # 既存の設定...
    depends_on:
      # プロビジョニング完了まで起動待機 (E2Eテストの同期ズレ防止)
      provisioner: { condition: service_completed_successfully }
```

---

## 4. 実装ステップ

1.  **Phase 1: Generator 改修**
    *   `esb build` で `resources.yml` を出力するように変更。
2.  **Phase 2: Provisioner Service 作成**
    *   Python コード、Dockerfile の実装。
3.  **Phase 3: インフラ定義の更新**
    *   `docker-compose.*.yml` へのサービス追加と依存関係定義。
4.  **Phase 4: クリーンアップ**
    *   `esb up` 等の旧コマンド削除。E2Eテストランナーの修正。

---

このアーキテクチャにより、ESB はより標準的でメンテナンス性の高い構成へと進化します。
