"""
設定管理モジュール

環境変数から設定値を読み込み、Pydanticモデルとして管理します。
"""

import os
from pydantic import BaseModel, Field


class GatewayConfig(BaseModel):
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

    # 認証・セキュリティ
    JWT_SECRET_KEY: str = Field(
        default="dev-secret-key-change-in-production", description="JWT署名用シークレット"
    )
    JWT_EXPIRES_DELTA: int = Field(default=3600, description="JWTトークン有効期間(秒)")
    # App Settings
    app_host: str = "0.0.0.0"
    app_port: int = 8000
    debug: bool = False
    root_path: str = ""
    AUTH_USER: str = Field(default="onpremise-user", description="認証対象ユーザ名")
    AUTH_PASS: str = Field(default="onpremise-pass", description="認証対象パスワード")
    AUTH_ENDPOINT_PATH: str = Field(
        default="/user/auth/ver1.0", description="内部認証エンドポイントパス"
    )
    X_API_KEY: str = Field(
        default="dev-api-key-change-in-production", description="Gateway認証用APIキー"
    )

    # RustFS設定 (パススルー用ドキュメント)
    RUSTFS_DEDUPLICATION: bool = Field(default=True, description="重複排除有効化")
    RUSTFS_COMPRESSION: str = Field(default="auto", description="圧縮モード")
    RUSTFS_LIFECYCLE_POLICY_PATH: str = Field(
        default="/app/config/lifecycle.yml", description="ライフサイクルポリシーパス"
    )

    # ScyllaDB設定 (パススルー用ドキュメント)
    SCYLLADB_HOST: str = Field(default="onpre-database", description="ScyllaDBホスト")
    SCYLLADB_PORT: int = Field(default=8000, description="Alternator APIポート")
    SCYLLADB_MEMORY: int = Field(default=1, description="メモリ割当(GiB)")


def load_config() -> GatewayConfig:
    """
    環境変数から設定を読み込む
    """
    # 環境変数から値を取得して辞書を作成
    # デフォルト値はPydanticモデル側で管理するため、取得できたものだけ渡す
    env_vars = {}
    for field_name in GatewayConfig.model_fields.keys():
        val = os.getenv(field_name)
        if val is not None:
            # bool型の処理
            if GatewayConfig.model_fields[field_name].annotation is bool:
                env_vars[field_name] = val.lower() in ("true", "1", "yes")
            else:
                env_vars[field_name] = val

    return GatewayConfig(**env_vars)


# シングルトンインスタンス
config = load_config()
