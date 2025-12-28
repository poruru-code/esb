import pytest
import httpx
import respx
from unittest.mock import MagicMock, AsyncMock
from services.gateway.services.lambda_invoker import LambdaInvoker
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.config import GatewayConfig
from services.gateway.core.exceptions import LambdaExecutionError
from services.common.models.internal import WorkerInfo


@pytest.fixture
def mock_registry():
    registry = MagicMock(spec=FunctionRegistry)
    registry.get_function_config.return_value = {"image": "hello-world", "environment": {}}
    return registry


@pytest.fixture
def mock_backend():
    backend = AsyncMock()

    # function_name に応じて異なるホストを返すように設定
    async def side_effect(function_name, **kwargs):
        if function_name == "func-1":
            return WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1", port=9001)
        if function_name == "func-2":
            return WorkerInfo(id="c2", name="w2", ip_address="10.0.0.2", port=9002)
        return WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1", port=9001)

    backend.acquire_worker.side_effect = side_effect
    backend.release_worker = AsyncMock()
    backend.evict_worker = AsyncMock()
    return backend


@pytest.fixture
def gateway_config():
    return GatewayConfig(GATEWAY_INTERNAL_URL="http://gateway:8080", LAMBDA_PORT=8080)


@pytest.mark.asyncio
@respx.mock
async def test_invoker_circuit_breaker_opens(mock_registry, mock_backend, gateway_config):
    """Invokerが失敗を検知して回路を遮断することを確認"""
    async with httpx.AsyncClient() as client:
        invoker = LambdaInvoker(
            client=client,
            registry=mock_registry,
            config=gateway_config,
            backend=mock_backend,
        )

        function_name = "test-function"
        rie_url = "http://10.0.0.1:9001/2015-03-31/functions/function/invocations"

        # 常に失敗(500)を返す
        respx.post(rie_url).mock(return_value=httpx.Response(500))

        # CircuitBreaker が 5回失敗で OPEN になる（デフォルト）
        for _ in range(5):
            with pytest.raises(LambdaExecutionError):
                await invoker.invoke_function(function_name, b"{}")

        # 6回目は CircuitBreakerOpenError が発生し、LambdaExecutionError にラップされる
        with pytest.raises(LambdaExecutionError) as exc:
            await invoker.invoke_function(function_name, b"{}")

        # OPEN 状態であることを確認
        assert "Circuit Breaker Open" in str(exc.value)


@pytest.mark.asyncio
@respx.mock
async def test_invoker_per_function_breaker(mock_registry, mock_backend, gateway_config):
    """関数ごとに独立したブレーカーが機能することを確認"""
    async with httpx.AsyncClient() as client:
        invoker = LambdaInvoker(
            client=client,
            registry=mock_registry,
            config=gateway_config,
            backend=mock_backend,
        )

        f1 = "func-1"
        f2 = "func-2"

        # ホストが分かれているのでURLで識別可能
        url1 = "http://10.0.0.1:9001/2015-03-31/functions/function/invocations"
        url2 = "http://10.0.0.2:9002/2015-03-31/functions/function/invocations"

        respx.post(url1).mock(return_value=httpx.Response(500))
        respx.post(url2).mock(return_value=httpx.Response(200, content=b"ok"))

        # func-1 を遮断状態にする
        for _ in range(5):
            try:
                await invoker.invoke_function(f1, b"{}")
            except Exception:
                pass

        # func-1 は遮断されている
        with pytest.raises(LambdaExecutionError) as exc:
            await invoker.invoke_function(f1, b"{}")
        assert "Circuit Breaker Open" in str(exc.value)

        # func-2 は正常に動くはず
        resp = await invoker.invoke_function(f2, b"{}")
        assert resp.status_code == 200
