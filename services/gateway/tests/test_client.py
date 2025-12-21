"""
ManagerClient のテスト (TDD Red フェーズ)

エラーマッピングとRequestId伝播の機能をテストします。
まだ実装がないため、これらのテストは失敗するはずです。
"""

import pytest
from unittest.mock import AsyncMock, MagicMock
import httpx
from services.gateway.client import ManagerClient


@pytest.fixture
def mock_client():
    client = AsyncMock(spec=httpx.AsyncClient)
    return client


@pytest.mark.asyncio
async def test_ensure_container_success(mock_client):
    """正常系: コンテナ起動成功"""
    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 200
    mock_response.json.return_value = {"host": "10.0.0.1"}
    mock_response.raise_for_status = MagicMock()

    mock_client.post.return_value = mock_response

    manager_client = ManagerClient(mock_client)
    host = await manager_client.ensure_container("test-func", "test-image")

    assert host == "10.0.0.1"
    mock_client.post.assert_called_once()


@pytest.mark.asyncio
async def test_ensure_container_network_failure(mock_client):
    """Manager への接続失敗 -> ManagerUnreachableError"""
    from services.gateway.core.exceptions import ManagerUnreachableError

    mock_client.post.side_effect = httpx.RequestError("Connection failed", request=MagicMock())

    manager_client = ManagerClient(mock_client)

    with pytest.raises(ManagerUnreachableError):
        await manager_client.ensure_container("test-func")


@pytest.mark.asyncio
async def test_ensure_container_timeout(mock_client):
    """Manager タイムアウト -> ManagerTimeoutError"""
    from services.gateway.core.exceptions import ManagerTimeoutError

    mock_client.post.side_effect = httpx.TimeoutException("Timeout", request=MagicMock())

    manager_client = ManagerClient(mock_client)

    with pytest.raises(ManagerTimeoutError):
        await manager_client.ensure_container("test-func")


@pytest.mark.asyncio
async def test_ensure_container_404_function_not_found(mock_client):
    """Manager 404 -> FunctionNotFoundError"""
    from services.gateway.core.exceptions import FunctionNotFoundError

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 404
    mock_response.text = "Image not found"

    mock_client.post.return_value = mock_response
    mock_client.post.return_value.raise_for_status.side_effect = httpx.HTTPStatusError(
        "404", request=MagicMock(), response=mock_response
    )

    manager_client = ManagerClient(mock_client)

    with pytest.raises(FunctionNotFoundError):
        await manager_client.ensure_container("test-func")


@pytest.mark.asyncio
async def test_ensure_container_400_docker_error(mock_client):
    """Manager 400 -> ManagerError"""
    from services.gateway.core.exceptions import ManagerError

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 400
    mock_response.text = "Docker API error"

    mock_client.post.return_value = mock_response
    mock_client.post.return_value.raise_for_status.side_effect = httpx.HTTPStatusError(
        "400", request=MagicMock(), response=mock_response
    )

    manager_client = ManagerClient(mock_client)

    with pytest.raises(ManagerError) as exc_info:
        await manager_client.ensure_container("test-func")

    assert exc_info.value.status_code == 400


@pytest.mark.asyncio
async def test_request_id_propagation(mock_client):
    """RequestId が X-Request-Id ヘッダーで伝播される"""
    from services.common.core.request_context import set_request_id, clear_request_id

    # RequestId を設定
    test_request_id = "test-request-123"
    set_request_id(test_request_id)

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 200
    mock_response.json.return_value = {"host": "10.0.0.1"}
    mock_response.raise_for_status = MagicMock()
    mock_client.post.return_value = mock_response

    manager_client = ManagerClient(mock_client)
    await manager_client.ensure_container("test-func")

    # X-Request-Id ヘッダーが付与されているか検証
    args, kwargs = mock_client.post.call_args
    assert "headers" in kwargs
    assert kwargs["headers"]["X-Request-Id"] == test_request_id

    clear_request_id()
