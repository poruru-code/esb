"""
Gateway configuration definition.

Loads configuration from environment variables and provides a Pydantic model.
Uses pydantic-settings for type safety and defaults.
"""

import os
import sys

from pydantic import Field

from services.common.core.config import BaseAppConfig


class ServiceDefaults:
    """Default values for internal services to avoid magic numbers."""

    PROTOCOL = "http"
    S3_PORT = 9000
    DYNAMODB_PORT = 8000  # Alternator internal port (now standard for all modes)
    VICTORIALOGS_PORT = 9428


class GatewayConfig(BaseAppConfig):
    """
    Configuration management for the Gateway service.
    """

    # Server settings
    UVICORN_WORKERS: int = Field(default=4, description="Number of worker processes")
    UVICORN_BIND_ADDR: str = Field(default="0.0.0.0:8000", description="Listen address")

    # Path settings
    RUNTIME_CONFIG_DIR: str = Field(
        default="/app/runtime-config",
        description="Directory for runtime config files (esb deploy target)",
    )
    SEED_CONFIG_DIR: str = Field(
        default="/app/seed-config", description="Directory for seed config files (initial fallback)"
    )
    ROUTING_CONFIG_PATH: str = Field(
        default="/app/runtime-config/routing.yml", description="Routing definition file path"
    )
    FUNCTIONS_CONFIG_PATH: str = Field(
        default="/app/runtime-config/functions.yml",
        description="Lambda function definition file path",
    )
    RESOURCES_CONFIG_PATH: str = Field(
        default="/app/runtime-config/resources.yml", description="Resources definition file path"
    )
    CONFIG_RELOAD_ENABLED: bool = Field(
        default=True, description="Enable hot reload of config files"
    )
    CONFIG_RELOAD_INTERVAL: float = Field(
        default=1.0, description="Config reload interval (seconds), minimum 0.5"
    )
    CONFIG_RELOAD_LOCK_TIMEOUT: float = Field(
        default=5.0, description="Lock timeout for config reload (seconds)"
    )

    SSL_CERT_PATH: str = Field(default="/app/config/ssl/server.crt", description="SSL cert path")
    SSL_KEY_PATH: str = Field(default="/app/config/ssl/server.key", description="SSL key path")
    DATA_ROOT_PATH: str = Field(default="/data", description="Root path for child container data")
    LOGS_ROOT_PATH: str = Field(default="/logs", description="Root path for log aggregation")
    VICTORIALOGS_URL: str = Field(default="", description="VictoriaLogs ingestion URL (Gateway)")
    GATEWAY_VICTORIALOGS_URL: str = Field(
        default="", description="VictoriaLogs ingestion URL (Lambda Injection)"
    )
    # Payload logging
    LOG_PAYLOADS: bool = Field(default=False, description="Log request/response payloads (verbose)")

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
    GATEWAY_OWNER_ID: str = Field(
        default_factory=lambda: os.getenv("HOSTNAME") or "gateway",
        description="Gateway owner identifier for Agent resource ownership",
    )
    DATA_PLANE_HOST: str = Field(default="", description="Host for data plane services")
    LAMBDA_INVOKE_TIMEOUT: float = Field(
        default=30.0, description="Lambda invoke timeout (seconds)"
    )

    # Data store endpoints for injection into Lambda
    DYNAMODB_ENDPOINT: str = Field(default="", description="Internal DynamoDB endpoint")
    S3_ENDPOINT: str = Field(default="", description="Internal S3 endpoint")
    S3_PRESIGN_ENDPOINT: str = Field(
        default="",
        description="Public S3 endpoint used only for presigned URL generation in workers",
    )

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
    ENABLE_CONTAINER_PAUSE: bool = Field(
        default=False, description="アイドル後にコンテナを一時停止するか"
    )
    PAUSE_IDLE_SECONDS: int = Field(default=30, description="Pauseまでのアイドル秒数")
    ORPHAN_GRACE_PERIOD_SECONDS: int = Field(
        default=60, description="Grace period before removing orphan containers (seconds)"
    )

    # Phase 1: Agent Settings
    AGENT_GRPC_ADDRESS: str = Field(default="agent:50051", description="Agent gRPC address")
    AGENT_INVOKE_PROXY: bool = Field(
        default=False, description="Invoke workers via Agent (L7 proxy) instead of direct IP"
    )
    AGENT_GRPC_TLS_ENABLED: bool = Field(
        default=False, description="Enable mTLS for Agent gRPC connections"
    )
    AGENT_GRPC_TLS_CA_CERT_PATH: str = Field(
        default="/app/config/ssl/rootCA.crt", description="Agent gRPC CA cert path"
    )
    AGENT_GRPC_TLS_CERT_PATH: str = Field(
        default="/app/config/ssl/client.crt", description="Agent gRPC client cert path"
    )
    AGENT_GRPC_TLS_KEY_PATH: str = Field(
        default="/app/config/ssl/client.key", description="Agent gRPC client key path"
    )

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
