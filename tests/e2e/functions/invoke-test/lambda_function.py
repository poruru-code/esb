import json
import boto3
# requests, urllib3 のインポートは不要になります

def lambda_handler(event, context):
    # RIEハートビートチェック対応 (変更なし)
    if isinstance(event, dict) and event.get("ping"):
        return {"statusCode": 200, "body": "pong"}

    print(f"Received event: {json.dumps(event)}")

    # ボディのパース (変更なし)
    body = event.get("body", {})
    if isinstance(body, str):
        try:
            body = json.loads(body)
        except json.JSONDecodeError:
            body = {}

    target_func = body.get("target")
    payload = body.get("payload", {})
    invoke_type = body.get("type", "RequestResponse")

    if not target_func:
        return {"statusCode": 400, "body": json.dumps({"error": "Target function name required"})}

    # ==================================================================
    # 変更点: requests.post ではなく boto3 を使用する
    # ==================================================================
    # sitecustomize.py により、自動的に endpoint_url が内部Gatewayに向けられます
    client = boto3.client("lambda")

    print(f"Invoking {target_func} with type {invoke_type} via boto3")

    try:
        # invokeメソッドの呼び出し
        response = client.invoke(
            FunctionName=target_func,
            InvocationType=invoke_type,
            Payload=json.dumps(payload)
        )

        # ステータスコードの取得 (HTTPステータスではなくLambda APIのレスポンスコード)
        status_code = response['StatusCode']
        print(f"Response Status: {status_code}")

        # Event (非同期) の場合
        if invoke_type == "Event":
            # 202 Accepted が返ることを期待
            success = status_code == 202
            return {
                "statusCode": 200,
                "body": json.dumps({
                    "success": success,
                    "target": target_func,
                    "type": invoke_type,
                    "status_code": status_code,
                    "message": "Async invocation started"
                })
            }

        # RequestResponse (同期) の場合
        # Payloadは StreamingBody なので read() する必要があります
        response_payload = response['Payload'].read()
        
        # レスポンスのデコード
        try:
            response_data = json.loads(response_payload)
        except Exception:
            # 文字列等の場合
            response_data = response_payload.decode('utf-8')

        return {
            "statusCode": 200,
            "body": json.dumps({
                "success": status_code == 200,
                "target": target_func,
                "type": invoke_type,
                "status_code": status_code,
                "response": response_data
            })
        }

    except Exception as e:
        print(f"Invocation failed: {e}")
        return {
            "statusCode": 500,
            "body": json.dumps({"success": False, "error": str(e), "target": target_func})
        }
