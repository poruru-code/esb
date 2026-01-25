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

### 0. グローバル設定

| 変数名 | 初期値 | 用途 |
| --- | --- | --- |
| `ENV` | `default` | 環境名の識別子。`PROJECT_NAME` やコンテナ名の suffix に使われます。 |
| `PROJECT_NAME` | `<brand>-<env>` | Compose のプロジェクト名/接頭辞。未指定時は `CLI_CMD + ENV` で自動生成されます。 |
| `<BRAND>_TAG` | `latest` | イメージタグ。本番は不変タグ（`vX.Y.Z` / `sha-<git-short>`）を指定します。 |
| `<BRAND>_REGISTRY` | なし | containerd 系のイメージレジストリ（末尾 `/` で正規化）。 |
| `CERT_DIR` | `~/.<brand>/certs` | mTLS 証明書のマウント元パス。 |

補足: ビルド由来のトレーサビリティ情報は `/app/version.json` に焼き込みます。環境変数での指定は不要です。

### 1. Gateway (`services/gateway`)

Gateway は Lambda 環境の "Master Config" として機能し、サービスエンドポイントをワーカーに注入します。

| 変数名                | Source (`.env` / Default)  | Gatewayでの用途                                                                  |
| --------------------- | -------------------------- | -------------------------------------------------------------------------------- |
| `JWT_SECRET_KEY`      | **必須**                   | `config.py`: 認証トークンの署名・検証に使用。                                    |
| `S3_ENDPOINT`         | (任意)                     | 明示的に上書きする場合に使用。未設定時は `http://s3-storage:9000` が注入される。 |
| `DYNAMODB_ENDPOINT`   | (任意)                     | 明示的に上書きする場合に使用。未設定時は `http://database:8000` が注入される。   |
| `VICTORIALOGS_URL`    | `http://victorialogs:9428` | 自身のログ送信先。                                                               |
| `CONTAINERS_NETWORK`  | `NETWORK_EXTERNAL`          | 自身の所属チェックおよびワーカーの状態監視に使用。                               |
| `RUSTFS_ACCESS_KEY`  | (自動生成)                 | S3 ストレージ (RustFS) のアクセスキー。未指定時は `esb` またはランダム値が設定される。 |
| `RUSTFS_SECRET_KEY`  | (自動生成)                 | S3 ストレージ (RustFS) のシークレットキー。未指定時はランダム値が設定される。         |
| `DATA_PLANE_HOST` | `10.88.0.1`                | **Containerd Mode**: ネットワークゲートウェイ兼 DNS サーバーの IP。           |

### 2. Agent & Runtime Node

| 変数名               | Source (`.env` / Default) | Agent/Runtimeでの用途                                                                                  |
| -------------------- | ------------------------- | ------------------------------------------------------------------------------------------------------ |
| `CNI_GW_IP`          | `DATA_PLANE_HOST`     | **Networking**: `runtime-node` 内でブリッジインターフェース (`esb-cni0`) に設定されるゲートウェイ IP。 |
| `CNI_DNS_SERVER`     | (任意)                    | **Networking**: ワーカー DNS の明示的なネームサーバー。未指定時は `CNI_GW_IP` または `10.88.0.1`。     |
| `CNI_SUBNET`         | (任意)                    | **Networking**: CNI のサブネット範囲。IPAM の subnet/range に反映される。                             |
| `CNI_NET_DIR`        | `/var/lib/cni/networks`   | **Networking**: CNI IP 割り当てファイルの保存先。Agent が `List` 時に IP を再解決する際に参照する。     |
| `CONTAINER_REGISTRY` | (内部管理)               | **Distribution**: Containerd モードの関数イメージ取得先。Compose が `<BRAND>_REGISTRY` から設定する内部値であり、運用者は変更しない。HTTPS が必須（Insecure は非サポート）。 |
| `CONTAINERD_RUNTIME` | (任意)                    | **Runtime**: `aws.firecracker` を指定すると Firecracker runtime/shim を使用する。 |
| `AGENT_INVOKE_MAX_RESPONSE_SIZE` | `10485760` (10MB) | **Security**: `InvokeWorker` レスポンスの最大サイズ制限（バイト）。                                   |
| `AGENT_GRPC_TLS_DISABLED` | (空) | **Security**: `1` を設定すると gRPC TLS を無効化します（デフォルトは有効）。                             |
| `AGENT_GRPC_REFLECTION` | (空) | **Security**: `1` を設定すると gRPC Reflection を有効化します（デフォルトは無効）。                     |
| `AGENT_LOG_LEVEL` | `info` | **Observability**: ログレベル（`debug`, `info`, `warn`, `error`）。未設定時は `LOG_LEVEL` を参照。 |\n| `LOG_LEVEL` | `info` | **Observability**: システム共通のログレベル設定。`AGENT_LOG_LEVEL` が優先される。 |
| `AGENT_LOG_FORMAT` | `text` | **Observability**: ログ形式（`text`, `json`）。                                                           |
| `AGENT_METRICS_PORT` | `9091` | **Observability**: Prometheus `/metrics` エンドポイント用ポート。内部ネットワーク限定を推奨。 |

