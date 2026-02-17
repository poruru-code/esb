from unittest.mock import AsyncMock, Mock

import pytest

from services.gateway.api.deps import get_function_registry, get_lambda_invoker
from services.gateway.models.result import InvocationResult

TEST_ACCOUNT_ID = "123456789012"
TEST_REGION = "us-east-1"
TEST_FUNCTION_NAME = "lambda-callback-sample"


@pytest.mark.asyncio
async def test_invoke_route_normalizes_full_arn_to_function_name(main_app, async_client):
    mock_registry = Mock()
    mock_registry.get_function_config.return_value = {}

    mock_invoker = AsyncMock()
    mock_invoker.invoke_function = AsyncMock(
        return_value=InvocationResult(success=True, status_code=200, payload=b'{"ok": true}')
    )

    main_app.dependency_overrides[get_function_registry] = lambda: mock_registry
    main_app.dependency_overrides[get_lambda_invoker] = lambda: mock_invoker

    arn = f"arn:aws:lambda:{TEST_REGION}:{TEST_ACCOUNT_ID}:function:{TEST_FUNCTION_NAME}"
    response = await async_client.post(
        f"/2015-03-31/functions/{arn}/invocations",
        content=b'{"message":"hello"}',
    )

    assert response.status_code == 200
    mock_registry.get_function_config.assert_called_once_with(TEST_FUNCTION_NAME)
    mock_invoker.invoke_function.assert_awaited_once()

    args, kwargs = mock_invoker.invoke_function.await_args
    assert args[0] == TEST_FUNCTION_NAME
    assert args[1] == b'{"message":"hello"}'
    assert kwargs["timeout"] > 0

    main_app.dependency_overrides = {}


@pytest.mark.asyncio
async def test_invoke_route_returns_404_for_unregistered_arn_function(main_app, async_client):
    mock_registry = Mock()
    mock_registry.get_function_config.return_value = None

    mock_invoker = AsyncMock()
    mock_invoker.invoke_function = AsyncMock()

    main_app.dependency_overrides[get_function_registry] = lambda: mock_registry
    main_app.dependency_overrides[get_lambda_invoker] = lambda: mock_invoker

    arn = f"arn:aws:lambda:{TEST_REGION}:{TEST_ACCOUNT_ID}:function:missing-function"
    response = await async_client.post(f"/2015-03-31/functions/{arn}/invocations", content=b"{}")

    assert response.status_code == 404
    assert response.json()["message"] == "Function not found: missing-function"
    mock_invoker.invoke_function.assert_not_called()

    main_app.dependency_overrides = {}
