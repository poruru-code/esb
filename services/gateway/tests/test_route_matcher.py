from unittest.mock import Mock, mock_open, patch

import pytest

from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.services.route_matcher import RouteMatcher


@pytest.fixture
def mock_registry():
    registry = Mock(spec=FunctionRegistry)
    registry.get_function_config.return_value = {"image": "test-env:latest"}
    return registry


@pytest.fixture
def mock_routes_yaml():
    return """
routes:
  - path: "/api/test/{id}"
    method: "POST"
    function: "test-func"
"""


def test_route_matcher_match_success(mock_registry, mock_routes_yaml):
    with patch("builtins.open", mock_open(read_data=mock_routes_yaml)):
        with patch("services.gateway.config.config.ROUTING_CONFIG_PATH", "dummy/routes.yml"):
            matcher = RouteMatcher(mock_registry)
            matcher.load_routing_config()

            container, path_params, route_path, config = matcher.match_route(
                "/api/test/123", "POST"
            )

            assert container == "test-func"
            assert path_params == {"id": "123"}
            assert route_path == "/api/test/{id}"
            assert config == {"image": "test-env:latest"}
            mock_registry.get_function_config.assert_called_with("test-func")


def test_route_matcher_no_match(mock_registry):
    with patch("builtins.open", mock_open(read_data="routes: []")):
        with patch("services.gateway.config.config.ROUTING_CONFIG_PATH", "dummy/routes.yml"):
            matcher = RouteMatcher(mock_registry)
            matcher.load_routing_config()

            container, _, _, _ = matcher.match_route("/unknown", "GET")
            assert container is None
