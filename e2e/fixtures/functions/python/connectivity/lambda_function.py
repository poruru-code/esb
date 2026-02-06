"""
Where: e2e/fixtures/functions/python/connectivity/lambda_function.py
What: Python connectivity Lambda fixture for E2E smoke coverage.
Why: Provide a common action contract across runtimes.
"""

import json
import logging
import os
import time

import boto3
from common.utils import create_response, handle_ping, parse_event_body

logger = logging.getLogger()
logger.setLevel(logging.INFO)

DEFAULT_TABLE = "e2e-test-table"
DEFAULT_BUCKET = "e2e-test-bucket"
DEFAULT_LAMBDA_TARGET = "lambda-echo"


def lambda_handler(event, context):
    # Handle RIE heartbeat checks.
    if ping_response := handle_ping(event):
        return ping_response

    body = parse_event_body(event)
    action = body.get("action", "echo")
    logger.info("Processing action: %s", action)

    try:
        if action == "echo":
            return handle_echo(body)
        if action == "dynamodb_put":
            return handle_dynamodb_put(body)
        if action == "s3_put":
            return handle_s3_put(body)
        if action == "chain_invoke":
            return handle_chain_invoke(body)
        if action == "test_cloudwatch":
            return handle_cloudwatch()
    except Exception as exc:
        return create_response(
            status_code=500,
            body={"success": False, "action": action, "error": str(exc)},
        )

    return create_response(body={"success": True, "action": action})


def handle_echo(body):
    message = body.get("message", "hello")
    return create_response(
        body={
            "success": True,
            "action": "echo",
            "message": f"Echo: {message}",
        }
    )


def handle_dynamodb_put(body):
    table = os.environ.get("E2E_TABLE_NAME", DEFAULT_TABLE)
    key = body.get("key", "python-smoke")
    value = body.get("value", "ok")

    client = boto3.client("dynamodb")
    client.put_item(
        TableName=table,
        Item={
            "id": {"S": str(key)},
            "value": {"S": str(value)},
        },
    )

    return create_response(body={"success": True, "action": "dynamodb_put"})


def handle_s3_put(body):
    bucket = body.get("bucket", os.environ.get("E2E_BUCKET", DEFAULT_BUCKET))
    key = body.get("key", "python-connectivity.txt")
    content = body.get("content", "ok")

    client = boto3.client("s3")
    try:
        client.create_bucket(Bucket=bucket)
    except Exception:
        pass  # bucket may already exist

    client.put_object(Bucket=bucket, Key=key, Body=str(content))

    return create_response(body={"success": True, "action": "s3_put"})


def handle_chain_invoke(body):
    target = body.get("target", DEFAULT_LAMBDA_TARGET)
    payload = {"message": body.get("message", "from-python")}

    client = boto3.client("lambda")
    response = client.invoke(
        FunctionName=target,
        InvocationType="RequestResponse",
        Payload=json.dumps(payload).encode("utf-8"),
    )
    payload_text = response["Payload"].read().decode("utf-8")
    try:
        child = json.loads(payload_text)
    except json.JSONDecodeError:
        child = payload_text

    return create_response(
        body={
            "success": True,
            "action": "chain_invoke",
            "child": child,
        }
    )


def handle_cloudwatch():
    try:
        logs_client = boto3.client("logs")
        log_group = "/lambda/hello-test"
        log_stream = f"test-stream-{int(time.time())}"

        try:
            logs_client.create_log_group(logGroupName=log_group)
        except Exception:
            pass  # Already exists

        try:
            logs_client.create_log_stream(logGroupName=log_group, logStreamName=log_stream)
        except Exception:
            pass  # Already exists

        timestamp_ms = int(time.time() * 1000)
        logs_client.put_log_events(
            logGroupName=log_group,
            logStreamName=log_stream,
            logEvents=[
                {
                    "timestamp": timestamp_ms,
                    "message": f"[INFO] Test log from Lambda at {timestamp_ms}",
                },
                {"timestamp": timestamp_ms + 1, "message": "[DEBUG] This is a debug message"},
                {"timestamp": timestamp_ms + 2, "message": "[ERROR] This is an error message"},
                {
                    "timestamp": timestamp_ms + 3,
                    "message": "CloudWatch Logs E2E verification successful!",
                },
            ],
        )

        return create_response(
            body={
                "success": True,
                "action": "test_cloudwatch",
                "log_stream": log_stream,
                "log_group": log_group,
            }
        )
    except Exception as exc:
        return create_response(
            status_code=500,
            body={"success": False, "error": str(exc), "action": "test_cloudwatch"},
        )
