<!--
Where: docs/spec.md
What: System specification and deployment overview.
Why: Provide a stable reference for ESB components and deployment models.
-->
# システム仕様書

## 1. 概要
本システムは、コンテナ技術(Docker / containerd)を用いてエッジサーバーレス環境をシミュレートするための基盤です。単一ホストの containerd 構成は `docker-compose.yml` + `docker-compose.node.yml` + `docker-compose.containerd.yml` を組み合わせ、Firecracker 分離構成では Control (`docker-compose.yml`) / Compute (`docker-compose.node.yml`) に分けて起動します。

## 2. コンポーネント構成

システムは以下の主要コンポーネントで構成されます。

```mermaid
flowchart TD
    User["Client / Developer"]
    
    subgraph Host ["Host OS"]
        Gateway["Gateway API<br>(:443)"]
        Agent["Go Agent (gRPC)<br>(:50051)"]
        CoreDNS["CoreDNS (Sidecar)<br>(:53)"]
        RustFS["RustFS S3<br>(:9000)"]
        Console["RustFS Console<br>(:9001)"]
        DB["ScyllaDB<br>(:8001)"]
        Logs["VictoriaLogs<br>(:9428)"]
        
        Gateway -->|Pool Management| PoolManager["PoolManager"]
        PoolManager -->|Capacity Control| ContainerPool["ContainerPool"]
        PoolManager -->|Prune/Reconcile| HeartbeatJanitor["HeartbeatJanitor"]
        
        Lambda["Lambda microVM/Container<br>(Ephemeral)"]
    end

    User -->|HTTP| Gateway
    User -->|S3 API| RustFS
    User -->|Web UI| Console
    User -->|Dynamo API| DB
    User -->|Web UI| Logs
    
    Gateway -->|gRPC| Agent
    Gateway -->|AWS SDK| RustFS
    Gateway -->|AWS SDK| DB
    Gateway -->|HTTP| Lambda
    
    Agent -->|containerd/CNI| Lambda
    Agent -.-|Pull (Containerd/FC only)| Registry["Registry"]
    
    Lambda -->|DNS Query| CoreDNS
    CoreDNS -->|Resolve| RustFS
    CoreDNS -->|Resolve| DB
    CoreDNS -->|Resolve| Logs
    Lambda -->|AWS SDK| RustFS
    Lambda -->|AWS SDK| DB
    Lambda -->|HTTP| Logs
```

### 2.1 Gateway API (FastAPI)
- **役割**: クライアントからのリクエスト受付、認証、およびLambda関数へのリクエストルーティング。
- **通信**: クライアントとはHTTPで通信。内部では Go Agent (gRPC) と連携し、Lambdaコンテナの起動確認とリクエスト転送を行います。
- **ポート**: `443`

#### ディレクトリ構成
```
services/gateway/
├── main.py              # エンドポイント定義（認証、ヘルスチェック、プロキシ）
├── config.py            # 環境変数ベースの設定管理
├── api/                 # DI/依存関係
├── core/                # 共通ロジック（認証、イベント構築、サーキットブレーカー等）
├── models/              # データモデル
└── services/            # ビジネスロジック
    ├── container_pool.py  # Conditionベースの同時実行制御とプーリング
    ├── pool_manager.py    # プール全体の管理
    ├── janitor.py         # アイドル/孤児コンテナ整理
    ├── lambda_invoker.py  # Lambda(RIE)へのHTTPリクエスト送信
    ├── grpc_provision.py  # Go Agent への gRPC プロビジョニング
    ├── function_registry.py # functions.yml 読み込み
    └── route_matcher.py   # routing.ymlベースのパスマッチング
```

#### 主要コンポーネント
| モジュール                                  | 責務                                                          |
| ------------------------------------------- | ------------------------------------------------------------- |
| `core/event_builder.py`                     | API Gateway Lambda Proxy Integration互換イベント構築          |
| `services/gateway/services/pool_manager.py` | コンテナのキャパシティ確保、プロビジョニング要求、返却管理    |
| `services/container_pool.py`                | 関数ごとの Condition 待ち行列管理とコンテナインスタンスの保持 |
| `services/janitor.py`                       | アイドル/孤児コンテナの整理                                   |
| `services/lambda_invoker.py`                | `httpx` を使用した Lambda RIE へのリクエスト送信              |
| `services/grpc_provision.py`                | Go Agent への gRPC 呼び出し                                   |

### 2.2 Go Agent (Internal)
- **役割**: Lambdaコンテナのライフサイクル管理（オンデマンド起動、削除、状態取得）。
- **通信**: Gateway からの gRPC リクエストにより containerd を操作。
- **主な RPC**:
    - `EnsureContainer`: コンテナ起動・Ready確認
    - `DestroyContainer`: コンテナ削除
    - `ListContainers`: 稼働中コンテナの状態取得（Janitor が利用）
    - `PauseContainer` / `ResumeContainer`: 将来的なウォームスタート向けの操作（未使用）

