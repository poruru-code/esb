"""
Common Configuration
"""

from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field


class BaseAppConfig(BaseSettings):
    """
    アプリケーション共通設定
    """

    LOG_LEVEL: str = Field(default="INFO", description="ログレベル")

    # ===== Lambda Container Defaults =====
    LAMBDA_PORT: int = Field(default=8080, description="Lambda RIEコンテナのポート番号")
    READINESS_TIMEOUT: int = Field(
        default=30, description="コンテナReadinessチェックのタイムアウト(秒)"
    )
    DOCKER_DAEMON_TIMEOUT: int = Field(
        default=30, description="Docker Daemon起動待機のタイムアウト(秒)"
    )

    model_config = SettingsConfigDict(
        env_file=".env", env_file_encoding="utf-8", case_sensitive=True, extra="ignore"
    )
