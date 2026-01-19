"""
Where: services/gateway/services/grpc_channel.py
What: Helpers for creating Agent gRPC channels (TLS or insecure).
Why: Centralize channel creation logic for Gateway components.
"""

from pathlib import Path

import grpc

from services.gateway.config import GatewayConfig


def create_agent_channel(agent_address: str, config: GatewayConfig) -> grpc.aio.Channel:
    if not config.AGENT_GRPC_TLS_ENABLED:
        return grpc.aio.insecure_channel(agent_address)  # ty: ignore[possibly-missing-attribute]

    ca_pem = Path(config.AGENT_GRPC_TLS_CA_CERT_PATH).read_bytes()
    cert_pem = Path(config.AGENT_GRPC_TLS_CERT_PATH).read_bytes()
    key_pem = Path(config.AGENT_GRPC_TLS_KEY_PATH).read_bytes()

    credentials = grpc.ssl_channel_credentials(
        root_certificates=ca_pem,
        private_key=key_pem,
        certificate_chain=cert_pem,
    )

    return grpc.aio.secure_channel(  # ty: ignore[possibly-missing-attribute]
        agent_address,
        credentials,
    )
