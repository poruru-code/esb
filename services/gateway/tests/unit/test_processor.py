import json
from unittest.mock import AsyncMock, MagicMock

import pytest

from services.gateway.models.context import InputContext
from services.gateway.models.result import InvocationResult
from services.gateway.services.processor import GatewayRequestProcessor


@pytest.mark.asyncio
async def test_process_request_success():
    # Arrange
    invoker = AsyncMock()
    event_builder = MagicMock()
    processor = GatewayRequestProcessor(invoker, event_builder)

    context = InputContext(
        function_name="test-function",
        method="POST",
        path="/test",
        headers={"content-type": "application/json"},
        query_params={},
        body=b'{"key": "value"}',
        timeout=30.0,
    )

    mock_event = {"path": "/test", "httpMethod": "POST"}
    event_builder.build.return_value = mock_event

    mock_result = InvocationResult(
        success=True,
        status_code=200,
        payload=b'{"message": "ok"}',
        headers={"Content-Type": "application/json"},
    )
    invoker.invoke_function.return_value = mock_result

    # Act
    result = await processor.process_request(context)

    # Assert
    assert result.success is True
    assert result.status_code == 200
    assert result.payload == b'{"message": "ok"}'

    event_builder.build.assert_called_once_with(context)
    invoker.invoke_function.assert_called_once_with(
        "test-function", json.dumps(mock_event).encode("utf-8"), timeout=30.0
    )


@pytest.mark.asyncio
async def test_process_request_invoker_failure():
    # Arrange
    invoker = AsyncMock()
    event_builder = MagicMock()
    processor = GatewayRequestProcessor(invoker, event_builder)

    context = InputContext(
        function_name="test-function", method="GET", path="/test", headers={}, timeout=10.0
    )

    event_builder.build.return_value = {}

    mock_result = InvocationResult(success=False, status_code=502, error="Bad Gateway")
    invoker.invoke_function.return_value = mock_result

    # Act
    result = await processor.process_request(context)

    # Assert
    assert result.success is False
    assert result.status_code == 502
    assert result.error == "Bad Gateway"


@pytest.mark.asyncio
async def test_process_request_unexpected_exception():
    # Arrange
    invoker = AsyncMock()
    event_builder = MagicMock()
    processor = GatewayRequestProcessor(invoker, event_builder)

    context = InputContext(function_name="test-function", method="GET", path="/test", headers={})

    event_builder.build.side_effect = Exception("Surprise error")

    # Act
    result = await processor.process_request(context)

    # Assert
    assert result.success is False
    assert result.status_code == 500
    assert result.error is not None
    assert "Internal Processing Error" in result.error
    assert "Surprise error" in result.error
