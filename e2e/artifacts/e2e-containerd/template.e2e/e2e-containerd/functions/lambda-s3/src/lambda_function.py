"""
S3-compatible Lambda (RustFS).

A simple Lambda function providing S3 API operations.
"""

import json
import os
from datetime import datetime, timezone

import boto3
from common.utils import create_response, handle_ping, parse_event_body


def lambda_handler(event, context):
    # RIE Heartbeat
    if ping_response := handle_ping(event):
        return ping_response

    # Get Trace ID from environment variables.
    trace_id = os.environ.get("_X_AMZN_TRACE_ID", "not-found")

    # Get user information.
    username = (
        event.get("requestContext", {}).get("authorizer", {}).get("cognito:username", "anonymous")
    )

    body = parse_event_body(event)
    action = body.get("action", "test")

    # Structured log output.
    timestamp = datetime.now(timezone.utc).isoformat(timespec="milliseconds")
    print(
        json.dumps(
            {
                "_time": timestamp,
                "level": "INFO",
                "trace_id": trace_id,
                "message": f"Received event: action={action}",
                "function": "s3-integration",
            }
        )
    )

    bucket = body.get("bucket", "test-bucket")
    key = body.get("key", "test-key.txt")
    data = body.get("body", body.get("data", "Hello from Lambda!"))

    try:
        s3_client = boto3.client("s3")

        if action == "test":
            response = s3_client.list_buckets()
            result = {
                "action": "test",
                "success": True,
                "buckets": [b["Name"] for b in response.get("Buckets", [])],
                "user": username,
            }

        elif action == "put":
            response = s3_client.put_object(
                Bucket=bucket,
                Key=key,
                Body=data.encode("utf-8"),
                ContentType="application/octet-stream",
            )
            result = {
                "action": "put",
                "success": True,
                "bucket": bucket,
                "key": key,
                "etag": response.get("ETag"),
                "user": username,
            }

        elif action == "get":
            response = s3_client.get_object(Bucket=bucket, Key=key)
            content = response["Body"].read().decode("utf-8")
            result = {
                "action": "get",
                "success": True,
                "bucket": bucket,
                "key": key,
                "content": content,
                "user": username,
            }

        elif action == "list":
            response = s3_client.list_objects_v2(Bucket=bucket, Prefix=body.get("prefix", ""))
            result = {
                "action": "list",
                "success": True,
                "bucket": bucket,
                "objects": [obj["Key"] for obj in response.get("Contents", [])],
                "user": username,
            }

        elif action == "delete":
            s3_client.delete_object(Bucket=bucket, Key=key)
            result = {
                "action": "delete",
                "success": True,
                "bucket": bucket,
                "key": key,
                "user": username,
            }

        elif action == "presign_get":
            expires_in = int(body.get("expires_in", 300))
            presigned_url = s3_client.generate_presigned_url(
                ClientMethod="get_object",
                Params={"Bucket": bucket, "Key": key},
                ExpiresIn=expires_in,
            )
            result = {
                "action": "presign_get",
                "success": True,
                "bucket": bucket,
                "key": key,
                "expires_in": expires_in,
                "url": presigned_url,
                "user": username,
            }

        elif action == "create_bucket":
            try:
                s3_client.create_bucket(
                    Bucket=bucket,
                    CreateBucketConfiguration={"LocationConstraint": "ap-northeast-1"},
                )
                result = {
                    "action": "create_bucket",
                    "success": True,
                    "bucket": bucket,
                    "user": username,
                }
            except s3_client.exceptions.BucketAlreadyOwnedByYou:
                result = {
                    "action": "create_bucket",
                    "success": True,
                    "bucket": bucket,
                    "message": "Bucket already exists",
                    "user": username,
                }

        else:
            result = {
                "action": action,
                "success": False,
                "error": f"Unknown action: {action}",
                "user": username,
            }

        return create_response(body=result)

    except Exception as e:
        return create_response(
            status_code=500,
            body={"success": False, "error": str(e), "action": action, "user": username},
        )
