import pytest
from unittest.mock import Mock
from fastapi import Request, Response
from services.gateway.main import trace_propagation_middleware
from services.common.core.request_context import get_trace_id, clear_trace_id


@pytest.mark.asyncio
async def test_trace_propagation_middleware_sets_context():
    # Setup
    clear_trace_id()
    expected_tid = "Root=1-6789abcd-1234567890abcdef12345678;Sampled=1"

    # Mock Request
    mock_request = Mock(spec=Request)
    mock_request.headers = {"X-Amzn-Trace-Id": expected_tid}
    mock_request.method = "GET"
    mock_request.url.path = "/test"

    # Mock call_next
    async def mock_call_next(request):
        # Verify context INSIDE the next call
        current_tid = get_trace_id()
        assert current_tid == expected_tid, (
            f"Context TraceId inside call_next mismatch: {current_tid}"
        )
        return Response(status_code=200)

    # Execute
    response = await trace_propagation_middleware(mock_request, mock_call_next)

    # Verify Response Header
    assert response.headers["X-Amzn-Trace-Id"] == expected_tid


@pytest.mark.asyncio
async def test_trace_propagation_middleware_generates_id_if_missing():
    clear_trace_id()

    mock_request = Mock(spec=Request)
    mock_request.headers = {}  # No X-Amzn-Trace-Id
    mock_request.method = "GET"
    mock_request.url.path = "/test"

    captured_tid = None

    async def mock_call_next(request):
        nonlocal captured_tid
        captured_tid = get_trace_id()
        return Response(status_code=200)

    response = await trace_propagation_middleware(mock_request, mock_call_next)

    assert captured_tid is not None
    assert response.headers["X-Amzn-Trace-Id"] == captured_tid
    assert captured_tid.startswith("Root=1-")


@pytest.mark.asyncio
async def test_trace_propagation_middleware_generates_request_id():
    """Ensure middleware generates Request ID independently of Trace ID and sets response header."""
    import uuid
    from services.common.core import request_context
    from unittest.mock import MagicMock

    # Arrange
    request = MagicMock(spec=Request)
    request.headers = {}
    request.method = "GET"
    request.url.path = "/test"
    request.client = MagicMock()
    request.client.host = "127.0.0.1"

    # Mock state attribute.
    request.state = MagicMock()

    async def call_next(req):
        # Capture context during middleware execution.
        req.state.captured_req_id = request_context.get_request_id()
        req.state.captured_trace_id = request_context.get_trace_id()
        return Response(status_code=200)

    # Act
    request_context.clear_trace_id()  # Ensure clean state
    response = await trace_propagation_middleware(request, call_next)

    # Assert
    # 1. Ensure Request ID is generated in context.
    req_id = request.state.captured_req_id
    assert req_id is not None
    assert isinstance(req_id, str)
    try:
        uuid.UUID(req_id)
    except ValueError:
        pytest.fail(f"Request ID is not UUID: {req_id}")

    # 2. Response header has x-amzn-RequestId.
    # Note: CaseInsensitiveDict ignores case, but FastAPI Response headers do not.
    assert "x-amzn-RequestId" in response.headers
    assert response.headers["x-amzn-RequestId"] == req_id

    # 3. Trace ID was also generated.
    trace_id = request.state.captured_trace_id
    assert trace_id is not None

    # 4. Request ID and Trace ID are different (Trace ID Root != Request ID).
    # Older implementations used Root=Request ID, but it should differ now.
    # Trace ID format: Root=1-xxx-xxx...
    assert req_id not in trace_id  # UUID should not appear in Trace ID Root
