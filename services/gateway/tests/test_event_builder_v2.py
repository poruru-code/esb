import pytest
import os

# Config読み込み回避のためのダミー環境変数設定
os.environ.setdefault("JWT_SECRET_KEY", "test-secret")
os.environ.setdefault("X_API_KEY", "test-key")
os.environ.setdefault("AUTH_USER", "admin")
os.environ.setdefault("AUTH_PASS", "admin")
os.environ.setdefault("CONTAINERS_NETWORK", "test-net")
os.environ.setdefault("GATEWAY_INTERNAL_URL", "http://gateway:8000")
os.environ.setdefault("MANAGER_URL", "http://manager:8001")

from unittest.mock import MagicMock
from fastapi import Request
from services.gateway.core.event_builder import V1ProxyEventBuilder
from services.common.core import request_context


@pytest.mark.asyncio
async def test_event_builder_uses_generated_request_id():
    """Event BuilderがContextのRequest IDを使用することを確認"""

    # Arrange
    builder = V1ProxyEventBuilder()
    request = MagicMock(spec=Request)
    request.url.path = "/test"
    request.method = "GET"
    request.headers = MagicMock()
    request.headers.keys.return_value = []  # keys() iterator
    request.headers.getlist.return_value = []
    request.headers.get.return_value = "gzip"  # for content-encoding check
    request.query_params = {}
    request.client.host = "1.2.3.4"
    request.scope = {"http_version": "1.1"}

    # Contextセット
    trace_id_str = "Root=1-abc-123;Sampled=1"

    request_context.clear_trace_id()
    request_context.set_trace_id(trace_id_str)

    # UUIDを生成してContextにセット
    req_id_str = request_context.generate_request_id()

    # Act
    event = await builder.build(request, b"")

    # Assert
    # requestContext.requestId should match the generated ID, NOT the Trace ID root
    print(f"Actual requestId: {event['requestContext']['requestId']}")
    assert event["requestContext"]["requestId"] == req_id_str
    assert event["requestContext"]["requestId"] != "1-abc-123"
