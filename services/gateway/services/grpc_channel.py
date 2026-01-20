"""
Where: services/gateway/services/grpc_channel.py
What: Helpers for creating Agent gRPC channels (TLS or insecure).
Why: Centralize channel creation logic for Gateway components.
"""

from pathlib import Path

import grpc
import grpc.aio as grpc_aio

from services.gateway.config import GatewayConfig


def create_agent_channel(agent_address: str, config: GatewayConfig) -> grpc_aio.Channel:
    if not config.AGENT_GRPC_TLS_ENABLED:
        return grpc_aio.insecure_channel(agent_address)

    ca_pem = Path(config.AGENT_GRPC_TLS_CA_CERT_PATH).read_bytes()
    cert_pem = Path(config.AGENT_GRPC_TLS_CERT_PATH).read_bytes()
    key_pem = Path(config.AGENT_GRPC_TLS_KEY_PATH).read_bytes()

    credentials = grpc.ssl_channel_credentials(
        root_certificates=ca_pem,
        private_key=key_pem,
        certificate_chain=cert_pem,
    )

    return grpc_aio.secure_channel(
        agent_address,
        credentials,
    )
