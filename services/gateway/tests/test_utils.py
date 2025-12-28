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
