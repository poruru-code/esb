"""
Gateway Utility Module
"""

import json
import logging
from typing import Any, Dict, Union

import httpx

from services.gateway.models.result import InvocationResult

logger = logging.getLogger("gateway.utils")


def parse_lambda_response(
    lambda_response: Union[httpx.Response, InvocationResult],
) -> Dict[str, Any]:
    """
    Parse Lambda RIE response and convert to FastAPI response data.

    Args:
        lambda_response: raw response from Lambda RIE (httpx.Response) or processed InvocationResult
    """
    if isinstance(lambda_response, InvocationResult):
        content = lambda_response.payload
        status = lambda_response.status_code
        headers = dict(lambda_response.headers)
        multi_headers = lambda_response.multi_headers
    else:
        content = lambda_response.content
        status = lambda_response.status_code
        headers = dict(lambda_response.headers)
        multi_headers = {
            k: lambda_response.headers.get_list(k) for k in lambda_response.headers.keys()
        }

    try:
        if not content:
            return {
                "status_code": status,
                "content": {},
                "headers": headers,
                "multi_headers": multi_headers,
            }

        response_data = json.loads(content)

        # When Lambda response uses API Gateway format.
        if isinstance(response_data, dict) and "statusCode" in response_data:
            status_code = response_data.get("statusCode", 200)
            response_headers = response_data.get("headers", {})
            response_multi_headers = response_data.get("multiValueHeaders", {})
            response_body = response_data.get("body", "")

            # Merge single headers into multi headers for consistent handling
            # Note: API Gateway behavior is that multiValueHeaders takes precedence if both exist,
            # but usually developers use one or the other. We'll merge.
            final_multi_headers = dict(response_multi_headers)
            for k, v in response_headers.items():
                if k not in final_multi_headers:
                    final_multi_headers[k] = [str(v)]

            # Parse body if it's a JSON string.
            if isinstance(response_body, str):
                try:
                    response_body = json.loads(response_body)
                except json.JSONDecodeError:
                    # Keep as string
                    logger.warning(
                        "Lambda response body is not valid JSON string",
                        extra={"snippet": response_body[:100] if response_body else ""},
                    )
                    pass

            return {
                "status_code": status_code,
                "content": response_body,
                "headers": response_headers,
                "multi_headers": final_multi_headers,
            }
        else:
            return {"status_code": 200, "content": response_data, "headers": headers}

    except (json.JSONDecodeError, TypeError):
        return {
            "status_code": status,
            "raw_content": content,
            "headers": headers,
            "multi_headers": multi_headers,
        }
