import pytest
from pydantic_settings import BaseSettings


def test_base_app_config_structure():
    """
    TDD: BaseAppConfig should exist and have LOG_LEVEL field.
    """
    try:
        from services.common.core.config import BaseAppConfig
    except ImportError:
        pytest.fail("BaseAppConfig could not be imported")

    assert issubclass(BaseAppConfig, BaseSettings)

    config = BaseAppConfig()
    assert hasattr(config, "LOG_LEVEL")
    assert config.LOG_LEVEL in ["INFO", "DEBUG"]


def test_base_app_config_env_file():
    """
    TDD: BaseAppConfig should verify env_file settings (model_config).
    """
    from services.common.core.config import BaseAppConfig

    assert BaseAppConfig.model_config.get("env_file") == ".env"
    assert BaseAppConfig.model_config.get("extra") == "ignore"
