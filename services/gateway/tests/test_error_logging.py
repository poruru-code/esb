"""
Gateway error handling log detail tests.

Verify detailed info is logged at error level on Lambda connection failures.
"""

import logging
from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest

from services.gateway.api.deps import (
    get_http_client,
    get_lambda_invoker,
    get_orchestrator_client,
    resolve_lambda_target,
    verify_authorization,
)
from services.gateway.config import GatewayConfig
from services.gateway.models import TargetFunction
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.services.lambda_invoker import LambdaInvoker


@pytest.fixture
def mock_dependencies(main_app):
    """
    Fixture to set up shared mocked dependencies.
    """
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    mock_manager = AsyncMock()
    mock_manager.ensure_container.return_value = "test-container-host"
    mock_manager.invalidate_cache = MagicMock()

    # LambdaInvoker mock notes:
    # We can use the real invoker and mock the client, or mock the invoker itself.
    # Since we test main.py error handling, invoker raising exceptions is enough.
    # main.py catches errors in gateway_handler (catch-all route) or invoke_lambda_api.
    # This test hits `client.get("/test-path")`, so it uses gateway_handler.

    main_app.dependency_overrides[get_http_client] = lambda: mock_client
    main_app.dependency_overrides[get_orchestrator_client] = lambda: mock_manager

    # Auth & Routing Mocks
    async def mock_auth():
        return "test-user"

    async def mock_resolve(
        request,
    ):  # Accept request arg to match deps signature (or just return a value)
        # deps.py resolve_lambda_target takes request and route_matcher, but
        # dependency_overrides can use any signature (FastAPI resolves it).
        # main.py expects TargetFunction, so return that.
        return TargetFunction(
            container_name="test-container",
            path_params={},
            route_path="/test-path",
            function_config={},
        )

    main_app.dependency_overrides[verify_authorization] = mock_auth
    main_app.dependency_overrides[resolve_lambda_target] = mock_resolve

    yield mock_client, mock_manager

    main_app.dependency_overrides = {}


@pytest.mark.asyncio
async def test_lambda_connection_error_logged_at_error_level(main_app, async_client, caplog):
    """
    Verify Lambda connection failures are logged at error level.
    """
    # Capture logs from gateway.lambda_invoker where the error is now logged
    caplog.set_level(logging.ERROR, logger="gateway.lambda_invoker")
    logging.getLogger("gateway.lambda_invoker").error(
        "TEST_LOG: error-logging tests use mocked LambdaInvoker (no real containers)"
    )

    # Override dependencies
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    # Manager Mock
    mock_manager = AsyncMock()
    mock_manager.ensure_container.return_value = "test-container"
    mock_manager.invalidate_cache = MagicMock()

    # Create Invoker with mock client
    mock_registry = MagicMock(spec=FunctionRegistry)
    mock_registry.get_function_config.return_value = {"environment": {}}

    # Backend Mock
    mock_backend = AsyncMock()
    mock_worker = MagicMock()
    mock_worker.ip_address = "1.2.3.4"
    mock_backend.acquire_worker.return_value = mock_worker

    config = GatewayConfig()

    invoker = LambdaInvoker(mock_client, mock_registry, config, mock_backend)

    async def http_client_override():
        return mock_client

    async def orchestrator_override():
        return mock_manager

    async def lambda_invoker_override():
        return invoker

    main_app.dependency_overrides[get_http_client] = http_client_override
    main_app.dependency_overrides[get_orchestrator_client] = orchestrator_override
    main_app.dependency_overrides[get_lambda_invoker] = lambda_invoker_override
    from services.gateway.api.deps import get_processor
    from services.gateway.services.processor import GatewayRequestProcessor

    async def processor_override():
        return GatewayRequestProcessor(invoker, main_app.state.event_builder)

    main_app.dependency_overrides[get_processor] = processor_override

    async def auth_override() -> str:
        return "test-user"

    async def target_override() -> TargetFunction:
        return TargetFunction(
            container_name="test-container",
            path_params={},
            route_path="/test-path",
            function_config={},
        )

    main_app.dependency_overrides[verify_authorization] = auth_override
    main_app.dependency_overrides[resolve_lambda_target] = target_override

    # Trigger Lambda connection error via gateway_handler.
    mock_client.post.side_effect = httpx.ConnectError("Connection refused")
    await async_client.get("/test-path", headers={"Authorization": "Bearer valid-token"})

    # Clean up overrides
    main_app.dependency_overrides = {}

    # Assert: Error level log should exist
    assert any(
        record.levelname == "ERROR"
        and record.name == "gateway.lambda_invoker"
        and "Invocation error for" in record.message
        for record in caplog.records
    ), "Lambda connection error should be logged at ERROR level"


