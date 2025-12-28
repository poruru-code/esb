"""
Sample Lambda function: Hello World.

Responds with the username from requestContext.
Includes CloudWatch Logs test functionality.
"""

import time
import boto3
import logging
from common.utils import handle_ping, parse_event_body, create_response

logger = logging.getLogger()
logger.setLevel(logging.INFO)


def lambda_handler(event, context):
    # Handle RIE heartbeat checks.
    if ping_response := handle_ping(event):
        return ping_response

    """
    Lambda function entry point.
    
    Args:
        event: event from API Gateway
        context: Lambda execution context
    
    Returns:
        API Gateway-compatible response
    """
    username = (
        event.get("requestContext", {}).get("authorizer", {}).get("cognito:username", "anonymous")
    )
    logger.info(f"Processing action for user: {username}")

    # Parse body for action
    body = parse_event_body(event)
    action = body.get("action", "hello")

    # CloudWatch Logs test.
    if action == "test_cloudwatch":
        try:
            logs_client = boto3.client("logs")
            log_group = "/lambda/hello-test"
            log_stream = f"test-stream-{int(time.time())}"

            # CreateLogGroup (ok if it already exists).
            try:
                logs_client.create_log_group(logGroupName=log_group)
            except Exception:
                pass  # Already exists

            # CreateLogStream
            try:
                logs_client.create_log_stream(logGroupName=log_group, logStreamName=log_stream)
            except Exception:
                pass  # Already exists

            # PutLogEvents
            timestamp_ms = int(time.time() * 1000)
            # PutLogEvents (sitecustomize.py transparently outputs to stdout).
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
        except Exception as e:
            return create_response(
                status_code=500,
                body={"success": False, "error": str(e), "action": "test_cloudwatch"},
            )

    # Default: Hello response.
    response_body = {
        "message": f"Hello, {username}!",
        "event": event,
        "function": "hello",
    }

    return create_response(body=response_body)