---

## Level 3: Worker Injection (Gateway -> Lambda)

### Consolidated Service Discovery (サービス解決の統合)

CoreDNS の導入により、すべての実行モード（Docker, Containerd）において、Lambda ワーカーは**論理サービス名**を使用して各サービスにアクセスできるようになりました。

1.  **デフォルトの解決プロセス**
    *   Gateway は以下のエンドポイントをデフォルトとしてワーカーに注入します：
        *   S3: `http://s3-storage:9000`
        *   DynamoDB: `http://database:8000`
        *   VictoriaLogs: `http://victorialogs:9428`
2.  **実行モードごとの解決方法**
    *   **Docker モード**: Docker 内部 DNS がサービス名をコンテナ IP に解決します。
    *   **Containerd モード**: `runtime-node` 上の **CoreDNS サイドカー** が、Docker 内部 DNS へリクエストをフォワードします。
    *   **Containerd + Firecracker**: `CONTAINERD_RUNTIME=aws.firecracker` の場合は WireGuard ゲートウェイ (`10.99.0.1`) を経由する構成を取ります。

---

## 全変数リファレンス (Reference)

### Gateway (`docker-compose.<mode>.yml` & Adapters)

```yaml
environment:
  # 共通設定 (docker-compose.yml)
  - JWT_SECRET_KEY=${JWT_SECRET_KEY}
  - CONTAINERS_NETWORK=${NETWORK_EXTERNAL:-${PROJECT_NAME:-esb-${ENV:-default}}-external}
  - DATA_PLANE_HOST=${DATA_PLANE_HOST:-10.88.0.1}

  # サービスエンドポイント (上書きが必要な場合のみ設定)
  - S3_ENDPOINT=${S3_ENDPOINT}
  - DYNAMODB_ENDPOINT=${DYNAMODB_ENDPOINT}
```

### Runtime Node (`docker-compose.containerd.yml`)

```yaml
environment:
  - CNI_GW_IP=${DATA_PLANE_HOST:-10.88.0.1}
  # 注意: 以前の DNAT_S3_IP, DNAT_DB_IP 等は CoreDNS 移行に伴い廃止されました。
```

### 注意事項

- **gRPC セキュリティ**: デフォルトで mTLS が有効です。証明書は `/app/config/ssl/` 配下の `server.crt`/`server.key` および `RootCA` (meta.RootCACertPath) を参照します。
- **イメージプル**: Insecure Registry (HTTP) はサポートされていません。常に HTTPS / mTLS (CA) 接続が試行されます。
- **Metrics (Docker モード)**: Docker 実行時の `GetContainerMetrics` は現在サポートされておらず、エラーとなります。Metrics を利用する場合は containerd モードを使用してください。
