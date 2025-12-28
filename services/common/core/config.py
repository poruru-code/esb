"""
Common Configuration
"""

from pydantic_settings import BaseSettings, SettingsConfigDict
from pydantic import Field


class BaseAppConfig(BaseSettings):
    """
    Common application settings.
    """

    LOG_LEVEL: str = Field(default="INFO", description="Log level")
    VERIFY_SSL: bool = Field(default=False, description="Whether to verify SSL certificates")

    # ===== Lambda Container Defaults =====
    LAMBDA_PORT: int = Field(default=8080, description="Port number for Lambda RIE container")
    READINESS_TIMEOUT: int = Field(
        default=30, description="Timeout in seconds for container readiness checks"
    )
    DOCKER_DAEMON_TIMEOUT: int = Field(
        default=30, description="Timeout in seconds waiting for Docker daemon startup"
    )

    model_config = SettingsConfigDict(
        env_file=".env", env_file_encoding="utf-8", case_sensitive=True, extra="ignore"
    )
