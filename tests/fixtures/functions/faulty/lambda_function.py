import json
import time


def lambda_handler(event, context):
    # RIE heartbeat
    if isinstance(event, dict) and event.get("ping"):
        return {"statusCode": 200, "body": "pong"}

    body = event.get("body", {})
    if isinstance(body, str):
        try:
            body = json.loads(body)
        except (ValueError, json.JSONDecodeError):
            body = {}

    action = body.get("action", "hello")
    print(f"DEBUG: Processing action='{action}'")
    import sys

    sys.stdout.flush()

    if action == "crash":
        print("DEBUG: CRASHING NOW")
        sys.stdout.flush()
        import os

        os._exit(1)

    if action == "delay":
        seconds = body.get("seconds", 5)
        print(f"DELAYING FOR {seconds} SECONDS")
        time.sleep(seconds)
        return {"statusCode": 200, "body": json.dumps({"message": f"Delayed for {seconds}s"})}

    return {
        "statusCode": 200,
        "body": json.dumps({"message": "Faulty Lambda is OK", "action": action}),
    }
