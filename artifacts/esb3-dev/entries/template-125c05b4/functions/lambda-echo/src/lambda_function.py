"""
Echo Lambda: lightweight Lambda that only returns a simple response.

Allows testing Lambda invocations without external dependencies like S3/DynamoDB.
"""

import json
import os
from datetime import datetime, timezone

from common.utils import create_response, handle_ping, parse_event_body


def lambda_handler(event, context):
    trace_id = os.environ.get("_X_AMZN_TRACE_ID", "not-found")
    # RIE Heartbeat
    if ping_response := handle_ping(event):
        return ping_response

    body = parse_event_body(event)
    username = (
        event.get("requestContext", {}).get("authorizer", {}).get("cognito:username", "anonymous")
    )

    message = f"Echo: {body.get('message', 'Hello')}"

    # Emit structured logs (for VictoriaLogs search).
    # sitecustomize auto-attaches trace_id, container_name, and job,
    # but output explicitly to validate consistency.
    log_entry = {
        "_time": datetime.now(timezone.utc).isoformat(timespec="milliseconds"),
        "level": "INFO",
        "trace_id": trace_id,
        "message": message,
        "function": "lambda-echo",
    }
    print(json.dumps(log_entry))

    # DEBUG log (test requirement).
    print(json.dumps({**log_entry, "level": "DEBUG", "message": "Debug log for quality test"}))

    return create_response(
        body={
            "success": True,
            "message": message,
            "user": username,
        }
    )