### 2.3 CoreDNS (Sidecar)
- **役割**: Lambda microVM/コンテナからの DNS クエリを解決し、論理名（`s3-storage`, `database` 等）を適切な IP へマッピングします。
- **ポート**: `53` (UDP/TCP, `10.88.0.1` で待ち受け)

### 2.4 RustFS (Storage)
- **役割**: AWS S3互換のオブジェクトストレージ。Lambdaコードやデータの保存に使用。
- **構成**:
    - **API**: ポート `9000` (S3互換)
    - **Console**: ポート `9001` (管理Web UI)
- **認証**: 環境変数でAccessKey/SecretKeyを設定。

### 2.5 ScyllaDB (Database)
- **役割**: Dockerコンテナ向けの高性能NoSQLデータベース。AWS DynamoDB互換API (Alternator) を提供。
- **ポート**: `8001` (Alternator API 外部公開用), `8000` (内部通信用)

### 2.6 VictoriaLogs
- **役割**: ログ収集・管理基盤。LambdaやGatewayのログを集約可。
- **ポート**: `9428` (Web UI/API)

## 3. ネットワーク仕様

Gateway は external_network 上で起動し、443 をホストに公開します。Agent は runtime-node の NetNS を共有し、runtime-node が 50051 を公開します。
分離構成では 50051（runtime-node/agent）は Compute 側に存在します。

| サービス名     | コンテナ内ポート | ホスト公開ポート | URL                           | プロトコル          |
| -------------- | ---------------- | ---------------- | ----------------------------- | ------------------- |
| Gateway API    | 443              | 443              | `https://localhost:443`       | HTTPS               |
| Agent gRPC     | 50051            | 50051            | `grpc://<compute-host>:50051` | gRPC                |
| CoreDNS        | 53               | なし             | `10.88.0.1:53`                | DNS (UDP/TCP)       |
| RustFS API     | 9000             | 9000             | `http://localhost:9000`       | HTTP                |
| RustFS Console | 9001             | 9001             | `http://localhost:9001`       | HTTP                |
| ScyllaDB       | 8000             | 8001             | `http://localhost:8001`       | HTTP (DynamoDB API) |
| VictoriaLogs   | 9428             | 9428             | `http://localhost:9428`       | HTTP                |

補足:
- 単一ノード構成では `docker-compose.containerd.yml` が runtime-node を external_network に参加させ、Gateway から `runtime-node:50051` で接続できます。
- 分離構成では Compute 側の 50051 を `AGENT_GRPC_ADDRESS` で指定します（例: `10.99.0.x:50051`）。

## 4. データ永続化

単一ノード構成/分離構成とも **named volume** を使用します。DinD 向けの bind mount 構成は現行の compose には含めていません。

- **named volume**:
    - `rustfs_data` -> RustFSデータ
    - `scylladb_data` -> ScyllaDBデータ
    - `victorialogs_data` -> ログデータ
    - `registry_data` -> レジストリデータ（Containerd/Firecracker のみ）

## 5. デプロイメントモデル

### 5.1 Compose ファイル構成

| ファイル                          | 役割                                   | 主な用途                                   |
| --------------------------------- | -------------------------------------- | ------------------------------------------ |
| `docker-compose.yml`              | Control/Core（Gateway + 依存サービス） | Control Plane（単一ノード/分離構成の共通） |
| **`docker-compose.registry.yml`** | **Registry**                           | Containerd/Firecracker モードで自動追加    |
| `docker-compose.node.yml`         | Compute（runtime-node/agent/coredns）  | Compute Node（Firecracker/remote）         |
| `docker-compose.containerd.yml`   | Adapter（単一ノード結合 / coredns）    | Core + Compute を同一ホストで統合          |

### 5.2 起動パターン（docker compose）

単一ノード（containerd）:
```bash
docker compose -f docker-compose.yml \
  -f docker-compose.registry.yml \
  -f docker-compose.node.yml \
  -f docker-compose.containerd.yml up -d
```

Control/Compute 分離（Firecracker）:
```bash
# Control
docker compose -f docker-compose.yml up -d

# Compute
docker compose -f docker-compose.node.yml up -d
```

注意:
- `docker compose -f` は指定順に合成され、後のファイルが前の内容を上書きします。
- パスは最初の `-f` のディレクトリ基準で解決されます（必要なら `--project-directory` を使用）。
- `esb up` は環境変数 `ESB_MODE` に応じて同じ組み合わせを自動選択します。
