import base64
from unittest.mock import patch

import httpx

from services.gateway.core.utils import parse_lambda_response


def test_parse_lambda_response_logs_warning_on_invalid_json_body():
    """
    TDD Red: log a warning when Lambda response body is JSON but parsing fails.
    """
    # Lambda response: has statusCode but body is invalid JSON.
    response_data = {
        "statusCode": 200,
        "headers": {},
        "body": "{invalid json here",  # Invalid JSON
    }
    mock_response = httpx.Response(200, json=response_data)

    with patch("services.gateway.core.utils.logger") as mock_logger:
        result = parse_lambda_response(mock_response)

        # Ensure a warning log is emitted.
        mock_logger.warning.assert_called_once()
        # Ensure snippet is included in the log.
        call_args = mock_logger.warning.call_args
        assert "extra" in call_args.kwargs
        assert "snippet" in call_args.kwargs["extra"]

        # Result should remain the original string.
        assert result["content"] == "{invalid json here"


def test_parse_lambda_response_multi_value_headers_precedence():
    response_data = {
        "statusCode": 200,
        "headers": {"Set-Cookie": "a=1", "X-Foo": "bar", "x-baz": "lower"},
        "multiValueHeaders": {"set-cookie": ["b=2", "c=3"], "X-Baz": ["one", "two"]},
        "body": '{"ok": true}',
    }
    mock_response = httpx.Response(200, json=response_data)

    result = parse_lambda_response(mock_response)

    assert result["status_code"] == 200
    assert result["content"] == {"ok": True}
    assert result["headers"] == {"X-Foo": "bar"}
    assert result["multi_headers"]["set-cookie"] == ["b=2", "c=3"]
    assert result["multi_headers"]["X-Baz"] == ["one", "two"]


def test_parse_lambda_response_decodes_base64_encoded_body():
    binary_payload = b"\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xffhello-gzip"
    response_data = {
        "statusCode": 200,
        "headers": {"Content-Encoding": "gzip", "Content-Type": "application/json"},
        "body": base64.b64encode(binary_payload).decode("utf-8"),
        "isBase64Encoded": True,
    }
    mock_response = httpx.Response(200, json=response_data)

    result = parse_lambda_response(mock_response)

    assert result["status_code"] == 200
    assert result["raw_content"] == binary_payload
    assert result["headers"]["Content-Encoding"] == "gzip"
    assert result["headers"]["Content-Type"] == "application/json"


def test_parse_lambda_response_logs_warning_on_invalid_base64_body():
    response_data = {
        "statusCode": 200,
        "headers": {"Content-Encoding": "gzip"},
        "body": "%%%not-base64%%%",
        "isBase64Encoded": True,
    }
    mock_response = httpx.Response(200, json=response_data)

    with patch("services.gateway.core.utils.logger") as mock_logger:
        result = parse_lambda_response(mock_response)

    mock_logger.warning.assert_called_once()
    assert result["status_code"] == 200
    assert result["content"] == "%%%not-base64%%%"
