# Where: tools/e2e-minimal-lambda/lambda_function.py
# What: Minimal Lambda handler for image-based E2E invocation tests.
# Why: Provide a stable response without external dependencies.
from __future__ import annotations

import json
from typing import Any


def _extract_message(event: Any) -> str:
    default = "hello-image"
    if not isinstance(event, dict):
        return default

    body = event.get("body")
    if isinstance(body, str):
        try:
            body_json = json.loads(body)
        except json.JSONDecodeError:
            body_json = {}
        if isinstance(body_json, dict):
            msg = body_json.get("message")
            if isinstance(msg, str) and msg:
                return msg

    msg = event.get("message")
    if isinstance(msg, str) and msg:
        return msg
    return default


def lambda_handler(event: Any, _context: Any) -> dict[str, Any]:
    message = _extract_message(event)
    return {
        "success": True,
        "message": message,
        "handler": "e2e-minimal-lambda",
    }
