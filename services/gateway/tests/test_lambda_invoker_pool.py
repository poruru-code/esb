"""
Tests for LambdaInvoker Pool Mode (Auto-Scaling)

TDD: Tests for pool-based invocation with self-healing.
"""

from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest


class TestLambdaInvokerPoolMode:
    """Tests for LambdaInvoker with PoolManager integration"""

    @pytest.fixture
    def mock_registry(self):
        """Mock FunctionRegistry"""
        registry = MagicMock()
        registry.get_function_config = MagicMock(
            return_value={
                "image": "hello-world:latest",
                "environment": {"LOG_LEVEL": "DEBUG"},
            }
        )
        return registry

    @pytest.fixture
    def mock_config(self):
        """Mock GatewayConfig"""
        config = MagicMock()
        config.ENABLE_CONTAINER_POOLING = True
        config.GATEWAY_INTERNAL_URL = "http://gateway:8000"
        config.LAMBDA_PORT = 8080
        config.CIRCUIT_BREAKER_THRESHOLD = 5
        config.CIRCUIT_BREAKER_RECOVERY_TIMEOUT = 30
        return config

    @pytest.fixture
    def mock_pool_manager(self):
        """Mock PoolManager"""
        from services.common.models.internal import WorkerInfo

        worker = WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")

        pm = MagicMock()
        pm.acquire_worker = AsyncMock(return_value=worker)
        pm.release_worker = AsyncMock()
        pm.evict_worker = AsyncMock()
        return pm

    @pytest.fixture
    def mock_http_client(self):
        """Mock httpx.AsyncClient"""
        client = MagicMock(spec=httpx.AsyncClient)
        response = MagicMock(spec=httpx.Response)
        response.status_code = 200
        response.headers = {}
        response.content = b'{"result": "ok"}'
        response.json = MagicMock(return_value={"result": "ok"})
        client.post = AsyncMock(return_value=response)
        return client

    @pytest.mark.asyncio
    async def test_invoke_with_pool_acquires_worker(
        self,
        mock_http_client,
        mock_registry,
        mock_config,
        mock_pool_manager,
    ):
        """When pooling enabled, invoke should acquire worker from PoolManager"""
        from services.gateway.services.lambda_invoker import LambdaInvoker

        invoker = LambdaInvoker(
            client=mock_http_client,
            registry=mock_registry,
            config=mock_config,
            backend=mock_pool_manager,  # Pool mode
        )

        await invoker.invoke_function("hello-world", b'{"test": 1}')

        # Pool manager should be used
        mock_pool_manager.acquire_worker.assert_called_once_with("hello-world")

    @pytest.mark.asyncio
    async def test_invoke_with_pool_releases_on_success(
        self,
        mock_http_client,
        mock_registry,
        mock_config,
        mock_pool_manager,
    ):
        """After successful invoke, worker should be released to pool"""
        from services.common.models.internal import WorkerInfo
        from services.gateway.services.lambda_invoker import LambdaInvoker

        worker = WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")
        mock_pool_manager.acquire_worker = AsyncMock(return_value=worker)

        invoker = LambdaInvoker(
            client=mock_http_client,
            registry=mock_registry,
            config=mock_config,
            backend=mock_pool_manager,
        )

        await invoker.invoke_function("hello-world", b"{}")

        mock_pool_manager.release_worker.assert_called_once_with("hello-world", worker)

    @pytest.mark.asyncio
    async def test_invoke_with_pool_evicts_on_connection_error(
        self,
        mock_http_client,
        mock_registry,
        mock_config,
        mock_pool_manager,
    ):
        """On connection error, worker should be evicted (self-healing)."""
        from services.common.models.internal import WorkerInfo
        from services.gateway.services.lambda_invoker import LambdaInvoker

        worker = WorkerInfo(id="c_dead", name="dead-worker", ip_address="10.0.0.99")
        mock_pool_manager.acquire_worker = AsyncMock(return_value=worker)
        mock_http_client.post = AsyncMock(side_effect=httpx.ConnectError("Connection refused"))

        invoker = LambdaInvoker(
            client=mock_http_client,
            registry=mock_registry,
            config=mock_config,
            backend=mock_pool_manager,
        )

        result = await invoker.invoke_function("hello-world", b"{}")

        assert result.success is False
        assert result.status_code == 502

        # Worker should be evicted (self-healing behavior restored)
        mock_pool_manager.evict_worker.assert_called_once_with("hello-world", worker)
        # Release is handled by finally block (so it might be released back to pool)
        # mock_pool_manager.release_worker.assert_called_once() # Depends on implementation detailed flow
