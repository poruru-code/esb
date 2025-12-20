import json
import os
import requests
import urllib3

# Suppress insecure request warnings for self-signed certs
urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)


def lambda_handler(event, context):
    print(f"Received event: {json.dumps(event)}")

    # Environment variables
    gateway_url = os.environ.get("GATEWAY_INTERNAL_URL", "https://onpre-gateway")

    # Parse body
    body = event.get("body", {})
    if isinstance(body, str):
        try:
            body = json.loads(body)
        except json.JSONDecodeError:
            body = {}

    target_func = body.get("target")
    payload = body.get("payload", {})
    invoke_type = body.get("type", "RequestResponse")  # RequestResponse or Event

    if not target_func:
        return {"statusCode": 400, "body": json.dumps({"error": "Target function name required"})}

    # Use AWS Lambda Invocation API compatible endpoint
    # POST /2015-03-31/functions/{appName}/invocations
    invoke_url = f"{gateway_url}/2015-03-31/functions/{target_func}/invocations"

    headers = {"Content-Type": "application/json", "X-Amz-Invocation-Type": invoke_type}

    print(f"Invoking {target_func} at {invoke_url} with type {invoke_type}")

    try:
        response = requests.post(
            invoke_url,
            json=payload,
            headers=headers,
            verify=False,  # Internal calls might use self-signed certs
            timeout=30,  # 30s timeout
        )

        # Determine success
        status_code = response.status_code
        print(f"Response Status: {status_code}")

        # For Event (Async), we expect 202
        if invoke_type == "Event":
            success = status_code == 202
            return {
                "statusCode": 200,
                "body": json.dumps(
                    {
                        "success": success,
                        "target": target_func,
                        "type": invoke_type,
                        "status_code": status_code,
                        "message": "Async invocation started",
                    }
                ),
            }

        # For RequestResponse (Sync), we expect 200 and the actual response body
        # The Gateway returns the function response directly or wrapped?
        # main.py: returns Response(content=resp.content, status_code=resp.status_code...)
        # So we get the raw response from the target lambda.

        response_data = {}
        try:
            response_data = response.json()
        except Exception:
            response_data = {"raw": response.text}

        return {
            "statusCode": 200,
            "body": json.dumps(
                {
                    "success": status_code == 200,
                    "target": target_func,
                    "type": invoke_type,
                    "status_code": status_code,
                    "response": response_data,
                }
            ),
        }

    except Exception as e:
        print(f"Invocation failed: {e}")
        return {
            "statusCode": 500,
            "body": json.dumps({"success": False, "error": str(e), "target": target_func}),
        }
