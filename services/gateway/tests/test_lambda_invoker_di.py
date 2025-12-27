import pytest
from unittest.mock import MagicMock, AsyncMock, patch
from services.gateway.services.lambda_invoker import LambdaInvoker
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.config import GatewayConfig


@pytest.mark.asyncio
async def test_lambda_invoker_di_initialization():
    """Test LambdaInvoker can be initialized with dependencies"""
    client = AsyncMock()
    registry = MagicMock(spec=FunctionRegistry)
    backend = AsyncMock()  # Protocol mock
    config = GatewayConfig()

    invoker = LambdaInvoker(client=client, registry=registry, config=config, backend=backend)

    assert invoker.client == client
    assert invoker.registry == registry
    assert invoker.backend == backend
    assert invoker.config == config


@pytest.mark.asyncio
async def test_lambda_invoker_invoke_flow():
    """Test invoke_function uses injected dependencies"""
    # Arrange
    client = AsyncMock()
    registry = MagicMock(spec=FunctionRegistry)
    backend = AsyncMock()
    config = GatewayConfig()
    config.GATEWAY_INTERNAL_URL = "http://gateway-internal"

    invoker = LambdaInvoker(client, registry, config, backend)

    function_name = "test-func"
    payload = b"{}"

    # Mock Registry
    registry.get_function_config.return_value = {
        "image": "test-image",
        "environment": {"VAR": "VAL"},
    }

    # Mock Backend
    mock_worker = MagicMock()
    mock_worker.ip_address = "10.0.0.5"
    backend.acquire_worker.return_value = mock_worker

    # Mock HTTP Client - return valid JSON response (not an error)
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.content = b'{"statusCode": 200, "body": "OK"}'
    mock_response.json.return_value = {"statusCode": 200, "body": "OK"}
    mock_response.headers = {}
    client.post.return_value = mock_response

    # Act
    await invoker.invoke_function(function_name, payload)

    # Assert
    # 1. Registry called
    registry.get_function_config.assert_called_with(function_name)

    # 2. Backend called with correct args
    backend.acquire_worker.assert_called_once_with(function_name)
    backend.release_worker.assert_called_once_with(function_name, mock_worker)

    # 3. HTTP Client called
    expected_url = f"http://10.0.0.5:{config.LAMBDA_PORT}/2015-03-31/functions/function/invocations"
    client.post.assert_called_once()
    assert client.post.call_args[0][0] == expected_url


@pytest.mark.asyncio
async def test_lambda_invoker_logging_on_error():
    """Test LambdaInvoker logs errors with extra context"""
    from services.gateway.core.exceptions import LambdaExecutionError
    import httpx

    client = AsyncMock()
    registry = MagicMock(spec=FunctionRegistry)
    backend = AsyncMock()
    config = GatewayConfig()

    invoker = LambdaInvoker(client, registry, config, backend)

    # Setup mocks
    registry.get_function_config.return_value = {"image": "img", "environment": {}}
    mock_worker = MagicMock()
    mock_worker.ip_address = "host"
    backend.acquire_worker.return_value = mock_worker
    client.post.side_effect = httpx.RequestError("Connection failed")

    with patch("services.gateway.services.lambda_invoker.logger") as mock_logger:
        with pytest.raises(LambdaExecutionError):
            await invoker.invoke_function("error-func", b"{}")

        mock_logger.error.assert_called_once()
        call_args = mock_logger.error.call_args
        assert "function_name" in call_args.kwargs["extra"]
        assert call_args.kwargs["extra"]["function_name"] == "error-func"


@pytest.mark.asyncio
async def test_lambda_invoker_always_uses_pool_backend():
    """
    Step 2: ENABLE_CONTAINER_POOLING が False でも PoolManager が注入されることを検証
    """
    from services.gateway.main import app
    from services.gateway.services.pool_manager import PoolManager
    from fastapi.testclient import TestClient

    # lifespan 内での外部呼び出しをすべて抑制するために patch する
    with (
        patch("services.gateway.main.config") as mock_config,
        patch(
            "services.gateway.services.pool_manager.PoolManager.sync_with_manager",
            new_callable=AsyncMock,
        ),
        patch("services.gateway.services.janitor.HeartbeatJanitor.start", new_callable=AsyncMock),
    ):
        # 他の必要なコンフィグを設定
        mock_config.LAMBDA_INVOKE_TIMEOUT = 30
        mock_config.ORCHESTRATOR_URL = "http://test"
        mock_config.LAMBDA_PORT = 8080
        mock_config.ORCHESTRATOR_TIMEOUT = 30
        mock_config.DEFAULT_MAX_CAPACITY = 10
        mock_config.DEFAULT_MIN_CAPACITY = 0
        mock_config.POOL_ACQUIRE_TIMEOUT = 30.0
        mock_config.HEARTBEAT_INTERVAL = 30
        mock_config.GATEWAY_IDLE_TIMEOUT_SECONDS = 300

        # TestClient を使うことで lifespan を実行させる
        with TestClient(app) as _:
            invoker = app.state.lambda_invoker
            # backend が PoolManager のインスタンスであることを確認
            assert isinstance(invoker.backend, PoolManager)

            # LegacyBackendAdapter (があれば) が使われていないことを確認
            try:
                from services.gateway.services.legacy_adapter import LegacyBackendAdapter

                assert not isinstance(invoker.backend, LegacyBackendAdapter)
            except ImportError:
                # 既に削除されている場合はパス
                pass
