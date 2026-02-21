# Where: e2e/fixtures/images/lambda/python/lambda_function.py
# What: Lambda handler for image-based E2E invocation tests.
# Why: Validate sitecustomize behaviors with PackageType=Image functions.
from __future__ import annotations

import json
import time
import uuid
from typing import Any

import boto3

DEFAULT_BUCKET = "e2e-test-bucket"
DEFAULT_TARGET = "lambda-echo"


def _parse_payload(event: Any) -> dict[str, Any]:
    if not isinstance(event, dict):
        return {}

    payload: dict[str, Any] = {}
    body = event.get("body")
    if isinstance(body, str):
        try:
            parsed = json.loads(body)
        except json.JSONDecodeError:
            parsed = {}
        if isinstance(parsed, dict):
            payload.update(parsed)
    elif isinstance(body, dict):
        payload.update(body)

    for key in ("action", "message", "target", "bucket", "key", "content", "marker"):
        if key in payload:
            continue
        value = event.get(key)
        if isinstance(value, str):
            payload[key] = value
    if "async" not in payload and isinstance(event.get("async"), bool):
        payload["async"] = event["async"]

    return payload


def _extract_message(payload: dict[str, Any]) -> str:
    default = "hello-image"
    msg = payload.get("message")
    if isinstance(msg, str) and msg:
        return msg
    return default


def _invoke_lambda(payload: dict[str, Any]) -> dict[str, Any]:
    target = payload.get("target", DEFAULT_TARGET)
    if not isinstance(target, str) or not target:
        target = DEFAULT_TARGET

    invoke_type = "Event" if payload.get("async") is True else "RequestResponse"
    message = _extract_message(payload)

    client = boto3.client("lambda")
    response = client.invoke(
        FunctionName=target,
        InvocationType=invoke_type,
        Payload=json.dumps({"message": message}).encode("utf-8"),
    )

    result: dict[str, Any] = {
        "target": target,
        "invocation_type": invoke_type,
        "status_code": response.get("StatusCode"),
    }
    if invoke_type == "RequestResponse":
        raw = response["Payload"].read().decode("utf-8")
        try:
            result["child"] = json.loads(raw)
        except json.JSONDecodeError:
            result["child"] = raw
    return result


def _s3_roundtrip(payload: dict[str, Any]) -> dict[str, Any]:
    bucket = payload.get("bucket", DEFAULT_BUCKET)
    if not isinstance(bucket, str) or not bucket:
        bucket = DEFAULT_BUCKET

    key = payload.get("key")
    if not isinstance(key, str) or not key:
        key = f"image-{uuid.uuid4().hex[:8]}.txt"

    content = payload.get("content", "from-image-s3")
    if not isinstance(content, str):
        content = str(content)

    client = boto3.client("s3")
    try:
        client.create_bucket(Bucket=bucket)
    except Exception:
        pass
    client.put_object(Bucket=bucket, Key=key, Body=content.encode("utf-8"))
    response = client.get_object(Bucket=bucket, Key=key)
    fetched = response["Body"].read().decode("utf-8")

    return {
        "bucket": bucket,
        "key": key,
        "content": fetched,
    }


def _write_cloudwatch_log(marker: str) -> dict[str, Any]:
    client = boto3.client("logs")
    log_group = "/lambda/image-function"
    log_stream = f"image-stream-{int(time.time() * 1000)}"
    try:
        client.create_log_group(logGroupName=log_group)
    except Exception:
        pass
    try:
        client.create_log_stream(logGroupName=log_group, logStreamName=log_stream)
    except Exception:
        pass
    timestamp_ms = int(time.time() * 1000)
    client.put_log_events(
        logGroupName=log_group,
        logStreamName=log_stream,
        logEvents=[
            {
                "timestamp": timestamp_ms,
                "message": f"[INFO] {marker}",
            },
        ],
    )

    return {
        "marker": marker,
        "log_group": log_group,
        "log_stream": log_stream,
    }


def lambda_handler(event: Any, _context: Any) -> dict[str, Any]:
    payload = _parse_payload(event)
    action = payload.get("action", "echo")
    if not isinstance(action, str) or not action:
        action = "echo"

    response: dict[str, Any] = {
        "success": True,
        "action": action,
        "handler": "esb-e2e-lambda-python",
    }
    try:
        if action == "chain_invoke":
            response["chain"] = _invoke_lambda(payload)
        elif action == "s3_roundtrip":
            response["s3"] = _s3_roundtrip(payload)
        elif action == "test_cloudwatch":
            marker = payload.get("marker")
            if not isinstance(marker, str) or not marker:
                marker = f"image-cloudwatch-{uuid.uuid4().hex}"
            response["cloudwatch"] = _write_cloudwatch_log(marker)
            print(
                json.dumps(
                    {
                        "level": "INFO",
                        "message": marker,
                        "function": "lambda-image",
                    }
                )
            )
        else:
            response["message"] = _extract_message(payload)
    except Exception as exc:
        return {
            "success": False,
            "action": action,
            "handler": "esb-e2e-lambda-python",
            "error": str(exc),
        }

    return response
