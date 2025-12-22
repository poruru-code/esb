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
            (b"x-request-id", b"test-req-id"),
        ],
        "client": ("127.0.0.1", 12345),
    }
    request = Request(scope)

    body = b'{"key": "value"}'
    user_id = "test-user"
    path_params = {"id": "123"}
    route_path = "/test/{id}"

    # Act
    with patch("services.gateway.core.event_builder.get_request_id", return_value="test-req-id"):
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
    assert context["requestId"] == "test-req-id"
    assert context["identity"]["sourceIp"] == "127.0.0.1"
    assert context["authorizer"]["claims"]["cognito:username"] == user_id


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
