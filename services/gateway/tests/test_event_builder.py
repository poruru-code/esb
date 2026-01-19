from unittest.mock import patch

import pytest

from services.gateway.core.event_builder import V1ProxyEventBuilder
from services.gateway.models.context import InputContext


@pytest.mark.asyncio
async def test_v1_event_builder_build():
    """Test V1ProxyEventBuilder builds correct event structure"""
    # Arrange
    builder = V1ProxyEventBuilder()

    context = InputContext(
        function_name="test-function",
        method="POST",
        path="/test/path",
        headers={
            "content-type": "application/json",
            "user-agent": "test-agent",
            "x-amzn-trace-id": "Root=1-12345678-abcdef0123456789abcdef01;Sampled=1",
        },
        multi_headers={
            "content-type": ["application/json"],
            "user-agent": ["test-agent"],
            "x-amzn-trace-id": ["Root=1-12345678-abcdef0123456789abcdef01;Sampled=1"],
        },
        query_params={"foo": "bar"},
        multi_query_params={"foo": ["bar"]},
        body=b'{"key": "value"}',
        user_id="test-user",
        path_params={"id": "123"},
        route_path="/test/{id}",
    )

    # Act
    with patch(
        "services.gateway.core.event_builder.get_request_id",
        return_value="req-uuid-1234",
    ):
        event = builder.build(context=context)

    # Assert
    assert event["resource"] == "/test/{id}"
    assert event["path"] == "/test/path"
    assert event["httpMethod"] == "POST"
    assert event["headers"]["content-type"] == "application/json"
    assert event["multiValueHeaders"]["content-type"] == ["application/json"]
    assert event["queryStringParameters"]["foo"] == "bar"
    assert event["pathParameters"]["id"] == "123"
    assert event["body"] == '{"key": "value"}'
    assert event["isBase64Encoded"] is False

    # Context checks
    context_data = event["requestContext"]
    assert context_data["requestId"] == "req-uuid-1234"
    assert context_data["authorizer"]["claims"]["cognito:username"] == "test-user"


@pytest.mark.asyncio
async def test_event_builder_uses_generated_request_id():
    """Ensure the Event Builder uses the Request ID from context."""
    from services.common.core import request_context

    # Arrange
    builder = V1ProxyEventBuilder()

    context = InputContext(
        function_name="test",
        method="GET",
        path="/test",
        headers={},
        multi_headers={},
        query_params={},
        multi_query_params={},
        body=b"",
    )

    # Set context.
    trace_id_str = "Root=1-abc-123;Sampled=1"

    request_context.clear_trace_id()
    request_context.set_trace_id(trace_id_str)

    # Generate UUID and set it in context.
    req_id_str = request_context.generate_request_id()

    # Act
    event = builder.build(context)

    # Assert
    # requestContext.requestId should match the generated ID, NOT the Trace ID root
    assert event["requestContext"]["requestId"] == req_id_str
    assert event["requestContext"]["requestId"] != "1-abc-123"


@pytest.mark.asyncio
async def test_v1_event_builder_binary_body():
    """Test V1ProxyEventBuilder with binary body"""
    builder = V1ProxyEventBuilder()

    # Binary data that fails utf-8 decode
    body = b"\x80\xff"

    context = InputContext(
        function_name="test",
        method="POST",
        path="/path",
        headers={},
        multi_headers={},
        query_params={},
        multi_query_params={},
        body=body,
        user_id="user",
        path_params={},
        route_path="/path",
    )

    event = builder.build(context)

    assert event["isBase64Encoded"] is True
    assert event["body"] is not None
