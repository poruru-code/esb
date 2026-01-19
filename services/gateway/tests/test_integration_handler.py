from unittest.mock import AsyncMock

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
        function_config={"image": "img", "environment": {}},
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
        function_config={"image": "img", "environment": {}},
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
