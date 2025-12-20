import pytest
import sys
import os
import httpx
from fastapi import Request
from unittest.mock import MagicMock, patch

# Add project root to sys.path
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../")))

from services.gateway.core.proxy import build_event
from services.gateway.services.lambda_invoker import invoke_function
from services.gateway.core.exceptions import ContainerStartError


def test_build_event_multi_value_headers():
    """Verify that build_event includes multiValueHeaders using proper getlist/keys mocks."""
    headers_dict = {"header1": ["value1"], "header2": ["value2", "value3"], "X-Custom": ["a"]}

    mock_headers = MagicMock()
    mock_headers.keys.return_value = headers_dict.keys()
    mock_headers.getlist.side_effect = lambda k: headers_dict.get(k, [])

    request = MagicMock(spec=Request)
    request.method = "POST"
    request.url.path = "/hello"
    request.headers = mock_headers
    request.query_params = MagicMock()
    request.query_params.keys.return_value = []
    request.client = MagicMock()
    request.client.host = "1.2.3.4"

    event = build_event(request, b"body", "user123", {}, "/hello")

    assert "multiValueHeaders" in event
    assert event["multiValueHeaders"]["header1"] == ["value1"]
    assert event["multiValueHeaders"]["header2"] == ["value2", "value3"]
    assert event["headers"]["header2"] == "value3"  # Last value


def test_build_event_multi_value_query_params():
    """Verify that build_event includes multiValueQueryStringParameters."""
    query_dict = {"param1": ["val1"], "param2": ["val2", "val3"]}

    mock_query = MagicMock()
    mock_query.keys.return_value = query_dict.keys()
    mock_query.getlist.side_effect = lambda k: query_dict.get(k, [])

    request = MagicMock(spec=Request)
    request.method = "GET"
    request.url.path = "/test"
    request.headers = MagicMock()
    request.headers.keys.return_value = []
    request.query_params = mock_query
    request.client = MagicMock()
    request.client.host = "1.2.3.4"

    event = build_event(request, b"", "user123", {}, "/test")

    assert "multiValueQueryStringParameters" in event
    assert event["multiValueQueryStringParameters"]["param1"] == ["val1"]
    assert event["multiValueQueryStringParameters"]["param2"] == ["val2", "val3"]
    assert event["queryStringParameters"]["param2"] == "val3"


@pytest.mark.asyncio
async def test_invoke_function_image_not_found():
    """Verify that invoke_function handles 404 from manager specifically."""
    with (
        patch("services.gateway.services.lambda_invoker.get_function_config") as mock_get_config,
        patch("services.gateway.services.lambda_invoker.get_lambda_host") as mock_get_host,
    ):
        mock_get_config.return_value = {"image": "non-existent-image"}

        # Simulate manager returning 404
        mock_response = MagicMock(spec=httpx.Response)
        mock_response.status_code = 404
        mock_response.text = "Image not found"

        mock_get_host.side_effect = httpx.HTTPStatusError(
            "404 Client Error", request=MagicMock(), response=mock_response
        )

        with pytest.raises(ContainerStartError) as excinfo:
            await invoke_function("test-func", b"{}")

        assert "404" in str(excinfo.value) or "not found" in str(excinfo.value).lower()
