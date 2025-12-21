import pytest
from unittest.mock import AsyncMock, patch
import httpx
from services.gateway.core.proxy import proxy_to_lambda


@pytest.mark.asyncio
async def test_proxy_to_lambda_uses_shared_client():
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_response = httpx.Response(200, json={"message": "ok"})
    mock_client.post.return_value = mock_response

    target_container = "test-container"
    event = {"key": "value"}

    # We mock resolve_container_ip using patch, assuming it is imported in proxy.py
    with patch("services.gateway.core.proxy.resolve_container_ip") as mock_resolve:
        mock_resolve.return_value = "1.2.3.4"

        # Call with client injected
        try:
            response = await proxy_to_lambda(target_container, event, client=mock_client)
        except TypeError:
            pytest.fail("proxy_to_lambda does not accept 'client' argument")

        assert response == mock_response
        mock_client.post.assert_called_once()
        args, kwargs = mock_client.post.call_args
        assert "http://1.2.3.4:8080" in args[0]


def test_build_event_propagates_request_id_from_context():
    from services.gateway.core.proxy import build_event
    from services.common.core.request_context import set_request_id, clear_request_id
    from fastapi import Request
    from unittest.mock import Mock

    # Setup
    clear_request_id()
    expected_rid = "test-trace-id-12345"
    set_request_id(expected_rid)

    # Mock Request
    mock_request = Mock(spec=Request)
    mock_request.url.path = "/test/path"
    mock_request.method = "POST"
    mock_request.headers = {}
    mock_request.query_params = {}
    mock_request.client.host = "127.0.0.1"

    # Execute
    event = build_event(
        request=mock_request,
        body=b"{}",
        user_id="test-user",
        path_params={},
        route_path="/test/path",
    )

    # Verify
    assert event["requestContext"]["requestId"] == expected_rid, (
        f"Expected {expected_rid}, but got {event['requestContext']['requestId']}"
    )

    # Verify fallback behavior (no context)
    clear_request_id()
    event_fallback = build_event(
        request=mock_request,
        body=b"{}",
        user_id="test-user",
        path_params={},
        route_path="/test/path",
    )
    assert event_fallback["requestContext"]["requestId"] is not None
    assert event_fallback["requestContext"]["requestId"] != expected_rid
    assert (
        event_fallback["requestContext"]["requestId"].startswith("req-")
        or len(event_fallback["requestContext"]["requestId"]) > 10
    )
