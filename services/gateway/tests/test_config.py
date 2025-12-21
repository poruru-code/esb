from services.gateway.config import GatewayConfig
from services.common.core.config import BaseAppConfig


def test_gateway_config_inheritance():
    """
    TDD: GatewayConfig should inherit from BaseAppConfig.
    """
    assert issubclass(GatewayConfig, BaseAppConfig)


def test_gateway_config_fields():
    """
    TDD: GatewayConfig should have MANAGER_URL and MANAGER_TIMEOUT.
    """
    config = GatewayConfig()
    assert hasattr(config, "MANAGER_URL")
    assert hasattr(config, "MANAGER_TIMEOUT")
    # Verify default values (optional, but good for regression)
    assert config.MANAGER_URL == "http://test-manager:8081"
    assert config.MANAGER_TIMEOUT == 30.0
