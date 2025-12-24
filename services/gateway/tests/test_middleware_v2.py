import pytest
import uuid
import os

# Config読み込み回避のためのダミー環境変数設定
os.environ.setdefault("JWT_SECRET_KEY", "test-secret")
os.environ.setdefault("X_API_KEY", "test-key")
os.environ.setdefault("AUTH_USER", "admin")
os.environ.setdefault("AUTH_PASS", "admin")
os.environ.setdefault("CONTAINERS_NETWORK", "test-net")
os.environ.setdefault("GATEWAY_INTERNAL_URL", "http://gateway:8000")
os.environ.setdefault("MANAGER_URL", "http://manager:8001")

from fastapi import Request, Response
# main.py から直接 import できるか確認が必要だが、app.middleware でデコレートされている関数自体は import 可能
from services.gateway.main import trace_propagation_middleware
from services.common.core import request_context
from unittest.mock import MagicMock

@pytest.mark.asyncio
async def test_trace_propagation_middleware_generates_request_id():
    """MiddlewareがTrace IDとは独立してRequest IDを生成し、レスポンスヘッダーに付与することを確認"""
    
    # Arrange
    request = MagicMock(spec=Request)
    request.headers = {}
    request.method = "GET"
    request.url.path = "/test"
    request.client = MagicMock()
    request.client.host = "127.0.0.1"
    
    # state 属性をモック
    request.state = MagicMock()

    async def call_next(req):
        # Middleware実行中のコンテキストをキャプチャ
        req.state.captured_req_id = request_context.get_request_id()
        req.state.captured_trace_id = request_context.get_trace_id()
        return Response(status_code=200)

    # Act
    request_context.clear_trace_id() # Ensure clean state
    response = await trace_propagation_middleware(request, call_next)

    # Assert
    # 1. Context内にRequest IDが生成されていること
    req_id = request.state.captured_req_id
    assert req_id is not None
    assert isinstance(req_id, str)
    try:
        uuid.UUID(req_id)
    except ValueError:
        pytest.fail(f"Request ID is not UUID: {req_id}")

    # 2. Response header has x-amzn-RequestId
    # 注意: 大文字小文字は CaseInsensitiveDict なら無視されるが、FastAPI Response headers はそう
    assert "x-amzn-RequestId" in response.headers
    assert response.headers["x-amzn-RequestId"] == req_id

    # 3. Trace ID was also generated
    trace_id = request.state.captured_trace_id
    assert trace_id is not None
    
    # 4. Request ID and Trace ID are different (Trace ID Root != Request ID)
    # 以前のTrace ID実装では Root=Request ID だったが、今は違うはず
    # Trace ID format: Root=1-xxx-xxx...
    assert req_id not in trace_id # UUIDがそのままTraceIDに含まれていないこと（Root部分として）
