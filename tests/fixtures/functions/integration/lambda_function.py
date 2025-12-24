from common.utils import handle_ping, parse_event_body, create_response
from handlers import s3, dynamo, invoker
from trace_bridge import hydrate_trace_id


@hydrate_trace_id
def lambda_handler(event, context):
    # RIE Heartbeat
    if ping_response := handle_ping(event):
        return ping_response

    # Dispatch based on Path or Action
    path = event.get("path", "")
    body = parse_event_body(event)
    action = body.get("action", "")

    # Router Logic
    if (
        path == "/api/s3"
        or action.startswith("s3-")
        or action in ["put", "get", "list", "create_bucket", "test"]
    ):
        return s3.handle(event, context)

    if (
        path == "/api/dynamo" or action.startswith("dynamo-") or "id" in body
    ):  # Heuristic for dynamo
        return dynamo.handle(event, context)

    if (
        path == "/api/invoke"
        or action == "invoke"
        or "target" in body  # Direct payload for invoker
    ):
        return invoker.handle(event, context)

    # Default / Fallback
    return create_response(
        status_code=400, body={"error": "Unknown path or action", "path": path, "action": action}
    )