@pytest.mark.asyncio
async def test_lambda_connection_error_includes_detailed_info(main_app, async_client, caplog):
    """
    Verify logs include detailed info (host, port, timeout, error_detail) on connection failure.
    """
    from services.gateway.config import config

    caplog.set_level(logging.ERROR, logger="gateway.lambda_invoker")
    logging.getLogger("gateway.lambda_invoker").error(
        "TEST_LOG: error-logging tests use mocked LambdaInvoker (no real containers)"
    )

    # Override dependencies
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    mock_manager = AsyncMock()
    mock_manager.ensure_container.return_value = "192.168.1.100"
    mock_manager.invalidate_cache = MagicMock()

    mock_registry = MagicMock(spec=FunctionRegistry)
    mock_registry.get_function_config.return_value = {"environment": {}}

    # Backend Mock
    mock_backend = AsyncMock()
    mock_worker = MagicMock()
    mock_worker.ip_address = "192.168.1.100"
    mock_backend.acquire_worker.return_value = mock_worker

    invoker = LambdaInvoker(mock_client, mock_registry, config, mock_backend)

    async def http_client_override():
        return mock_client

    async def orchestrator_override():
        return mock_manager

    async def lambda_invoker_override():
        return invoker

    main_app.dependency_overrides[get_http_client] = http_client_override
    main_app.dependency_overrides[get_orchestrator_client] = orchestrator_override
    main_app.dependency_overrides[get_lambda_invoker] = lambda_invoker_override
    from services.gateway.api.deps import get_processor
    from services.gateway.services.processor import GatewayRequestProcessor

    async def processor_override():
        return GatewayRequestProcessor(invoker, main_app.state.event_builder)

    main_app.dependency_overrides[get_processor] = processor_override

    async def auth_override() -> str:
        return "test-user"

    async def target_override() -> TargetFunction:
        return TargetFunction(
            container_name="test-container",
            path_params={},
            route_path="/test-path",
            function_config={},
        )

    main_app.dependency_overrides[verify_authorization] = auth_override
    main_app.dependency_overrides[resolve_lambda_target] = target_override

    # Trigger Lambda connection error.
    mock_client.post.side_effect = httpx.ConnectTimeout("Timeout after 30s")
    await async_client.get("/test-path", headers={"Authorization": "Bearer valid-token"})

    # Clean up overrides
    main_app.dependency_overrides = {}

    # Assert: Log record should contain detailed info in extra fields
    error_records = [
        r
        for r in caplog.records
        if r.levelname == "ERROR"
        and r.name == "gateway.lambda_invoker"
        and "Invocation error for" in r.message
    ]
    if not error_records:
        print("\nCaptured Log Records:")
        for r in caplog.records:
            print(f"  Level: {r.levelname}, Message: {r.message}")
    assert len(error_records) > 0, "Should have at least one ERROR log"

    # Check for detailed fields in log record
    error_record = error_records[0]

    # LambdaInvoker logs: function_name, target_url, error_type, error_detail
    assert hasattr(error_record, "function_name"), "Log should include function_name"
    assert hasattr(error_record, "target_url"), "Log should include target_url"
    assert hasattr(error_record, "error_detail"), "Log should include error_detail"

    assert (
        error_record.function_name == "test-container"
    )  # container_name is passed as function_name
    assert "192.168.1.100" in error_record.target_url
    assert "Timeout" in error_record.error_detail
