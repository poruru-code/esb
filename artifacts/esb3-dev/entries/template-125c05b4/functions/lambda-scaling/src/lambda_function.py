import json
import time


def lambda_handler(event, context):
    body = {}
    try:
        if "body" in event:
            if isinstance(event["body"], str):
                body = json.loads(event["body"])
            else:
                body = event["body"]
    except Exception:
        pass

    sleep_ms = body.get("sleep_ms", 0)
    if sleep_ms > 0:
        time.sleep(sleep_ms / 1000.0)

    return {
        "statusCode": 200,
        "headers": {"Content-Type": "application/json"},
        "body": json.dumps({"message": "scaling-test", "slept_ms": sleep_ms}),
    }
