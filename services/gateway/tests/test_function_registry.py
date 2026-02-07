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
            # Verify environment merging logic
            assert config.environment["GLOBAL_ENV"] == "true"
            assert config.environment["FUNC_ENV"] == "123"


def test_function_registry_get_nonexistent():
    with patch("builtins.open", mock_open(read_data="functions: {}")):
        with patch("services.gateway.config.config.FUNCTIONS_CONFIG_PATH", "dummy/path.yml"):
            registry = FunctionRegistry()
            registry.load_functions_config()
            assert registry.get_function_config("nonexistent") is None


def test_function_registry_accepts_image_entry():
    valid_yaml = """
defaults:
  environment:
    GLOBAL_ENV: "true"

functions:
  test-func:
    environment:
      FUNC_ENV: "123"
"""
    image_yaml = """
functions:
  test-func:
    image: "registry:5010/example/repo:latest"
    environment:
      FUNC_ENV: "456"
"""
    valid_open = mock_open(read_data=valid_yaml)
    image_open = mock_open(read_data=image_yaml)
    with patch(
        "builtins.open",
        side_effect=[valid_open.return_value, image_open.return_value],
    ):
        with patch("services.gateway.config.config.FUNCTIONS_CONFIG_PATH", "dummy/path.yml"):
            registry = FunctionRegistry()
            registry.load_functions_config()
            config_before = registry.get_function_config("test-func")
            assert config_before is not None
            registry.load_functions_config(force=True)
            config_after = registry.get_function_config("test-func")
            assert config_after is not None
            assert config_after.environment["FUNC_ENV"] == "456"
            assert config_after.image == "registry:5010/example/repo:latest"
