import os
import json
import boto3
import logging
from trace_bridge import hydrate_trace_id
from common.utils import handle_ping, parse_event_body, create_response

logger = logging.getLogger()
logger.setLevel(logging.INFO)


@hydrate_trace_id
def lambda_handler(event, context):
    # RIE Heartbeat
    if ping_response := handle_ping(event):
        return ping_response

    # 環境変数から Trace ID を取得
    trace_id = os.environ.get("_X_AMZN_TRACE_ID", "not-found")
    logger.info(f"Trace ID in environment: {trace_id}")

    # Context からの Request ID も記録
    aws_request_id = context.aws_request_id
    logger.info(f"AWS Request ID in context: {aws_request_id}")

    # ボディのパース (API Gateway or Direct)
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
            "aws_request_id": aws_request_id,
            "child": child_info,
        }
    )
