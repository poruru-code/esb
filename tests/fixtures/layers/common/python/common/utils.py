import json


def parse_event_body(event):
    """
    Parse the API Gateway event body and return a dict.
    For Proxy Events, parse the 'body' key; otherwise parse the event itself.
    """
    if not isinstance(event, dict):
        return {}

    # Proxy Event case.
    if "body" in event:
        body = event["body"]
        if isinstance(body, str):
            try:
                return json.loads(body)
            except (ValueError, json.JSONDecodeError):
                return {}
        return body

    # Non-proxy event (direct payload) case.
    return event


def handle_ping(event):
    """
    Handle heartbeat (ping) from RIE.
    If return value is not None, return it as the response.
    """
    if isinstance(event, dict) and event.get("ping"):
        return {"statusCode": 200, "body": "pong"}
    return None


def create_response(status_code=200, body=None, headers=None):
    """
    Create an API Gateway-compatible response dict.
    """
    if headers is None:
        headers = {"Content-Type": "application/json"}

    if body is not None and not isinstance(body, str):
        body = json.dumps(body)

    return {"statusCode": status_code, "headers": headers, "body": body}
