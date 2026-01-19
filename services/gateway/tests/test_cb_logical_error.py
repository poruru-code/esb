import re
from unittest.mock import AsyncMock, MagicMock

import httpx
import pytest
import respx

from services.gateway.config import GatewayConfig
from services.gateway.services.lambda_invoker import LambdaInvoker


@pytest.mark.asyncio
async def test_circuit_breaker_on_rie_200_error_FINAL():
    config = GatewayConfig(
        JWT_SECRET_KEY="test-secret-key-32-chars-long-!!!",
        X_API_KEY="test",
        AUTH_USER="test",
        AUTH_PASS="test",
        CONTAINERS_NETWORK="test",
        GATEWAY_INTERNAL_URL="http://test",
        CIRCUIT_BREAKER_THRESHOLD=2,
        CIRCUIT_BREAKER_RECOVERY_TIMEOUT=10.0,
    )

    mock_client = httpx.AsyncClient()
    registry = MagicMock()  # It's used synchronously in LambdaInvoker
    from services.gateway.models.function import FunctionEntity

    registry.get_function_config.return_value = FunctionEntity(
        name="test", image="test", environment={}
    )

    from services.common.models.internal import WorkerInfo

    backend = AsyncMock()
    mock_worker = WorkerInfo(id="w1", name="w1", ip_address="localhost", port=9000)
    backend.acquire_worker.return_value = mock_worker
    backend.release_worker = AsyncMock()
    backend.evict_worker = AsyncMock()

    invoker = LambdaInvoker(mock_client, registry, config, backend)

    # RIE often returns "200 but error" responses.
    error_body = {
        "errorType": "Runtime.ExitError",
        "errorMessage": "RequestId: xxx Error: Runtime exited with error: exit status 1",
    }

    with respx.mock:
        respx.post(url=re.compile(r".*/invocations")).mock(
            return_value=httpx.Response(200, json=error_body)
        )

        # First request (200 Error) -> should result in failure result.
        result1 = await invoker.invoke_function("test-func", b"{}")
        assert result1.success is False

        # Second request -> failure result.
        result2 = await invoker.invoke_function("test-func", b"{}")
        assert result2.success is False

        # Third request -> CircuitBreakerOpenError expected due to open circuit.
        # LambdaInvoker returns a failure result with specific error message
        result3 = await invoker.invoke_function("test-func", b"{}")
        assert result3.success is False
        assert result3.error is not None
        assert "Circuit is open" in result3.error
        print("\n✅ Circuit Breaker validated with logical 200 errors!")


if __name__ == "__main__":
    import asyncio

    asyncio.run(test_circuit_breaker_on_rie_200_error_FINAL())
