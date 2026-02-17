"""
Gateway Utility Module
"""

import base64
import binascii
import json
import logging
from typing import Any, Dict, Union

import httpx

from services.gateway.models.result import InvocationResult

logger = logging.getLogger("gateway.utils")


def _decode_base64_response_body(body: Any) -> bytes | None:
    """Decode API Gateway-style base64 body, returning None on invalid input."""
    if body is None:
        return b""
    if isinstance(body, bytes):
        raw_body = body
    elif isinstance(body, str):
        raw_body = body.encode("utf-8")
    else:
        return None

    try:
        return base64.b64decode(raw_body, validate=True)
    except (binascii.Error, ValueError):
        return None


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
            response_headers = response_data.get("headers") or {}
            response_multi_headers = response_data.get("multiValueHeaders") or {}

            if not isinstance(response_headers, dict):
                response_headers = {}
            if not isinstance(response_multi_headers, dict):
                response_multi_headers = {}
            response_body = response_data.get("body", "")
            # Decode only when the contract value is explicitly boolean true.
            # Avoid truthy coercion (e.g. "false" -> True) from loosely typed runtimes.
            is_base64_encoded = response_data.get("isBase64Encoded") is True

            normalized_multi_headers: Dict[str, list[str]] = {}
            for key, values in response_multi_headers.items():
                if values is None:
                    continue
                if isinstance(values, list):
                    normalized_multi_headers[key] = [str(v) for v in values]
                else:
                    normalized_multi_headers[key] = [str(values)]

            # multiValueHeaders takes precedence when both are provided.
            multi_keys_lower = {key.lower() for key in normalized_multi_headers.keys()}
            filtered_headers = {
                key: str(value)
                for key, value in response_headers.items()
                if key.lower() not in multi_keys_lower
            }

            if is_base64_encoded:
                decoded = _decode_base64_response_body(response_body)
                if decoded is None:
                    logger.warning(
                        "Lambda response body marked isBase64Encoded but decode failed",
                        extra={"snippet": str(response_body)[:100] if response_body else ""},
                    )
                    return {
                        "status_code": status_code,
                        "content": response_body,
                        "headers": filtered_headers,
                        "multi_headers": normalized_multi_headers,
                    }
                return {
                    "status_code": status_code,
                    "raw_content": decoded,
                    "headers": filtered_headers,
                    "multi_headers": normalized_multi_headers,
                }

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
                "headers": filtered_headers,
                "multi_headers": normalized_multi_headers,
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
