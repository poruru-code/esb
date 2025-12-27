"""
Gateway設定定義

環境変数から設定をロードし、Pydanticモデルとして提供します。
pydantic-settings を使用して型安全性とデフォルト値を管理します。
"""

import sys
from pydantic import Field
from services.common.core.config import BaseAppConfig


class GatewayConfig(BaseAppConfig):
    """
    Gatewayサービスの設定管理
    """

    # サーバー設定
    UVICORN_WORKERS: int = Field(default=4, description="ワーカープロセス数")
    UVICORN_BIND_ADDR: str = Field(default="0.0.0.0:8000", description="リッスンアドレス")

    # パス設定
    ROUTING_CONFIG_PATH: str = Field(
        default="/app/config/routing.yml", description="ルーティング定義ファイルパス"
    )
    FUNCTIONS_CONFIG_PATH: str = Field(
        default="/app/config/functions.yml", description="Lambda関数定義ファイルパス"
    )
    SSL_CERT_PATH: str = Field(default="/app/config/ssl/server.crt", description="SSL証明書パス")
    SSL_KEY_PATH: str = Field(default="/app/config/ssl/server.key", description="SSL秘密鍵パス")
    DATA_ROOT_PATH: str = Field(default="/data", description="子コンテナデータのルートパス")
    LOGS_ROOT_PATH: str = Field(default="/logs", description="ログ集約先のルートパス")

    # 認証・セキュリティ (必須 - 環境変数から設定)
    JWT_SECRET_KEY: str = Field(..., min_length=32, description="JWT署名用シークレットキー")
    JWT_EXPIRES_DELTA: int = Field(default=3000, description="トークン有効期限(秒)")
    # x-api-key は静的なダミー認証キー
    X_API_KEY: str = Field(..., description="内部サービス間通信用APIキー")

    # モックユーザー認証情報 (必須 - 環境変数から設定)
    AUTH_USER: str = Field(..., description="認証ユーザー名")
    AUTH_PASS: str = Field(..., description="認証パスワード")

    # 認証エンドポイント
    AUTH_ENDPOINT_PATH: str = Field(default="/user/auth/v1", description="認証エンドポイントパス")

    # 外部連携 (接続先ホスト名は必須 - 環境変数から設定)
    CONTAINERS_NETWORK: str = Field(..., description="Lambdaコンテナの所属ネットワーク")
    GATEWAY_INTERNAL_URL: str = Field(..., description="コンテナから見たGateway URL")
    ORCHESTRATOR_URL: str = Field(..., description="OrchestratorサービスURL")
    ORCHESTRATOR_TIMEOUT: float = Field(
        default=30.0, description="Orchestrator通信タイムアウト(秒)"
    )
    LAMBDA_INVOKE_TIMEOUT: float = Field(default=30.0, description="Lambda呼び出しタイムアウト(秒)")

    # サーキットブレーカー設定
    CIRCUIT_BREAKER_THRESHOLD: int = Field(default=5, description="失敗しきい値")
    CIRCUIT_BREAKER_RECOVERY_TIMEOUT: float = Field(
        default=30.0, description="復旧試行までの待機時間(秒)"
    )

    # Auto-Scaling
    DEFAULT_MAX_CAPACITY: int = Field(default=1, description="デフォルト最大容量")
    DEFAULT_MIN_CAPACITY: int = Field(default=0, description="デフォルト最小容量")
    POOL_ACQUIRE_TIMEOUT: float = Field(default=30.0, description="ワーカー取得タイムアウト")
    HEARTBEAT_INTERVAL: int = Field(default=30, description="Heartbeat送信間隔(秒)")
    GATEWAY_IDLE_TIMEOUT_SECONDS: int = Field(
        default=300, description="Gateway側アイドルタイムアウト(秒)"
    )

    # Phase 1: Go Agent Settings
    AGENT_GRPC_ADDRESS: str = Field(default="esb-agent:50051", description="Go Agent gRPC address")
    USE_GRPC_AGENT: bool = Field(default=False, description="Whether to use Go Agent via gRPC")

    # FastAPI設定
    root_path: str = Field(default="", description="APIのルートパス（プロキシ用）")

    # model_config is inherited


# シングルトンとして設定をロード
# pydantic-settings はインスタンス化時に環境変数を読み込む
try:
    config = GatewayConfig()
except Exception as e:
    # 開発環境など .env がない場合や必須変数が欠けている場合のフォールバックは
    # 必要に応じて検討するが、基本はエラーで落とす
    sys.stderr.write(f"Failed to load configuration: {e}\n")
    # テスト実行時などのために、一部デフォルトで許容する場合のロジックを入れることも可能
    raise
