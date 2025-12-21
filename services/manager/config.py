"""
Manager設定定義
"""

from pydantic import Field
from services.common.core.config import BaseAppConfig


class ManagerConfig(BaseAppConfig):
    """
    Managerサービスの設定管理
    """

    IDLE_TIMEOUT_MINUTES: int = Field(default=5, description="アイドルコンテナのタイムアウト(分)")
    CONTAINERS_NETWORK: str = Field(..., description="コンテナネットワーク名")


# シングルトンとして設定をロード
try:
    config = ManagerConfig()
except Exception as e:
    print(f"Failed to load Manager configuration: {e}")
    raise
