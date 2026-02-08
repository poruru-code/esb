from unittest.mock import AsyncMock, Mock

import pytest
from starlette.requests import Request

from services.gateway.api.deps import (
    get_processor,
    resolve_lambda_target,
    verify_authorization,
)
from services.gateway.main import gateway_handler
from services.gateway.models import TargetFunction
from services.gateway.models.context import InputContext
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


@pytest.mark.asyncio
async def test_gateway_handler_propagates_request_id(main_app, async_client, mock_processor):
    """Verify that GatewayRequestProcessor is called and context is passed."""

    # Override dependencies
    async def auth_override() -> str:
        return "test-user"

    async def target_override() -> TargetFunction:
        return TargetFunction(
            container_name="test-function",
            function_config={"environment": {}},
            path_params={},
            route_path="/api/test",
        )

    async def processor_override():
        return mock_processor

    main_app.dependency_overrides[verify_authorization] = auth_override
    main_app.dependency_overrides[resolve_lambda_target] = target_override
    main_app.dependency_overrides[get_processor] = processor_override

    response = await async_client.post("/api/test", json={"action": "test"})

    # Verify
    assert response.status_code == 200
    assert mock_processor.process_request.called

    # Check context passed to processor
    context = mock_processor.process_request.call_args[0][0]
    assert context.function_name == "test-function"
    assert context.user_id == "test-user"

    # Clean up
    main_app.dependency_overrides = {}


@pytest.mark.asyncio
async def test_gateway_handler_returns_error_result(main_app, async_client, mock_processor):
    """Verify that failed InvocationResult is handled correctly."""
    # Setup mock to return a failure
    mock_processor.process_request.return_value = InvocationResult(
        success=False,
        status_code=503,
        error="Service Unavailable",
    )

    async def auth_override() -> str:
        return "test-user"

    async def target_override() -> TargetFunction:
        return TargetFunction(
            container_name="test-function",
            function_config={"environment": {}},
            path_params={},
            route_path="/api/test",
        )

    async def processor_override():
        return mock_processor

    main_app.dependency_overrides[verify_authorization] = auth_override
    main_app.dependency_overrides[resolve_lambda_target] = target_override
    main_app.dependency_overrides[get_processor] = processor_override

    response = await async_client.get("/api/test")

    # Verify
    assert response.status_code == 503
    assert response.json() == {"message": "Service Unavailable"}

    main_app.dependency_overrides = {}


@pytest.mark.asyncio
async def test_gateway_handler_allows_patch(main_app, async_client, mock_processor):
    """Verify PATCH method is accepted by the catch-all handler."""

    async def auth_override() -> str:
        return "test-user"

    async def target_override() -> TargetFunction:
        return TargetFunction(
            container_name="test-function",
            function_config={"environment": {}},
            path_params={},
            route_path="/api/test",
        )

    async def processor_override():
        return mock_processor

    main_app.dependency_overrides[verify_authorization] = auth_override
    main_app.dependency_overrides[resolve_lambda_target] = target_override
    main_app.dependency_overrides[get_processor] = processor_override

    response = await async_client.patch("/api/test", json={"action": "patch"})

    assert response.status_code == 200
    assert mock_processor.process_request.called

    main_app.dependency_overrides = {}


@pytest.mark.asyncio
async def test_gateway_handler_head_falls_back_to_get_route(main_app, tmp_path, mock_processor):
    """Verify HEAD resolves to GET route and handler returns an empty body."""
    mock_processor.process_request.return_value = InvocationResult(
        success=True,
        status_code=200,
        payload=(b'{"statusCode": 200, "headers": {"Content-Type": "text/plain"}, "body": "ok"}'),
        headers={},
    )

    async def auth_override() -> str:
        return "test-user"

    async def processor_override():
        return mock_processor

    main_app.dependency_overrides[verify_authorization] = auth_override
    main_app.dependency_overrides[get_processor] = processor_override

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

    scope = {
        "type": "http",
        "asgi": {"version": "3.0"},
        "http_version": "1.1",
        "method": "HEAD",
        "path": "/api/test/123",
        "raw_path": b"/api/test/123",
        "query_string": b"",
        "headers": [],
        "client": ("testclient", 50000),
        "server": ("testserver", 80),
        "scheme": "http",
        "root_path": "",
        "app": main_app,
    }

    async def receive():
        return {"type": "http.request", "body": b"", "more_body": False}

    request = Request(scope, receive)
    target = await resolve_lambda_target(request, matcher)
    assert target.route_path == "/api/test/{id}"
    assert target.path_params == {"id": "123"}

    context = InputContext(
        function_name=target.container_name,
        method="HEAD",
        path="/api/test/123",
        headers={},
        multi_headers={},
        query_params={},
        multi_query_params={},
        body=b"",
        user_id="test-user",
        path_params=target.path_params,
        route_path=target.route_path,
        timeout=30.0,
    )
    response = await gateway_handler(context, mock_processor)

    assert response.status_code == 200
    assert response.body == b""
    assert response.headers.get("content-type") == "text/plain"
    assert context.method == "HEAD"
    assert context.path == "/api/test/123"
    assert context.route_path == "/api/test/{id}"


@pytest.mark.asyncio
async def test_cors_preflight_returns_headers(async_client):
    """Verify OPTIONS preflight returns CORS headers without auth."""
    response = await async_client.options(
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
