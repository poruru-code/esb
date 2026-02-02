from unittest.mock import Mock, mock_open, patch

import pytest

from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.services.route_matcher import RouteMatcher


@pytest.fixture
def mock_registry():
    registry = Mock(spec=FunctionRegistry)
    registry.get_function_config.return_value = {}
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
            assert config == {}
            mock_registry.get_function_config.assert_called_with("test-func")


def test_route_matcher_no_match(mock_registry):
    with patch("builtins.open", mock_open(read_data="routes: []")):
        with patch("services.gateway.config.config.ROUTING_CONFIG_PATH", "dummy/routes.yml"):
            matcher = RouteMatcher(mock_registry)
            matcher.load_routing_config()

            container, _, _, _ = matcher.match_route("/unknown", "GET")
            assert container is None


def test_route_matcher_invalid_yaml_keeps_previous(mock_registry):
    valid_yaml = """
routes:
  - path: "/hello"
    method: "GET"
    function: "test-func"
"""
    invalid_yaml = "routes: [\n  - path: /broken\n"
    valid_open = mock_open(read_data=valid_yaml)
    invalid_open = mock_open(read_data=invalid_yaml)
    with patch(
        "builtins.open",
        side_effect=[valid_open.return_value, invalid_open.return_value],
    ):
        with patch("services.gateway.config.config.ROUTING_CONFIG_PATH", "dummy/routes.yml"):
            matcher = RouteMatcher(mock_registry)
            matcher.load_routing_config()
            container, _, _, _ = matcher.match_route("/hello", "GET")
            assert container == "test-func"
            matcher.load_routing_config(force=True)
            container, _, _, _ = matcher.match_route("/hello", "GET")
            assert container == "test-func"
