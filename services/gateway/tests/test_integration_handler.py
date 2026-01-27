from unittest.mock import AsyncMock, Mock

import pytest
from fastapi.testclient import TestClient

from services.gateway.api.deps import (
    get_processor,
    resolve_lambda_target,
    verify_authorization,
)
from services.gateway.main import app
from services.gateway.models import TargetFunction
from services.gateway.models.result import InvocationResult
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.services.route_matcher import RouteMatcher


@pytest.fixture
def mock_processor():
    processor = AsyncMock()
    # Setup default response logic using InvocationResult
    processor.process_request.return_value = InvocationResult(
        success=True,
        status_code=200,
        payload=b'{"statusCode": 200, "body": "ok"}',
        headers={"Content-Type": "application/json"},
    )
    return processor


def test_gateway_handler_propagates_request_id(mock_processor):
    """Verify that GatewayRequestProcessor is called and context is passed."""
    # Override dependencies
    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="test-function",
        function_config={"environment": {}},
        path_params={},
        route_path="/api/test",
    )
    app.dependency_overrides[get_processor] = lambda: mock_processor

    with TestClient(app) as client:
        response = client.post("/api/test", json={"action": "test"})

    # Verify
    assert response.status_code == 200
    assert mock_processor.process_request.called

    # Check context passed to processor
    context = mock_processor.process_request.call_args[0][0]
    assert context.function_name == "test-function"
    assert context.user_id == "test-user"

    # Clean up
    app.dependency_overrides = {}


def test_gateway_handler_returns_error_result(mock_processor):
    """Verify that failed InvocationResult is handled correctly."""
    # Setup mock to return a failure
    mock_processor.process_request.return_value = InvocationResult(
        success=False,
        status_code=503,
        error="Service Unavailable",
    )

    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="test-function",
        function_config={"environment": {}},
        path_params={},
        route_path="/api/test",
    )
    app.dependency_overrides[get_processor] = lambda: mock_processor

    with TestClient(app) as client:
        response = client.get("/api/test")

    # Verify
    assert response.status_code == 503
    assert response.json() == {"message": "Service Unavailable"}

    app.dependency_overrides = {}


def test_gateway_handler_allows_patch(mock_processor):
    """Verify PATCH method is accepted by the catch-all handler."""
    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="test-function",
        function_config={"environment": {}},
        path_params={},
        route_path="/api/test",
    )
    app.dependency_overrides[get_processor] = lambda: mock_processor

    with TestClient(app) as client:
        response = client.patch("/api/test", json={"action": "patch"})

    assert response.status_code == 200
    assert mock_processor.process_request.called

    app.dependency_overrides = {}


def test_gateway_handler_head_falls_back_to_get_route(tmp_path, mock_processor):
    """Verify HEAD resolves to a GET route and returns an empty body."""
    mock_processor.process_request.return_value = InvocationResult(
        success=True,
        status_code=200,
        payload=(b'{"statusCode": 200, "headers": {"Content-Type": "text/plain"}, "body": "ok"}'),
        headers={},
    )

    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[get_processor] = lambda: mock_processor

    routing_path = tmp_path / "routing.yml"
    routing_path.write_text(
        """
routes:
  - path: "/api/test/{id}"
    method: "GET"
    function: "test-func"
""",
        encoding="utf-8",
    )

    registry = Mock(spec=FunctionRegistry)
    registry.get_function_config.return_value = {}
    matcher = RouteMatcher(registry)
    matcher.config_path = str(routing_path)
    matcher.load_routing_config()

    with TestClient(app) as client:
        original_matcher = app.state.route_matcher
        app.state.route_matcher = matcher
        response = client.head("/api/test/123")
        app.state.route_matcher = original_matcher

    assert response.status_code == 200
    assert response.content == b""
    assert response.headers.get("content-type") == "text/plain"

    context = mock_processor.process_request.call_args[0][0]
    assert context.method == "HEAD"
    assert context.path == "/api/test/123"
    assert context.route_path == "/api/test/{id}"

    app.dependency_overrides = {}


def test_cors_preflight_returns_headers():
    """Verify OPTIONS preflight returns CORS headers without auth."""
    with TestClient(app) as client:
        response = client.options(
            "/any/path",
            headers={
                "Origin": "https://example.com",
                "Access-Control-Request-Method": "POST",
                "Access-Control-Request-Headers": "Authorization,Content-Type",
            },
        )

    assert response.status_code == 204
    assert response.headers.get("access-control-allow-origin") == "https://example.com"
    assert "POST" in response.headers.get("access-control-allow-methods", "")
    assert response.headers.get("access-control-allow-headers") == "Authorization,Content-Type"
