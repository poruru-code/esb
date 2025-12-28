import pytest
from unittest.mock import MagicMock, AsyncMock
from services.gateway.services.lambda_invoker import LambdaInvoker
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.config import GatewayConfig
from typing import Any, Protocol


class InvocationBackend(Protocol):
    async def acquire_worker(self, function_name: str) -> Any: ...
    async def release_worker(self, function_name: str, worker: Any) -> None: ...
    async def evict_worker(self, function_name: str, worker: Any) -> None: ...


@pytest.mark.asyncio
async def test_lambda_invoker_strategy_initialization():
    """TDD RED: Test LambdaInvoker can be initialized with the new Strategy backend"""
    client = AsyncMock()
    registry = MagicMock(spec=FunctionRegistry)
    config = GatewayConfig()
    backend = AsyncMock(spec=InvocationBackend)

    # This call should fail with the current LambdaInvoker (old args).
    invoker = LambdaInvoker(client=client, registry=registry, config=config, backend=backend)

    assert invoker.backend == backend


@pytest.mark.asyncio
async def test_lambda_invoker_calls_backend_acquire():
    """TDD RED: Test LambdaInvoker calls backend.acquire_worker"""
    client = AsyncMock()
    registry = MagicMock(spec=FunctionRegistry)
    config = GatewayConfig()
    backend = AsyncMock(spec=InvocationBackend)

    mock_worker = MagicMock()
    mock_worker.ip_address = "1.2.3.4"
    mock_worker.port = 8080
    backend.acquire_worker.return_value = mock_worker

    invoker = LambdaInvoker(client, registry, config, backend)

    registry.get_function_config.return_value = {"image": "img"}

    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.content = b"{}"
    mock_response.headers = {}
    client.post.return_value = mock_response

    await invoker.invoke_function("test-func", b"{}")

    backend.acquire_worker.assert_called_once_with("test-func")
    backend.release_worker.assert_called_once_with("test-func", mock_worker)
