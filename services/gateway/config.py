"""
Gateway configuration definition.

Loads configuration from environment variables and provides a Pydantic model.
Uses pydantic-settings for type safety and defaults.
"""

import sys
from pydantic import Field
from services.common.core.config import BaseAppConfig


class GatewayConfig(BaseAppConfig):
    """
    Configuration management for the Gateway service.
    """

    # Server settings
    UVICORN_WORKERS: int = Field(default=4, description="Number of worker processes")
    UVICORN_BIND_ADDR: str = Field(default="0.0.0.0:8000", description="Listen address")

    # Path settings
    ROUTING_CONFIG_PATH: str = Field(
        default="/app/config/routing.yml", description="Routing definition file path"
    )
    FUNCTIONS_CONFIG_PATH: str = Field(
        default="/app/config/functions.yml", description="Lambda function definition file path"
    )
    SSL_CERT_PATH: str = Field(default="/app/config/ssl/server.crt", description="SSL cert path")
    SSL_KEY_PATH: str = Field(default="/app/config/ssl/server.key", description="SSL key path")
    DATA_ROOT_PATH: str = Field(default="/data", description="Root path for child container data")
    LOGS_ROOT_PATH: str = Field(default="/logs", description="Root path for log aggregation")
    VICTORIALOGS_URL: str = Field(default="", description="VictoriaLogs ingestion URL")

    # Authentication/security (required from env)
    JWT_SECRET_KEY: str = Field(..., min_length=32, description="JWT signing secret key")
    JWT_EXPIRES_DELTA: int = Field(default=3000, description="Token expiry (seconds)")
    # x-api-key is a static dummy auth key
    X_API_KEY: str = Field(..., description="API key for internal service communication")

    # Mock user credentials (required from env)
    AUTH_USER: str = Field(..., description="Auth username")
    AUTH_PASS: str = Field(..., description="Auth password")

    # Auth endpoint
    AUTH_ENDPOINT_PATH: str = Field(default="/user/auth/v1", description="Auth endpoint path")

    # External integration (hostnames required from env)
    CONTAINERS_NETWORK: str = Field(..., description="Network for Lambda containers")
    GATEWAY_INTERNAL_URL: str = Field(..., description="Gateway URL from containers")
    LAMBDA_INVOKE_TIMEOUT: float = Field(default=30.0, description="Lambda invoke timeout (seconds)")

    # Flow control (Phase 4-1)
    MAX_CONCURRENT_REQUESTS: int = Field(default=10, description="Max concurrent per function")
    QUEUE_TIMEOUT_SECONDS: int = Field(default=10, description="Queue wait timeout")

    # Circuit breaker settings
    CIRCUIT_BREAKER_THRESHOLD: int = Field(default=5, description="Failure threshold")
    CIRCUIT_BREAKER_RECOVERY_TIMEOUT: float = Field(
        default=30.0, description="Wait time before recovery attempt (seconds)"
    )

    # Auto-Scaling
    DEFAULT_MAX_CAPACITY: int = Field(default=1, description="Default max capacity")
    DEFAULT_MIN_CAPACITY: int = Field(default=0, description="Default min capacity")
    POOL_ACQUIRE_TIMEOUT: float = Field(default=30.0, description="Worker acquisition timeout")
    HEARTBEAT_INTERVAL: int = Field(default=30, description="Heartbeat interval (seconds)")
    GATEWAY_IDLE_TIMEOUT_SECONDS: int = Field(
        default=300, description="Gateway idle timeout (seconds)"
    )
    ORPHAN_GRACE_PERIOD_SECONDS: int = Field(
        default=60, description="Grace period before removing orphan containers (seconds)"
    )

    # Phase 1: Go Agent Settings
    AGENT_GRPC_ADDRESS: str = Field(default="esb-agent:50051", description="Go Agent gRPC address")
    USE_GRPC_AGENT: bool = Field(default=False, description="Whether to use Go Agent via gRPC")

    # FastAPI settings
    root_path: str = Field(default="", description="API root path (for proxy)")

    # model_config is inherited


# Load config as a singleton.
# pydantic-settings reads environment variables during instantiation.
try:
    config = GatewayConfig()
except Exception as e:
    # Consider fallback for missing .env or required vars in dev environments,
    # but default to failing fast.
    sys.stderr.write(f"Failed to load configuration: {e}\n")
    # For tests, optional logic could allow some defaults if needed.
    raise
