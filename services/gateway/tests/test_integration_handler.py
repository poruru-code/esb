import pytest
from unittest.mock import patch, AsyncMock, MagicMock
from fastapi.testclient import TestClient
from services.gateway.main import app
from services.gateway.core.exceptions import FunctionNotFoundError


# Mock dependencies
@pytest.fixture
def mock_proxy():
    with patch("services.gateway.main.proxy_to_lambda", new_callable=AsyncMock) as mock:
        # Define a side effect to verify arguments immediately and return a mock response
        def side_effect(container, event, client):
            print(f"DEBUG: Proxy called with Event: {event}")
            resp = MagicMock()
            resp.status_code = 200
            resp.json.return_value = {"statusCode": 200, "body": "ok"}
            return resp

        mock.side_effect = side_effect
        yield mock


@pytest.fixture
def mock_ensure_container():
    # Mock the CLASS so when main.py instantiates it, it gets a mock
    with patch("services.gateway.main.ManagerClient") as MockClass:
        instance = MockClass.return_value
        instance.ensure_container = AsyncMock(return_value="127.0.0.1")
        yield instance.ensure_container


@pytest.fixture
def mock_registry():
    # Mocking FunctionRegistry methods call inside RouteMatcher/etc if needed
    # But for catch-all route, we might need to mock dependencies injected via app.state
    # services/gateway/main.py initializes them in lifespan.
    # We might need to override app dependency overrides or state.
    pass


# Remove global client
# client = TestClient(app)


def test_gateway_handler_propagates_request_id(mock_proxy, mock_ensure_container):
    # Override dependencies
    from services.gateway.api.deps import verify_authorization, resolve_lambda_target
    from services.gateway.models import TargetFunction

    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="test-container",
        function_config={"image": "img", "environment": {}},
        path_params={},
        route_path="/api/s3/test",
    )

    trace_id = "integration-trace-id-999"
    headers = {"X-Request-Id": trace_id}

    # Execute with Context Manager to trigger startup/shutdown and use current mocks
    with TestClient(app) as client:
        response = client.post("/api/s3/test", headers=headers, json={"action": "test"})

    # Verify
    assert response.status_code == 200

    # Check what was passed to proxy_to_lambda
    assert mock_proxy.called
    args, kwargs = mock_proxy.call_args
    # call args: container_host, event, client=...
    event = args[1]

    assert event["requestContext"]["requestId"] == trace_id, (
        f"Expected {trace_id}, got {event['requestContext']['requestId']}"
    )

    # Clean up
    app.dependency_overrides = {}


def test_gateway_handler_returns_404_when_function_not_found(mock_proxy, mock_ensure_container):
    """
    FunctionNotFoundError が発生した場合に 404 を返すことを検証
    """
    # Override dependencies
    from services.gateway.api.deps import (
        verify_authorization,
        resolve_lambda_target,
        get_manager_client,
    )
    from services.gateway.models import TargetFunction

    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="missing-container",
        function_config={"image": "img", "environment": {}},
        path_params={},
        route_path="/api/missing",
    )

    # Mock ManagerClient to raise FunctionNotFoundError
    mock_manager = AsyncMock()
    mock_manager.ensure_container.side_effect = FunctionNotFoundError("missing-container")
    app.dependency_overrides[get_manager_client] = lambda: mock_manager

    # Execute
    from fastapi.testclient import TestClient

    client = TestClient(app)
    response = client.get("/api/missing", headers={"Authorization": "Bearer token"})

    # Verify
    assert response.status_code == 404
    assert response.json() == {"message": "Function not found on manager"}

    app.dependency_overrides = {}
