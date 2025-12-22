import pytest
from unittest.mock import MagicMock, AsyncMock
from services.gateway.services.lambda_invoker import LambdaInvoker
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.config import GatewayConfig


@pytest.mark.asyncio
async def test_lambda_invoker_di_initialization():
    """Test LambdaInvoker can be initialized with dependencies"""
    client = AsyncMock()
    registry = MagicMock(spec=FunctionRegistry)
    container_manager = AsyncMock()  # Protocol mock
    config = GatewayConfig()

    invoker = LambdaInvoker(
        client=client, registry=registry, container_manager=container_manager, config=config
    )

    assert invoker.client == client
    assert invoker.registry == registry
    assert invoker.container_manager == container_manager
    assert invoker.config == config


@pytest.mark.asyncio
async def test_lambda_invoker_invoke_flow():
    """Test invoke_function uses injected dependencies"""
    # Arrange
    client = AsyncMock()
    registry = MagicMock(spec=FunctionRegistry)
    container_manager = AsyncMock()
    config = GatewayConfig()
    config.GATEWAY_INTERNAL_URL = "http://gateway-internal"

    invoker = LambdaInvoker(client, registry, container_manager, config)

    function_name = "test-func"
    payload = b"{}"

    # Mock Registry
    registry.get_function_config.return_value = {
        "image": "test-image",
        "environment": {"VAR": "VAL"},
    }

    # Mock Container Manager
    container_manager.get_lambda_host.return_value = "10.0.0.5"

    # Mock HTTP Client
    client.post.return_value = MagicMock(status_code=200, content=b"result")

    # Act
    await invoker.invoke_function(function_name, payload)

    # Assert
    # 1. Registry called
    registry.get_function_config.assert_called_with(function_name)

    # 2. Container Manager called with correct args
    container_manager.get_lambda_host.assert_called_once()
    args, kwargs = container_manager.get_lambda_host.call_args
    # get_lambda_host(function_name, image, env)
    assert kwargs.get("function_name") == function_name or args[0] == function_name

    # Verify environment extension
    called_env = kwargs.get("env") or args[2]
    assert called_env["VAR"] == "VAL"
    assert called_env["GATEWAY_INTERNAL_URL"] == "http://gateway-internal"

    # 3. HTTP Client called
    expected_url = f"http://10.0.0.5:{config.LAMBDA_PORT}/2015-03-31/functions/function/invocations"
    client.post.assert_called_once()
    assert client.post.call_args[0][0] == expected_url
