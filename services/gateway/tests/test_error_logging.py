"""
Gateway エラーハンドリングのログ詳細化テスト

Lambda接続失敗時に適切なログレベル（error）で詳細情報が記録されることを検証
"""

import pytest
from unittest.mock import AsyncMock, patch, MagicMock
import httpx
import logging
from services.gateway.api.deps import (
    get_http_client,
    get_manager_client,
    verify_authorization,
    resolve_lambda_target,
)
from services.gateway.models import TargetFunction


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
    main_app.dependency_overrides[get_manager_client] = lambda: mock_manager

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

    caplog.set_level(logging.ERROR, logger="gateway.main")

    # Override dependencies
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    # Manager Mock
    mock_manager = AsyncMock()
    mock_manager.ensure_container.return_value = "test-container"

    app.dependency_overrides[get_http_client] = lambda: mock_client
    app.dependency_overrides[get_manager_client] = lambda: mock_manager
    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="test-container",
        path_params={},
        route_path="/test-path",
        function_config={"image": "test-image"},
    )

    from fastapi.testclient import TestClient

    client = TestClient(app)

    # Trigger Lambda connection error via gateway_handler
    # proxy_to_lambda is imported in main.py, so we patch it there.
    with patch("services.gateway.main.build_event", return_value={}):
        with patch("services.gateway.main.proxy_to_lambda") as mock_proxy:
            mock_proxy.side_effect = httpx.ConnectError("Connection refused")

            client.get("/test-path", headers={"Authorization": "Bearer valid-token"})

    # Clean up overrides
    app.dependency_overrides = {}

    # Assert: Error level log should exist
    assert any(
        record.levelname == "ERROR" and "Lambda connection failed" in record.message
        for record in caplog.records
    ), "Lambda connection error should be logged at ERROR level"


@pytest.mark.asyncio
async def test_lambda_connection_error_includes_detailed_info(caplog):
    """
    Lambda接続失敗時のログに詳細情報（host, port, timeout, error_detail）が含まれることを検証
    """
    from services.gateway.main import app
    from services.gateway.config import config

    caplog.set_level(logging.ERROR, logger="gateway.main")

    # Override dependencies
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_client.timeout.read = 30.0

    mock_manager = AsyncMock()
    mock_manager.ensure_container.return_value = "192.168.1.100"
    mock_manager.invalidate_cache = MagicMock()  # 同期メソッドなので MagicMock

    app.dependency_overrides[get_http_client] = lambda: mock_client
    app.dependency_overrides[get_manager_client] = lambda: mock_manager
    app.dependency_overrides[verify_authorization] = lambda: "test-user"
    app.dependency_overrides[resolve_lambda_target] = lambda: TargetFunction(
        container_name="test-container",
        path_params={},
        route_path="/test-path",
        function_config={"image": "test-image"},
    )

    from fastapi.testclient import TestClient

    client = TestClient(app)

    # Trigger Lambda connection error
    with patch("services.gateway.main.build_event", return_value={}):
        with patch("services.gateway.main.proxy_to_lambda") as mock_proxy:
            mock_proxy.side_effect = httpx.ConnectTimeout("Timeout after 30s")

            client.get("/test-path", headers={"Authorization": "Bearer valid-token"})

    # Clean up overrides
    app.dependency_overrides = {}

    # Assert: Log record should contain detailed info in extra fields
    error_records = [r for r in caplog.records if r.levelname == "ERROR"]
    if not error_records:
        print("\nCaptured Log Records:")
        for r in caplog.records:
            print(f"  Level: {r.levelname}, Message: {r.message}")
    assert len(error_records) > 0, "Should have at least one ERROR log"

    # Check for detailed fields in log record
    error_record = error_records[0]

    # ログ出力内容の検証
    # extraフィールドは直接属性としてアクセスできない場合がある（LogRecordの仕様）が、
    # Pythonのloggingモジュールでは extra で渡した辞書は LogRecord の属性になる。
    assert hasattr(error_record, "container_host"), "Log should include container_host"
    assert hasattr(error_record, "port"), "Log should include port"
    assert hasattr(error_record, "timeout"), "Log should include timeout"
    assert hasattr(error_record, "error_detail"), "Log should include error_detail"

    assert error_record.container_host == "192.168.1.100"
    assert error_record.port == config.LAMBDA_PORT
    assert error_record.timeout == 30.0
    assert "Timeout" in error_record.error_detail

    # Invalidate cache が呼ばれたことも確認
    mock_manager.invalidate_cache.assert_called_with("test-container")
