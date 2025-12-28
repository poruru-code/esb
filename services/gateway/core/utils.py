"""
Gateway Utility Module
"""

import json
import logging
from typing import Dict, Any

import httpx

logger = logging.getLogger("gateway.utils")


def parse_lambda_response(lambda_response: httpx.Response) -> Dict[str, Any]:
    """
    Parse Lambda RIE response and convert to FastAPI response data.

    Args:
        lambda_response: raw response from Lambda RIE

    Returns:
        Dict for FastAPI response:
        {
            "status_code": int,
            "content": Any,
            "headers": dict,
            "raw_content": bytes (only when JSON parsing fails)
        }
    """
    try:
        response_data = lambda_response.json()

        # When Lambda response uses API Gateway format.
        if isinstance(response_data, dict) and "statusCode" in response_data:
            status_code = response_data.get("statusCode", 200)
            response_headers = response_data.get("headers", {})
            response_body = response_data.get("body", "")

            # Parse body if it's a JSON string.
            if isinstance(response_body, str):
                try:
                    response_body = json.loads(response_body)
                except json.JSONDecodeError:
                    logger.warning(
                        "Failed to parse Lambda response body as JSON. Returning as string.",
                        extra={
                            "snippet": response_body[:200] if response_body else "",
                            "status_code": status_code,
                        },
                    )

            return {
                "status_code": status_code,
                "content": response_body,
                "headers": response_headers,
            }
        else:
            return {"status_code": 200, "content": response_data, "headers": {}}

    except json.JSONDecodeError:
        return {
            "status_code": lambda_response.status_code,
            "raw_content": lambda_response.content,
            "headers": dict(lambda_response.headers),
        }
