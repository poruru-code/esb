import pytest
import json
from unittest.mock import AsyncMock
from fastapi.testclient import TestClient
from services.gateway.main import app
from services.gateway.core.exceptions import FunctionNotFoundError
from services.gateway.api.deps import (
    verify_authorization,
    resolve_lambda_target,
    get_lambda_invoker,
    get_orchestrator_client,
)
from services.gateway.models import TargetFunction


@pytest.fixture
def mock_invoker():
    from httpx import Response

    invoker = AsyncMock()
    # Setup default response logic
    invoker.invoke_function.return_value = Response(
        status_code=200,
        headers={"Content-Type": "application/json"},
        content=b'{"statusCode": 200, "body": "ok"}',
    )
    return invoker


@pytest.fixture
def mock_ensure_container():
    # Mock the CLASS so when main.py instantiates it, it gets a mock
    # Wait, we inject ManagerClient via DI, so we should override get_orchestrator_client in tests,
    # OR rely on app.state.manager_client being mocked?
    # In test_error_logging we override dependency. Here let's override too.
    # But this fixture was using patch on main.py imports.
    # We should switch to dependency overrides for consistency.
    pass


def test_gateway_handler_propagates_trace_id(mock_invoker):
    # Override dependencies
    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="test-container",
        function_config={"image": "img", "environment": {}},
        path_params={},
        route_path="/api/s3/test",
    )
    app.dependency_overrides[get_lambda_invoker] = lambda: mock_invoker

    # Mock manager client just in case (though unused if invoker is mocked)
    mock_manager = AsyncMock()
    app.dependency_overrides[get_orchestrator_client] = lambda: mock_manager

    trace_id = "Root=1-integration-999-abcdef0123456789abcdef01;Sampled=1"
    headers = {"X-Amzn-Trace-Id": trace_id}

    # Execute with Context Manager to trigger startup/shutdown and use current mocks
    with TestClient(app) as client:
        response = client.post("/api/s3/test", headers=headers, json={"action": "test"})

    # Verify
    assert response.status_code == 200

    # Check what was passed to invoker.invoke_function
    assert mock_invoker.invoke_function.called
    args, kwargs = mock_invoker.invoke_function.call_args
    # call args: container_name, payload (bytes)
    function_name = args[0]
    payload_bytes = args[1]

    assert function_name == "test-container"

    # Parse payload (JSON event)
    event = json.loads(payload_bytes)

    import uuid

    # Request ID should now be a UUID, NOT the Trace Root
    req_id = event["requestContext"]["requestId"]
    expected_root_id = "1-integration-999-abcdef0123456789abcdef01"

    assert req_id != expected_root_id, "Request ID should be independent of Trace ID"
    try:
        uuid.UUID(req_id)
    except ValueError:
        pytest.fail(f"Request ID is not a valid UUID: {req_id}")

    # Clean up
    app.dependency_overrides = {}


def test_gateway_handler_returns_404_when_function_not_found():
    """
    Should FunctionNotFoundError result in 503/404?
    In main.py, FunctionNotFoundError is caught?
    main.py catches FunctionNotFoundError (added in import list).
    But where is it raised?
    resolve_lambda_target raises HTTPException(404).
    If invoke_function raises it?
    LambdaInvoker doesn't raise FunctionNotFoundError usually.
    But let's assume we simulate a case where manager client raises it or something.
    Ah, the original test mocked ensure_container to raise FunctionNotFoundError.
    But now ensure_container is called by Invoker.
    Invoker catches ContainerStartError.
    Does invoker catch FunctionNotFoundError?
    LambdaInvoker.invoke_function doesn't seem to explicitly catch FunctionNotFoundError.
    Core exceptions handler (global_exception_handler or http_exception_handler) should catch it.

    Let's simulate FunctionNotFoundError via resolve_lambda_target OR invoker.
    The original test tested "Function not found on manager".
    Let's have invoker raise FunctionNotFoundError.
    """

    # Setup dependencies
    mock_invoker = AsyncMock()
    mock_invoker.invoke_function.side_effect = FunctionNotFoundError("missing-container")

    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="missing-container",
        function_config={"image": "img", "environment": {}},
        path_params={},
        route_path="/api/missing",
    )
    app.dependency_overrides[get_lambda_invoker] = lambda: mock_invoker

    with TestClient(app) as client:
        response = client.get("/api/missing", headers={"Authorization": "Bearer token"})

    # Verify
    # FunctionNotFoundError is mapped to 404 in exception handlers?
    # services/gateway/core/exceptions.py says: FunctionNotFoundError -> 404.
    assert response.status_code == 404
    assert response.json() == {"message": "Function not found: missing-container"}

    app.dependency_overrides = {}
