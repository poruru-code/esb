<!--
Where: services/provisioner/docs/architecture.md
What: Provisioner execution flow and responsibilities.
Why: Document the resource initialization path triggered by deploy.
-->
# Provisioner アーキテクチャ

## 概要
Provisioner は `resources.yml` を読み取り、S3（RustFS）と DynamoDB（ScyllaDB Alternator）
に対して **必要なリソースを作成**します。既存リソースはスキップされ、冪等的に動作します。

## 実行シーケンス

```mermaid
sequenceDiagram
    autonumber
    participant CLI as esb deploy
    participant DC as docker compose
    participant PR as provisioner
    participant S3 as RustFS (S3)
    participant DB as ScyllaDB (Dynamo)

    CLI->>DC: docker compose run provisioner
    DC->>PR: start container
    PR->>PR: load /app/runtime-config/resources.yml
    alt Dynamo tables defined
        PR->>DB: create_table (boto3)
        DB-->>PR: success/exists
    end
    alt S3 buckets defined
        PR->>S3: create_bucket (boto3)
        S3-->>PR: success/exists
        PR->>S3: put_bucket_lifecycle_configuration (optional)
    end
    PR-->>DC: exit 0/1
```

## 役割
- `resources.yml` を読み取り、**DynamoDB テーブル**と **S3 バケット**を作成
- 既存リソースは **スキップ**（エラーにしない）
- `LifecycleConfiguration` の一部を boto3 形式に変換して適用

## データソース
### マニフェスト
`/app/runtime-config/resources.yml` が唯一の入力です。

### エンドポイント
`sitecustomize.py` により以下の env を boto3 が参照します:
- `DYNAMODB_ENDPOINT`
- `S3_ENDPOINT`

---

## Implementation references
- `services/provisioner/src/main.py`
- `services/provisioner/entrypoint.sh`
- `docker-compose.docker.yml`
- `docker-compose.containerd.yml`
