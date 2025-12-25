"""
Manager設定定義
"""

import sys
from pydantic import Field
from services.common.core.config import BaseAppConfig


class ManagerConfig(BaseAppConfig):
    """
    Managerサービスの設定管理
    """

    IDLE_TIMEOUT_MINUTES: int = Field(default=5, description="アイドルコンテナのタイムアウト(分)")
    CONTAINERS_NETWORK: str = Field(..., description="コンテナネットワーク名")
    CONTAINER_READINESS_TIMEOUT: float = Field(
        default=30.0, description="コンテナ起動待機タイムアウト(秒)"
    )
    PING_TIMEOUT: float = Field(default=1.0, description="コンテナPing確認タイムアウト(秒)")
    READINESS_POLL_INTERVAL: float = Field(
        default=0.5, description="コンテナReady確認のポーリング間隔(秒)"
    )

    # Docker操作の安全性確保用設定
    DOCKER_MAX_WORKERS: int = Field(
        default=20, description="Docker操作用スレッドプールの最大ワーカー数"
    )
    DOCKER_CLIENT_TIMEOUT: int = Field(
        default=60, description="Dockerクライアントの通信タイムアウト(秒)"
    )


# シングルトンとして設定をロード
try:
    config = ManagerConfig()
except Exception as e:
    sys.stderr.write(f"Failed to load Manager configuration: {e}\n")
    raise
