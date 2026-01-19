from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest

from services.common.models.internal import WorkerInfo
from services.gateway.models.function import FunctionEntity
from services.gateway.services.lambda_invoker import LambdaInvoker


@pytest.mark.asyncio
async def test_invoke_function_success_isolation():
    client = AsyncMock(spec=httpx.AsyncClient)
    registry = MagicMock()
    config = MagicMock()
    backend = AsyncMock()

    # Mock configuration
    config.LAMBDA_PORT = 8080
    config.CIRCUIT_BREAKER_THRESHOLD = 5
    config.CIRCUIT_BREAKER_RECOVERY_TIMEOUT = 30

    registry.get_function_config.return_value = {"image": "test-image", "environment": {}}

    worker = WorkerInfo(id="w1", name="w1", ip_address="127.0.0.1", port=8080)
    backend.acquire_worker.return_value = worker

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 200
    mock_response.content = b'{"result": "ok"}'
    mock_response.headers = {}
    client.post.return_value = mock_response

    invoker = LambdaInvoker(client, registry, config, backend)

    response = await invoker.invoke_function("test-fn", b"payload")

    assert response.status_code == 200
    backend.acquire_worker.assert_called_once_with("test-fn")
    backend.release_worker.assert_called_once_with("test-fn", worker)


@pytest.mark.asyncio
async def test_invoke_function_backend_failure_isolation():
    client = AsyncMock(spec=httpx.AsyncClient)
    registry = MagicMock()
    config = MagicMock()
    backend = AsyncMock()

    config.LAMBDA_PORT = 8080

    registry.get_function_config.return_value = {"image": "test-image"}
    backend.acquire_worker.side_effect = Exception("Provisioning failed")

    invoker = LambdaInvoker(client, registry, config, backend)

    result = await invoker.invoke_function("test-fn", b"payload")

    assert result.success is False
    assert result.status_code == 503  # ContainerStartError maps to 503
    assert result.error and "Provisioning failed" in result.error
    backend.release_worker.assert_not_called()


@pytest.mark.asyncio
async def test_invoke_function_http_failure_triggers_evict():
    client = AsyncMock(spec=httpx.AsyncClient)
    registry = MagicMock()
    config = MagicMock()
    backend = AsyncMock()

    config.LAMBDA_PORT = 8080
    config.CIRCUIT_BREAKER_THRESHOLD = 5
    config.CIRCUIT_BREAKER_RECOVERY_TIMEOUT = 30

    registry.get_function_config.return_value = FunctionEntity(name="test-fn", image="test-image")
    worker = WorkerInfo(id="w1", name="w1", ip_address="127.0.0.1", port=8080)
    backend.acquire_worker.return_value = worker

    # Simulate connection error
    client.post.side_effect = httpx.ConnectError("Connection refused")

    invoker = LambdaInvoker(client, registry, config, backend)

    result = await invoker.invoke_function("test-fn", b"payload")

    assert result.success is False
    assert result.status_code == 502
    # Verify eviction on connection error
    backend.evict_worker.assert_called_once_with("test-fn", worker)
    # Ensure release is NOT called (because handled in handle_error/finally logic)
    # In my new implementation, I use worker_evicted flag.
    backend.release_worker.assert_not_called()
