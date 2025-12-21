# Sample DinD Lambda Simulator
Docker in Docker (DinD) 技術を活用した、オンプレミス環境向けのエッジサーバーレス・シミュレーターです。

## 特徴
- **Pure DinD**: 1つの親コンテナ内に Gateway, Storage, DB, Lambda 等の全サービスを集約。
- **Serverless-like**: Lambda関数はオンデマンドで起動し、アイドル時に自動停止（デフォルト5分）。
- **AWS API Gateway Compatibility**: `multiValueHeaders` や `multiValueQueryStringParameters` に対応した高度な互換性。
- **Direct Access**: リバースプロキシを排し、各サービスへHTTPポートで直接アクセス可能。
- **Hybrid Dev**: 開発用 (`docker-compose.yml`) と 本番用 (`docker-compose.dind.yml`) をシームレスに切り替え。

## アーキテクチャ

本システムは、セキュリティの向上とリソース管理の最適化のため、**API Gateway (Facade)** と **Container Manager (Orchestrator)** を分離したマイクロサービス構成を採用しています。

### 分離の意義とメリット
1. **セキュリティ（最小権限の原則）**:
   - `docker.sock` へのアクセス権を **Manager サービスのみ**に限定。
   - 外部に公開される `Gateway` は Docker 操作権限を持たないため、万一 Gateway が侵害されてもホスト全体の制御を奪われるリスクを低減します。
2. **信頼性と堅牢性**:
   - **ゾンビコンテナ対策 & 状態復元**: Manager が起動時に既存のコンテナ状態を Docker から同期（Adopt & Sync）し、実行中のコンテナは管理下に復帰、停止中のコンテナのみクリーンアップします。Manager 再起動時もサービス断を最小化します。
   - **アイドル監視**: バックグラウンドスケジューラーがアイドル状態のコンテナを常時監視・停止します（デフォルト5分）。
   - **関心の分離**: Gateway は認証とルーティングに、Manager はライフサイクル管理に専念することで、コードの肥大化と密結合を防止しています。
3. **パフォーマンス**:
   - Gateway は `httpx` による完全非同期プロキシとして動作し、多数の同時リクエストを効率的に処理します。
   - **コンテナホストキャッシュ**: Gateway はコンテナの起動状態を TTL 付き LRU キャッシュで保持し、Warm Start 時は Manager への問い合わせをスキップしてレイテンシを削減します。

### 構成図

```mermaid
graph TD
    Client["Client (HTTPS)"] --> Gateway["API Gateway (FastAPI)"]
    subgraph "Privileged Layer"
        Manager["Container Manager (CRUD API)"]
        Docker["Docker Engine (docker.sock)"]
    end
    Gateway -- "Invoke API (Async HTTP)" --> Manager
    Manager -- "Manage Lifecycle" --> Docker
    Gateway -- "Proxy Request (Async HTTP)" --> Lambda["Lambda RIE Container"]
    Lambda -- "Data Access" --> Storage["RustFS (S3)"]
    Lambda -- "Data Access" --> DB["ScyllaDB (DynamoDB)"]
    
    style Gateway fill:#f9f,stroke:#333,stroke-width:2px
    style Manager fill:#bbf,stroke:#333,stroke-width:2px
    style Docker fill:#dfd,stroke:#333,stroke-width:2px
```

### サービス一覧
ホストOS、またはDinD親コンテナ上で以下のサービス群が動作します。

| サービス           | ポート | 役割                         | URL                      |
| ------------------ | ------ | ---------------------------- | ------------------------ |
| **Gateway API**    | `443`  | 認証・ルーティング・プロキシ | `https://localhost:443`  |
| **Manager API**    | -      | コンテナの選定・起動・停止   | (内部通信のみ)           |
| **RustFS API**     | `9000` | S3互換オブジェクトストレージ | `http://localhost:9000`  |
| **ScyllaDB**       | `8001` | DynamoDB互換DB               | `http://localhost:8001`  |
| **VictoriaLogs**   | `9428` | ログ管理 Web UI              | `http://localhost:9428`  |

### ファイル構成
```
.
├── docker-compose.yml       # 複合サービスの構成定義
├── services/
│   ├── gateway/             # ステートレスな API ゲートウェイ
│   │   ├── main.py              # ルーティング & プロキシロジック
│   │   ├── client.py            # Manager 呼び出しクライアント
│   │   ├── config.py            # 設定管理
│   │   └── core/
│   │       ├── security.py      # JWT認証
│   │       └── proxy.py         # 非同期 httpx プロキシ実装
│   └── manager/             # ステートフルなコンテナオーケストレーター
│       ├── main.py              # ライフサイクル API & スケジューラー
│       └── service.py           # Docker Python SDK を用いた操作
├── gateway/config/          # Lambda 関数・ルート定義
│   ├── functions.yml        # Lambda 関数リスト
│   └── routing.yml          # パスベースのルーティング定義
├── lambda_functions/        # Lambda 関数コード
└── tests/                   # E2Eテスト & ユーティリティ
```

## クイックスタート

### 1. 開発モード (推奨)
ホスト上のDockerで直接サービス群を起動します。開発の反復に最適です。
```bash
# 起動
docker compose up -d

# 停止
docker compose down
```

#### 構成・ネットワーク設定（環境変数）
| 変数名 | デフォルト | 説明 |
|--------|-----------|------|
| `IDLE_TIMEOUT_MINUTES` | `5` | アイドル状態のLambdaコンテナを停止するまでの分数 |
| `LAMBDA_NETWORK` | `onpre-internal-network` | Lambdaコンテナが参加するDockerネットワーク名 |
| `CONTAINER_CACHE_TTL` | `30` | Gateway のコンテナホストキャッシュ TTL（秒） |


```bash
# 例: アイドルタイムアウトを15分に設定
IDLE_TIMEOUT_MINUTES=15 docker compose up -d
```

### 2. 本番/DinDモード
商用環境を模した、単一コンテナ(DinD)内で全サービスを起動します。
```bash
# 起動
docker compose -f docker-compose.dind.yml up -d

# ログ確認
docker logs -f onpre-app-root
```

## 開発ガイド

### 開発環境セットアップ
```bash
# 1. 仮想環境作成と依存関係インストール
uv venv
.venv\Scripts\activate  # Windows
# source .venv/bin/activate  # macOS/Linux
uv pip install -e ".[dev]"

# 2. Git hooks のセットアップ
cd ..
lefthook install
```

### テスト実行
E2EテストはDinD環境を起動して実行されます。
```bash
python tests/run_tests.py --build
```
※ `--build` オプションで再ビルド可能。

### API利用例 (Gateway)

**認証:**
```bash
curl -k -X POST https://localhost:443/user/auth/ver1.0 \
  -H "x-api-key: dev-api-key-change-in-production" \
  -H "Content-Type: application/json" \
  -d '{"AuthParameters": {"USERNAME": "onpremise-user", "PASSWORD": "onpremise-pass"}}'
```
> `-k` オプションは自己署名証明書を許可します。

**S3互換アクセス:**
AWS SDK等からは `http://localhost:9000` をエンドポイントとして指定してください。

## 詳細ドキュメント
- [システム仕様書](docs/spec.md)
- [クライアント認証仕様](docs/client-auth-spec.md)
- [ログ収集アダプター](docs/local-logging-adapter.md)
- [ネットワーク解決の最適化（IPベース）](docs/network-optimization.md)
- [コンテナキャッシュによるレイテンシ最適化](docs/container-cache.md)
- [Manager再起動時のコンテナ復元（Adopt & Sync）](docs/manager-restart-resilience.md)
