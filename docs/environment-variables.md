<!--
Where: docs/environment-variables.md
What: Environment variable reference and propagation flow.
Why: Trace how configuration moves from Host -> Compose -> Services -> Lambda Workers.
-->
# 環境変数と構成の伝播 (Variable Propagation)

## 概要

本ドキュメントでは、Edge Serverless Box で使用される環境変数が**どこで定義され**、**どのように各コンポーネントへ伝播し**、最終的に**Lambdaワーカーに注入されるか**のデータフローを解説します。これにより、設定変更がシステム全体にどのように影響するかを理解できます。

---

## 🚀 Propagation Flow (構成の伝播経路)

ESB の設定は以下の3段階で伝播します：

1.  **Level 1: Host Configuration** (`.env` -> `docker-compose`)
    *   開発者が `.env` で設定した値が `docker-compose` によって読み込まれ、各コンテナの環境変数として渡されます。
2.  **Level 2: Service Configuration** (Gateway/Agent/Runtime)
    *   各サービス (`services/gateway/config.py` や `services/agent/main.go` など) が、自身に渡された環境変数を読み込み、内部設定として利用します。
3.  **Level 3: Worker Injection** (Gateway -> Lambda)
    *   Gateway が Lambda コンテナを作成する際、自身の内部設定値や、Lambdaワーカー専用の変数を**動的に生成し、ワーカーコンテナに環境変数として注入**します。

---

## Level 1 & 2: Host -> Services (docker-compose & サービス内部)

各コンポーネントが `docker-compose` から受け取り、自身の内部ロジックで使用する主要な変数です。

### 1. Gateway (`services/gateway`)

Gateway は Lambda 環境の "Master Config" として機能し、多くの変数をワーカーへの注入元として利用します。

| 変数名 | Source (`.env` / Default) | Gatewayでの用途 |
|--------|---------------------------|-----------------|
| `JWT_SECRET_KEY` | **必須** | `config.py`: 認証トークンの署名・検証に使用。 |
| `S3_ENDPOINT` | (Dockerモードのみ) | **Injection Source**: ワーカーに `AWS_ENDPOINT_URL_S3` として渡す値を決定。Containerdモードでは自動生成されるため不要。 |
| `DYNAMODB_ENDPOINT` | (Dockerモードのみ) | **Injection Source**: ワーカーに `AWS_ENDPOINT_URL_DYNAMODB` として渡す値を決定。 |
| `VICTORIALOGS_URL` | `http://esb-victorialogs:9428` | 自身のログ送信先。 |
| `GATEWAY_VICTORIALOGS_URL` | (Dockerモードのみ) | ワーカー注入専用。Containerdモードでは自動生成されるため不要。 |
| `CONTAINERS_NETWORK` | `ESB_NETWORK_EXTERNAL` | 自身の所属チェックおよびワーカーの状態監視に使用。 |
| `ESB_DATA_PLANE_HOST` | `10.88.0.1` | **Containerd Mode**: 注入用エンドポイントURLを動的に生成するためのホストIP。 |

### 2. Agent & Runtime Node (`docker-compose.containerd.yml`)

| 変数名 | Source (`.env` / Default) | Agent/Runtimeでの用途 |
|--------|---------------------------|-----------------------|
| `CNI_GW_IP` | `ESB_DATA_PLANE_HOST` | **Networking**: `runtime-node` 内でブリッジインターフェース (`esb-cni0`) に設定されるゲートウェイ IP。 |
| `DNAT_*_IP` | `127.0.0.1` | **Networking**: `iptables` ルールを生成し、`ESB_DATA_PLANE_HOST` (:9000等) へのアクセスをこの IP へ DNAT 転送。 |

---

## Level 3: Worker Injection (Gateway -> Lambda)

### Hybrid Variable Resolution (ハイブリッド変数解決)

Gateway は、Lambdaワーカーに注入するエンドポイント (`AWS_ENDPOINT_URL_S3` 等) を以下の優先順位で決定します。これにより、Dockerモード（DNS名依存）とContainerdモード（固定IP依存）の両方をシームレスにサポートします。

1.  **明示的な環境変数 (Docker Mode Priority)**
    *   Gateway コンテナに `S3_ENDPOINT` 等が設定されていれば、それをそのまま使用します。
    *   例: `S3_ENDPOINT=http://esb-s3-storage:9000`
2.  **動的生成 (Containerd Mode Fallback)**
    *   変数が空の場合、`ESB_DATA_PLANE_HOST` (`10.88.0.1`) と標準ポート、標準プロトコル (`http`) を組み合わせて URL を生成します。
    *   生成ロジック: `http://{ESB_DATA_PLANE_HOST}:{DEFAULT_PORT}`
    *   デフォルトポート:
        *   S3: `9000`
        *   DynamoDB: `8001`
        *   VictoriaLogs: `9428`

この仕組みにより、`docker-compose.containerd.yml` の記述量が大幅に削減されています。

---

## 全変数リファレンス (Reference)

### Gateway (`docker-compose.yml` & Adapters)

```yaml
environment:
  # 共通設定 (docker-compose.yml)
  - JWT_SECRET_KEY=${JWT_SECRET_KEY}
  - CONTAINERS_NETWORK=${ESB_NETWORK_EXTERNAL}
  - ESB_DATA_PLANE_HOST=${ESB_DATA_PLANE_HOST:-10.88.0.1}

  # Docker Mode (docker-compose.docker.yml で追加)
  - S3_ENDPOINT=http://${ESB_PROJECT_NAME}-s3-storage:9000
  - DYNAMODB_ENDPOINT=http://${ESB_PROJECT_NAME}-database:8000
  - GATEWAY_VICTORIALOGS_URL=http://${ESB_PROJECT_NAME}-victorialogs:9428

  # Containerd Mode (docker-compose.containerd.yml で追加)
  # (なし - ESB_DATA_PLANE_HOST から自動生成)
```

### Runtime Node (`docker-compose.containerd.yml`)

```yaml
environment:
  - CNI_GW_IP=${ESB_DATA_PLANE_HOST:-10.88.0.1}
  - DNAT_S3_IP=127.0.0.1 (固定)
  - DNAT_DB_IP=127.0.0.1 (固定)
```
