import json
import logging
import os

import boto3
from common.utils import create_response, handle_ping, parse_event_body

logger = logging.getLogger()
logger.setLevel(logging.INFO)


def lambda_handler(event, context):
    # RIE heartbeat.
    if ping_response := handle_ping(event):
        return ping_response

    # Get Trace ID from environment variables.
    trace_id = os.environ.get("_X_AMZN_TRACE_ID", "not-found")
    logger.info(f"Trace ID in environment: {trace_id}")

    # Parse body (API Gateway or direct).
    body = parse_event_body(event)
    next_target = body.get("next_target")
    is_async = body.get("async", False)

    child_info = None

    if next_target:
        logger.info(f"Chaining {'async' if is_async else 'sync'} invocation to {next_target}")
        invoke_type = "Event" if is_async else "RequestResponse"

        client = boto3.client("lambda")
        response = client.invoke(
            FunctionName=next_target,
            InvocationType=invoke_type,
            Payload=json.dumps({"message": "from-chain"}).encode("utf-8"),
        )

        if not is_async:
            child_payload = response["Payload"].read().decode("utf-8")
            child_info = json.loads(child_payload)
        else:
            child_info = {"status": "async-started", "status_code": response["StatusCode"]}

    return create_response(
        body={
            "success": True,
            "trace_id": trace_id,
            "child": child_info,
        }
    )
