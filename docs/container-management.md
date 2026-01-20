# コンテナ管理とイメージ運用

本ドキュメントでは、Edge Serverless Box (ESB) のコンテナ管理とイメージビルドの仕組みについて解説します。

## イメージ階層構造

ESB は効率的なビルドのために、階層化されたイメージ構造を採用しています。

```mermaid
flowchart TD
    A[public.ecr.aws/lambda/python:3.12] --> B[esb-lambda-base:latest]
    B --> C[lambda-xxx:latest]
    
    subgraph Base["ベースイメージ (esb-lambda-base)"]
        B
        B1[sitecustomize.py]
        B2[AWS SDK パッチ]
    end
    
    subgraph Function["Lambda関数イメージ"]
        C
        C1[Layer: common-lib]
        C2[関数コード]
    end
```

| レイヤー           | 内容                             | 更新頻度 |
| ------------------ | -------------------------------- | -------- |
| AWS Lambda RIE     | 公式Pythonランタイム             | 低       |
| `esb-lambda-base`  | sitecustomize.py (AWS SDKパッチ) | 低       |
| Lambda関数イメージ | Layers + 関数コード              | 高       |

## ビルドプロセス

`esb build` コマンドを実行すると、以下の順序でビルドが行われます。

```mermaid
flowchart LR
    A[esb build] --> B[template.yaml パース]
    B --> C[設定ファイル生成]
    C --> D[ベースイメージビルド]
    D --> E[Lambda関数イメージビルド]
```

### ベースイメージ (`esb-lambda-base`)

**ソース**: `cli/internal/generator/assets/`

```
cli/internal/generator/assets/
├── Dockerfile.lambda-base
└── site-packages/
    └── sitecustomize.py    # AWS SDK パッチ & Direct Logging
```

ベースイメージには以下が含まれます:
- **sitecustomize.py**: Python 起動時に自動ロードされ、AWS SDK の挙動修正とログの送信を行います。

### Lambda関数イメージ

**ソース**: Generator により自動生成

生成される Dockerfile（`CONTAINER_REGISTRY` 設定時はプレフィックス付き、未設定時はローカルイメージ名を使用）:
```dockerfile
# CONTAINER_REGISTRY未設定時の例
FROM esb-lambda-base:latest

# Layer (template.yaml で定義)
COPY tests/fixtures/layers/common/ /opt/

# 関数コード
COPY tests/fixtures/functions/xxx/ ${LAMBDA_TASK_ROOT}/

CMD [ "lambda_function.lambda_handler" ]
```

## コンテナライフサイクル

Lambda RIE コンテナは **Go Agent** により動的に管理されます。Gateway は gRPC で Go Agent に依頼し、runtime（docker / containerd）経由でコンテナを起動・削除します。Gateway の Janitor がアイドル/孤児コンテナを定期的に整理します（詳細は [orchestrator-restart-resilience.md](./orchestrator-restart-resilience.md) を参照）。

```mermaid
sequenceDiagram
    participant Client
    participant Gateway
    participant PoolManager
    participant Provisioner as Go Agent (gRPC)
    participant Lambda

    Client->>Gateway: リクエスト
    Gateway->>PoolManager: acquire_worker
    
    alt アイドルコンテナあり
        PoolManager-->>Gateway: コンテナ情報 (Reuse)
    else キャパ余裕あり
        PoolManager->>Provisioner: Provision Request (gRPC)
        Provisioner->>Lambda: Create Container (containerd/CNI)
        Lambda-->>Provisioner: 起動完了
        Provisioner-->>PoolManager: コンテナ情報
        PoolManager-->>Gateway: コンテナ情報 (New)
    else フル稼働
        PoolManager->>PoolManager: キューで待機
        Note over PoolManager: 他の実行が完了し次第割り当て
    end
    
    Gateway->>Lambda: Lambda Invoke
    Lambda-->>Gateway: レスポンス
    Gateway->>PoolManager: release_worker
    Gateway-->>Client: レスポンス
    
    Note over Provisioner,Lambda: 一定時間のリクエスト不在で Janitor が削除
```

### コンテナ状態遷移

| 状態           | 説明                                     |
| -------------- | ---------------------------------------- |
| `STOPPED`      | コンテナ未起動                           |
| `PROVISIONING` | 新規コンテナ作成中                       |
| `BUSY`         | リクエスト処理中（プールから払い出し中） |
| `IDLE`         | リクエスト待機中（プール内で再利用可能） |
| `CLEANUP`      | 長期間未使用による自動削除対象           |

## 運用コマンド

### イメージ管理

```bash
# 全イメージを強制リビルド
esb build --no-cache

# サービス起動時にリビルド
esb up --build
```

### 未使用イメージのクリーンアップ

```bash
# ESB関連のみをクリーンアップ（推奨）
docker images | grep -E "^(esb-|lambda-)" | awk '{print $3}' | xargs docker rmi

# 全 dangling イメージを削除（注意: 他プロジェクトにも影響）
docker image prune -f
```

### コンテナログの確認

```bash
# Go Agent のログ
docker logs esb-agent

# containerd 側の状態確認
ctr -n esb-runtime containers list
```

## トラブルシューティング

### 問題: 古いコードが実行される

**原因**: イメージが再ビルドされていない

**解決策**:
```bash
esb build --no-cache
esb up --build
```

### 問題: 大量の `<untagged>` イメージ

**原因**: 頻繁なリビルドによる中間レイヤーの蓄積

**解決策**:
```bash
docker image prune -f
```

### 問題: コンテナが起動しない

**確認手順**:
```bash
# Go Agent ログの確認
docker logs esb-agent

# containerd 側の状態確認
ctr -n esb-runtime containers list
```
