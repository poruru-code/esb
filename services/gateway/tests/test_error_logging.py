"""
Gateway エラーハンドリングのログ詳細化テスト

Lambda接続失敗時に適切なログレベル（error）で詳細情報が記録されることを検証
"""

import pytest
from unittest.mock import AsyncMock, patch
import httpx
import logging


@pytest.mark.asyncio
async def test_lambda_connection_error_logged_at_error_level(caplog):
    """
    Lambda接続失敗時にerrorレベルでログされることを検証

    TDD Red Phase: このテストは現在失敗する（warningレベルでログされているため）
    """
    from services.gateway.main import app
    from services.gateway.services.function_registry import FunctionRegistry
    from services.gateway.services.route_matcher import RouteMatcher
    from services.gateway.services.lambda_invoker import LambdaInvoker
    from services.gateway.client import ManagerClient

    # Setup
    caplog.set_level(logging.ERROR, logger="gateway.main")

    # Mock dependencies
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    # Simulate httpx.RequestError on Lambda invocation
    mock_client.post.side_effect = httpx.ConnectError("Connection refused")

    # Initialize app state
    app.state.http_client = mock_client
    app.state.function_registry = FunctionRegistry()
    app.state.route_matcher = RouteMatcher(app.state.function_registry)
    app.state.lambda_invoker = LambdaInvoker(mock_client, app.state.function_registry)
    app.state.manager_client = ManagerClient(mock_client)

    # Load test config
    app.state.function_registry.load_functions_config()

    # Mock manager client to return a container host
    app.state.manager_client.ensure_container = AsyncMock(return_value="test-container")

    # Mock dependency injection for resolve_lambda_target and verify_authorization
    from services.gateway.api.deps import resolve_lambda_target, verify_authorization
    from services.gateway.models.schemas import TargetFunction

    async def mock_resolve_lambda_target():
        return TargetFunction(
            container_name="test-container",
            path_params={},
            route_path="/test-path",
            function_config={"image": "test-image"},
        )

    async def mock_verify_authorization():
        return "test-user"

    app.dependency_overrides[resolve_lambda_target] = mock_resolve_lambda_target
    app.dependency_overrides[verify_authorization] = mock_verify_authorization

    from fastapi.testclient import TestClient

    client = TestClient(app)

    # Trigger Lambda connection error via gateway_handler
    # This should log at ERROR level with detailed info
    with patch("services.gateway.main.build_event", return_value={}):
        with patch("services.gateway.main.proxy_to_lambda") as mock_proxy:
            mock_proxy.side_effect = httpx.ConnectError("Connection refused")

            client.get("/test-path", headers={"Authorization": "Bearer valid-token"})

    # Clean up overrides
    app.dependency_overrides = {}

    # Assert: Error level log should exist
    assert any(
        record.levelname == "ERROR" and "Lambda connection failed" in record.message
        for record in caplog.records
    ), "Lambda connection error should be logged at ERROR level"


@pytest.mark.asyncio
async def test_lambda_connection_error_includes_detailed_info(caplog):
    """
    Lambda接続失敗時のログに詳細情報（host, port, timeout, error_detail）が含まれることを検証

    TDD Red Phase: このテストは現在失敗する（詳細情報が含まれていないため）
    """
    from services.gateway.main import app
    from services.gateway.services.function_registry import FunctionRegistry
    from services.gateway.services.route_matcher import RouteMatcher
    from services.gateway.services.lambda_invoker import LambdaInvoker
    from services.gateway.client import ManagerClient

    # Setup
    caplog.set_level(logging.ERROR, logger="gateway.main")

    # Mock dependencies
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    # Initialize app state
    app.state.http_client = mock_client
    app.state.function_registry = FunctionRegistry()
    app.state.route_matcher = RouteMatcher(app.state.function_registry)
    app.state.lambda_invoker = LambdaInvoker(mock_client, app.state.function_registry)
    app.state.manager_client = ManagerClient(mock_client)

    # Load test config
    app.state.function_registry.load_functions_config()

    # Mock manager to return specific host
    app.state.manager_client.ensure_container = AsyncMock(return_value="192.168.1.100")

    # Mock dependency injection for resolve_lambda_target and verify_authorization
    from services.gateway.api.deps import resolve_lambda_target, verify_authorization
    from services.gateway.models.schemas import TargetFunction

    async def mock_resolve_lambda_target():
        return TargetFunction(
            container_name="test-container",
            path_params={},
            route_path="/test-path",
            function_config={"image": "test-image"},
        )

    async def mock_verify_authorization():
        return "test-user"

    app.dependency_overrides[resolve_lambda_target] = mock_resolve_lambda_target
    app.dependency_overrides[verify_authorization] = mock_verify_authorization

    from fastapi.testclient import TestClient

    client = TestClient(app)

    # Trigger Lambda connection error
    with patch("services.gateway.main.build_event", return_value={}):
        with patch("services.gateway.main.proxy_to_lambda") as mock_proxy:
            mock_proxy.side_effect = httpx.ConnectTimeout("Timeout after 30s")

            client.get("/test-path", headers={"Authorization": "Bearer valid-token"})

    # Clean up overrides
    app.dependency_overrides = {}

    # Assert: Log record should contain detailed info in extra fields
    error_records = [r for r in caplog.records if r.levelname == "ERROR"]
    if not error_records:
        print("\nCaptured Log Records:")
        for r in caplog.records:
            print(f"  Level: {r.levelname}, Message: {r.message}")
    assert len(error_records) > 0, "Should have at least one ERROR log"

    # Check for detailed fields in log record
    error_record = error_records[0]
    assert hasattr(error_record, "container_host"), "Log should include container_host"
    assert hasattr(error_record, "port"), "Log should include port"
    assert hasattr(error_record, "timeout"), "Log should include timeout"
    assert hasattr(error_record, "error_detail"), "Log should include error_detail"
    assert hasattr(error_record, "error_type"), "Log should include error_type"

    # Verify values
    from services.gateway.config import config

    assert error_record.container_host == "192.168.1.100"
    assert error_record.port == config.LAMBDA_PORT
    assert error_record.timeout == 30.0
    assert "Timeout" in error_record.error_detail
