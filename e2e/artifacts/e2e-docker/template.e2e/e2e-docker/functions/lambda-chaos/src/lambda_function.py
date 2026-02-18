import os
import sys
import time

from common.utils import create_response, handle_ping, parse_event_body


def lambda_handler(event, context):
    # RIE heartbeat
    if ping_response := handle_ping(event):
        return ping_response

    body = parse_event_body(event)
    action = body.get("action", "hello")
    print(f"DEBUG: Processing action='{action}'")
    sys.stdout.flush()

    if action == "crash":
        print("DEBUG: CRASHING NOW")
        sys.stdout.flush()
        os._exit(1)

    if action == "delay":
        seconds = body.get("seconds", 5)
        print(f"DELAYING FOR {seconds} SECONDS")
        time.sleep(seconds)
        return create_response(body={"message": f"Delayed for {seconds}s"})

    return create_response(body={"message": "Faulty Lambda is OK", "action": action})
