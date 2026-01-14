"""
Where: services/gateway/tests/test_config_defaults.py
What: Validate default GatewayConfig values for critical flags.
Why: Keep config defaults stable as environment defaults evolve.
"""

from services.gateway.config import GatewayConfig


def _set_required_env(monkeypatch) -> None:
    monkeypatch.setenv("JWT_SECRET_KEY", "test-secret-key-must-be-very-long-for-security")
    monkeypatch.setenv("X_API_KEY", "test-api-key")
    monkeypatch.setenv("AUTH_USER", "test-user")
    monkeypatch.setenv("AUTH_PASS", "test-pass")
    monkeypatch.setenv("CONTAINERS_NETWORK", "test-net")
    monkeypatch.setenv("GATEWAY_INTERNAL_URL", "http://test-gateway:8000")


def test_agent_invoke_proxy_default_false(monkeypatch):
    _set_required_env(monkeypatch)
    monkeypatch.delenv("AGENT_INVOKE_PROXY", raising=False)

    config = GatewayConfig(_env_file=None)

    assert config.AGENT_INVOKE_PROXY is False
