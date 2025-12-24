"""
Echo Lambda: シンプルな応答のみを行う軽量 Lambda

S3/DynamoDB など外部依存なしで Lambda 呼び出しをテスト可能。
"""

import json
import os
from datetime import datetime, timezone
from common.utils import handle_ping, create_response, parse_event_body
from trace_bridge import hydrate_trace_id


@hydrate_trace_id
def lambda_handler(event, context):
    trace_id = os.environ.get("_X_AMZN_TRACE_ID", "not-found")
    # RIE Heartbeat
    if ping_response := handle_ping(event):
        return ping_response

    body = parse_event_body(event)
    username = (
        event.get("requestContext", {}).get("authorizer", {}).get("cognito:username", "anonymous")
    )
    request_id = event.get("requestContext", {}).get("requestId", "unknown")

    message = f"Echo: {body.get('message', 'Hello')}"

    # 構造化ログを出力 (VictoriaLogs 検索用)
    print(
        json.dumps(
            {
                "_time": datetime.now(timezone.utc).isoformat(timespec="milliseconds"),
                "level": "INFO",
                "request_id": request_id,
                "trace_id": trace_id,
                "message": message,
                "function": "lambda-echo",
            }
        )
    )

    return create_response(
        body={
            "success": True,
            "message": message,
            "user": username,
        }
    )
