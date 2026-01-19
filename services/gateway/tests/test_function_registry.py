from unittest.mock import mock_open, patch

import pytest

from services.gateway.services.function_registry import FunctionRegistry


@pytest.fixture
def mock_functions_yaml():
    return """
defaults:
  environment:
    GLOBAL_ENV: "true"

functions:
  test-func:
    image: "test-image:latest"
    environment:
      FUNC_ENV: "123"
"""


def test_function_registry_load_success(mock_functions_yaml):
    with patch("builtins.open", mock_open(read_data=mock_functions_yaml)):
        with patch("services.gateway.config.config.FUNCTIONS_CONFIG_PATH", "dummy/path.yml"):
            registry = FunctionRegistry()
            registry.load_functions_config()

            config = registry.get_function_config("test-func")

            assert config is not None
            assert config.image == "test-image:latest"
            # Verify environment merging logic
            assert config.environment["GLOBAL_ENV"] == "true"
            assert config.environment["FUNC_ENV"] == "123"


def test_function_registry_get_nonexistent():
    with patch("builtins.open", mock_open(read_data="functions: {}")):
        with patch("services.gateway.config.config.FUNCTIONS_CONFIG_PATH", "dummy/path.yml"):
            registry = FunctionRegistry()
            registry.load_functions_config()
            assert registry.get_function_config("nonexistent") is None
