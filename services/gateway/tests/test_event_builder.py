import pytest
from unittest.mock import patch
from fastapi import Request
from services.gateway.core.event_builder import V1ProxyEventBuilder


@pytest.mark.asyncio
async def test_v1_event_builder_build():
    """Test V1ProxyEventBuilder builds correct event structure"""
    # Arrange
    builder = V1ProxyEventBuilder()

    # Mock Request
    scope = {
        "type": "http",
        "http_version": "1.1",
        "method": "POST",
        "path": "/test/path",
        "query_string": b"foo=bar",
        "headers": [
            (b"content-type", b"application/json"),
            (b"user-agent", b"test-agent"),
            (b"x-amzn-trace-id", b"Root=1-12345678-abcdef0123456789abcdef01;Sampled=1"),
        ],
        "client": ("127.0.0.1", 12345),
    }
    request = Request(scope)

    body = b'{"key": "value"}'
    user_id = "test-user"
    path_params = {"id": "123"}
    route_path = "/test/{id}"

    # Act
    with patch(
        "services.gateway.core.event_builder.get_request_id",
        return_value="req-uuid-1234",
    ):
        event = await builder.build(
            request=request,
            body=body,
            user_id=user_id,
            path_params=path_params,
            route_path=route_path,
        )

    # Assert
    assert event["resource"] == route_path
    assert event["path"] == "/test/path"
    assert event["httpMethod"] == "POST"
    assert event["headers"]["content-type"] == "application/json"
    assert event["multiValueHeaders"]["content-type"] == ["application/json"]
    assert event["queryStringParameters"]["foo"] == "bar"
    assert event["pathParameters"]["id"] == "123"
    assert event["body"] == '{"key": "value"}'
    assert event["isBase64Encoded"] is False

    # Context checks
    context = event["requestContext"]
    assert context["requestId"] == "req-uuid-1234"
    assert context["identity"]["sourceIp"] == "127.0.0.1"
    assert context["authorizer"]["claims"]["cognito:username"] == user_id


@pytest.mark.asyncio
async def test_event_builder_uses_generated_request_id():
    """Ensure the Event Builder uses the Request ID from context."""
    from unittest.mock import MagicMock
    from services.common.core import request_context

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

    # Set context.
    trace_id_str = "Root=1-abc-123;Sampled=1"

    request_context.clear_trace_id()
    request_context.set_trace_id(trace_id_str)

    # Generate UUID and set it in context.
    req_id_str = request_context.generate_request_id()

    # Act
    event = await builder.build(request, b"")

    # Assert
    # requestContext.requestId should match the generated ID, NOT the Trace ID root
    assert event["requestContext"]["requestId"] == req_id_str
    assert event["requestContext"]["requestId"] != "1-abc-123"


@pytest.mark.asyncio
async def test_v1_event_builder_binary_body():
    """Test V1ProxyEventBuilder with binary body"""
    builder = V1ProxyEventBuilder()
    scope = {"type": "http", "headers": [], "path": "/path", "query_string": b"", "method": "POST"}
    request = Request(scope)

    # Binary data that fails utf-8 decode
    body = b"\x80\xff"

    event = await builder.build(request, body, user_id="user", path_params={}, route_path="/path")

    assert event["isBase64Encoded"] is True
    assert event["body"] is not None
