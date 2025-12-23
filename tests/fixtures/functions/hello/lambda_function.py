"""
サンプルLambda関数: Hello World

requestContextからユーザー名を取得して応答します。
CloudWatch Logs テスト機能も含みます。
"""

import json
import time
import boto3


def lambda_handler(event, context):
    # RIEハートビートチェック対応
    if isinstance(event, dict) and event.get("ping"):
        return {"statusCode": 200, "body": "pong"}

    """
    Lambda関数のエントリーポイント
    
    Args:
        event: API Gatewayからのイベント
        context: Lambda実行コンテキスト
    
    Returns:
        API Gateway互換のレスポンス
    """
    # requestContextからユーザー名を取得
    username = (
        event.get("requestContext", {}).get("authorizer", {}).get("cognito:username", "anonymous")
    )

    # Parse body for action
    body = event.get("body", {})
    if isinstance(body, str):
        try:
            body = json.loads(body)
        except json.JSONDecodeError:
            body = {}

    action = body.get("action", "hello")

    # CloudWatch Logs テスト
    if action == "test_cloudwatch":
        try:
            logs_client = boto3.client("logs")
            log_group = "/lambda/hello-test"
            log_stream = f"test-stream-{int(time.time())}"

            # CreateLogGroup (既存でもOK)
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
            # PutLogEvents (sitecustomize.py により透過的に stdout へ出力される)
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

            return {
                "statusCode": 200,
                "headers": {"Content-Type": "application/json"},
                "body": json.dumps(
                    {
                        "success": True,
                        "action": "test_cloudwatch",
                        "log_stream": log_stream,
                        "log_group": log_group,
                    }
                ),
            }
        except Exception as e:
            return {
                "statusCode": 500,
                "headers": {"Content-Type": "application/json"},
                "body": json.dumps(
                    {"success": False, "error": str(e), "action": "test_cloudwatch"}
                ),
            }

    # デフォルト: Hello レスポンス
    response_body = {"message": f"Hello, {username}!", "event": event, "function": "hello"}

    return {
        "statusCode": 200,
        "headers": {"Content-Type": "application/json"},
        "body": json.dumps(response_body),
    }
