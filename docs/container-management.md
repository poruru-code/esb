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

| レイヤー | 内容 | 更新頻度 |
|---------|------|---------|
| AWS Lambda RIE | 公式Pythonランタイム | 低 |
| `esb-lambda-base` | sitecustomize.py (AWS SDKパッチ) | 低 |
| Lambda関数イメージ | Layers + 関数コード | 高 |

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

**ソース**: `tools/generator/runtime/`

```
tools/generator/runtime/
├── Dockerfile.base
└── site-packages/
    └── sitecustomize.py    # AWS SDK パッチ & Direct Logging
```

ベースイメージには以下が含まれます:
- **sitecustomize.py**: Python 起動時に自動ロードされ、AWS SDK の挙動修正とログの送信を行います。

### Lambda関数イメージ

**ソース**: Generator により自動生成

生成される Dockerfile:
```dockerfile
FROM esb-lambda-base:latest

# Layer (template.yaml で定義)
COPY tests/fixtures/layers/common/ /opt/

# 関数コード
COPY tests/fixtures/functions/xxx/ ${LAMBDA_TASK_ROOT}/

CMD [ "lambda_function.lambda_handler" ]
```

## コンテナライフサイクル

Lambda RIE コンテナは Manager により動的に管理されます。Manager は再起動時にも実行中のコンテナを認識し、管理下に復帰させる **Adopt & Sync** 機能を備えています（詳細は [manager-restart-resilience.md](./manager-restart-resilience.md) を参照）。

```mermaid
sequenceDiagram
    participant Client
    participant Gateway
    participant Manager
    participant Lambda

    Client->>Gateway: リクエスト
    Gateway->>Manager: コンテナ状態確認
    
    alt コンテナ未起動
        Manager->>Lambda: docker run
        Lambda-->>Manager: 起動完了
    end
    
    Manager-->>Gateway: ホスト:ポート情報
    Gateway->>Lambda: Lambda Invoke
    Lambda-->>Gateway: レスポンス
    Gateway-->>Client: レスポンス
    
    Note over Manager,Lambda: アイドルタイムアウト後に自動停止
```

### コンテナ状態遷移

| 状態 | 説明 |
|-----|------|
| `STOPPED` | コンテナ未起動 |
| `STARTING` | コンテナ起動中 |
| `RUNNING` | リクエスト受付可能 |
| `IDLE` | アイドル状態（自動停止待ち） |

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
# 特定のLambdaコンテナログ
docker logs lambda-xxx

# リアルタイム監視
docker logs -f lambda-xxx
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
# Manager ログの確認
docker logs onpre-manager

# コンテナ状態の確認
docker ps -a | grep lambda-
```
