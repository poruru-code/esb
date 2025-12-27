"""
Gateway エラーハンドリングのログ詳細化テスト

Lambda接続失敗時に適切なログレベル（error）で詳細情報が記録されることを検証
"""

import pytest
from unittest.mock import AsyncMock, MagicMock
import httpx
import logging
from services.gateway.api.deps import (
    get_http_client,
    get_orchestrator_client,
    get_lambda_invoker,
    verify_authorization,
    resolve_lambda_target,
)
from services.gateway.models import TargetFunction
from services.gateway.services.lambda_invoker import LambdaInvoker
from services.gateway.services.function_registry import FunctionRegistry
from services.gateway.config import GatewayConfig


@pytest.fixture
def mock_dependencies(main_app):
    """
    共通のモック依存関係をセットアップするフィクスチャ
    """
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    mock_manager = AsyncMock()
    mock_manager.ensure_container.return_value = "test-container-host"
    mock_manager.invalidate_cache = MagicMock()

    # LambdaInvoker のモック (内部で client を使うため)
    # ここでは Invoker 自体は本物使い、Client をモックするか、Invoker もモックするか。
    # テスト対象が main.py のエラーハンドリングなので、Invoker が例外を吐けばよい。
    # しかし main.py は proxy_to_lambda を呼んでいる箇所でエラーをキャッチしている（キャッチオールルートの場合）。
    # invoke_lambda_api の場合は invoker.invoke_function を呼ぶ。
    # このテストは `client.get("/test-path")` なので、`gateway_handler` -> `proxy_to_lambda` ルート。
    # proxy_to_lambda は http_client を使う。

    main_app.dependency_overrides[get_http_client] = lambda: mock_client
    main_app.dependency_overrides[get_orchestrator_client] = lambda: mock_manager

    # Auth & Routing Mocks
    async def mock_auth():
        return "test-user"

    async def mock_resolve(
        request,
    ):  # request引数を受け取るように修正（deps実装に合わせる） or単に値を返す
        # deps.py の resolve_lambda_target は request と route_matcher を取るが、
        # dependency_overrides で上書きする場合、シグネチャは自由（FastAPIが解決）。
        # しかし main.py で `target: LambdaTargetDep` なので、戻り値が TargetFunction であればよい。
        return TargetFunction(
            container_name="test-container",
            path_params={},
            route_path="/test-path",
            function_config={"image": "test-image"},
        )

    main_app.dependency_overrides[verify_authorization] = mock_auth
    main_app.dependency_overrides[resolve_lambda_target] = mock_resolve

    yield mock_client, mock_manager

    main_app.dependency_overrides = {}


@pytest.mark.asyncio
async def test_lambda_connection_error_logged_at_error_level(caplog):
    """
    Lambda接続失敗時にerrorレベルでログされることを検証
    """
    from services.gateway.main import app

    # Capture logs from gateway.lambda_invoker where the error is now logged
    caplog.set_level(logging.ERROR, logger="gateway.lambda_invoker")

    # Override dependencies
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    # Manager Mock
    mock_manager = AsyncMock()
    mock_manager.ensure_container.return_value = "test-container"
    mock_manager.invalidate_cache = MagicMock()

    # Create Invoker with mock client
    mock_registry = MagicMock(spec=FunctionRegistry)
    mock_registry.get_function_config.return_value = {"image": "test-image", "environment": {}}

    # Backend Mock
    mock_backend = AsyncMock()
    mock_worker = MagicMock()
    mock_worker.ip_address = "1.2.3.4"
    mock_backend.acquire_worker.return_value = mock_worker

    config = GatewayConfig()

    invoker = LambdaInvoker(mock_client, mock_registry, config, mock_backend)

    app.dependency_overrides[get_http_client] = lambda: mock_client
    app.dependency_overrides[get_orchestrator_client] = lambda: mock_manager
    app.dependency_overrides[get_lambda_invoker] = lambda: invoker
    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="test-container",
        path_params={},
        route_path="/test-path",
        function_config={"image": "test-image"},
    )

    from fastapi.testclient import TestClient

    # Trigger Lambda connection error via gateway_handler
    # proxy_to_lambda is imported in main.py, so we patch it there.
    # Note: proxy_to_lambda is gone. We mock invoker now (overridden dependency).
    # But mock_client is used in invoker? Yes we set up invoker with mock_client.
    # And we trigger error via mock_client side_effect.

    with TestClient(app) as client:
        # Trigger Lambda connection error
        mock_client.post.side_effect = httpx.ConnectError("Connection refused")
        client.get("/test-path", headers={"Authorization": "Bearer valid-token"})

    # Clean up overrides
    app.dependency_overrides = {}

    # Assert: Error level log should exist
    assert any(
        record.levelname == "ERROR"
        and record.name == "gateway.lambda_invoker"
        and "Lambda invocation failed" in record.message
        for record in caplog.records
    ), "Lambda connection error should be logged at ERROR level"


@pytest.mark.asyncio
async def test_lambda_connection_error_includes_detailed_info(caplog):
    """
    Lambda接続失敗時のログに詳細情報（host, port, timeout, error_detail）が含まれることを検証
    """
    from services.gateway.main import app
    from services.gateway.config import config

    caplog.set_level(logging.ERROR, logger="gateway.lambda_invoker")

    # Override dependencies
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    mock_manager = AsyncMock()
    mock_manager.ensure_container.return_value = "192.168.1.100"
    mock_manager.invalidate_cache = MagicMock()

    mock_registry = MagicMock(spec=FunctionRegistry)
    mock_registry.get_function_config.return_value = {"image": "test-image", "environment": {}}

    # Backend Mock
    mock_backend = AsyncMock()
    mock_worker = MagicMock()
    mock_worker.ip_address = "192.168.1.100"
    mock_backend.acquire_worker.return_value = mock_worker

    invoker = LambdaInvoker(mock_client, mock_registry, config, mock_backend)

    app.dependency_overrides[get_http_client] = lambda: mock_client
    app.dependency_overrides[get_orchestrator_client] = lambda: mock_manager
    app.dependency_overrides[get_lambda_invoker] = lambda: invoker
    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="test-container",
        path_params={},
        route_path="/test-path",
        function_config={"image": "test-image"},
    )

    from fastapi.testclient import TestClient

    with TestClient(app) as client:
        # Trigger Lambda connection error
        mock_client.post.side_effect = httpx.ConnectTimeout("Timeout after 30s")

        client.get("/test-path", headers={"Authorization": "Bearer valid-token"})

    # Clean up overrides
    app.dependency_overrides = {}

    # Assert: Log record should contain detailed info in extra fields
    error_records = [
        r for r in caplog.records if r.levelname == "ERROR" and r.name == "gateway.lambda_invoker"
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
