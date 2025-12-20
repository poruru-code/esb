"""
E2Eテスト: HTTP → FastAPI → Lambda RIE 統合テスト

docker compose でGatewayコンテナ（DinD環境）を起動し、
外部HTTPリクエストで完全なフローをテストします。

前提条件:
- docker compose up -d gateway でGatewayを起動済み
- 内部でLambda RIE + RustFSが自動起動

テストフロー:
1. 認証（/user/auth/v1）
2. ルーティング経由でLambda呼び出し（/api/s3/test）
"""

import os
import time
import json

import pytest
import requests
import urllib3

from gateway.app.config import config

# 自己署名証明書の警告を抑制
urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

# テスト用設定
GATEWAY_URL = os.getenv("GATEWAY_URL", "https://localhost:443")
API_KEY = config.X_API_KEY
VERIFY_SSL = False


@pytest.fixture(scope="module")
def gateway_health():
    """Gatewayのヘルスチェック"""
    for i in range(10):
        try:
            response = requests.get(f"{GATEWAY_URL}/health", timeout=5, verify=VERIFY_SSL)
            if response.status_code == 200:
                return True
            print(f"Gateway returned status: {response.status_code}")
        except Exception as e:
            print(f"Waiting for Gateway... ({i + 1}/10) Error: {e}")
            time.sleep(3)
    pytest.skip(
        f"Gateway is not running on {GATEWAY_URL}. Start with: docker compose up -d gateway"
    )


def get_auth_token() -> str:
    """認証してトークンを取得"""
    response = requests.post(
        f"{GATEWAY_URL}{config.AUTH_ENDPOINT_PATH}",
        json={"AuthParameters": {"USERNAME": config.AUTH_USER, "PASSWORD": config.AUTH_PASS}},
        headers={"x-api-key": API_KEY},
        verify=VERIFY_SSL,
    )
    assert response.status_code == 200, f"Auth failed: {response.text}"
    return response.json()["AuthenticationResult"]["IdToken"]


class TestE2E:
    """E2E統合テスト: HTTP → FastAPI → Lambda RIE"""

    def test_health(self, gateway_health):
        """E2E: ヘルスチェック"""
        response = requests.get(f"{GATEWAY_URL}/health", verify=VERIFY_SSL)
        assert response.status_code == 200
        assert response.json()["status"] == "healthy"

    def test_auth(self, gateway_health):
        """E2E: 認証フロー"""
        token = get_auth_token()
        assert token is not None
        assert len(token) > 0

    def test_routing_401(self, gateway_health):
        """E2E: 認証なし → 401"""
        response = requests.post(
            f"{GATEWAY_URL}/api/s3/test", json={"action": "test"}, verify=VERIFY_SSL
        )
        assert response.status_code == 401

    def test_routing_404(self, gateway_health):
        """E2E: 存在しないルート → 404"""
        token = get_auth_token()
        response = requests.post(
            f"{GATEWAY_URL}/api/nonexistent",
            json={"action": "test"},
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )
        assert response.status_code == 404

    def test_lambda_invocation(self, gateway_health):
        """E2E: 認証 → ルーティング → Lambda呼び出し"""
        token = get_auth_token()

        response = requests.post(
            f"{GATEWAY_URL}/api/s3/test",
            json={"action": "test"},
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )

        # Lambda RIEが起動していない場合は502になる可能性がある
        if response.status_code == 502:
            pytest.skip("Lambda RIE not available in Gateway container")

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["user"] == config.AUTH_USER

    def test_scylla_integration(self, gateway_health):
        """E2E: ScyllaDB連携テスト"""
        token = get_auth_token()

        # ScyllaDBの起動待ち（Lambdaが起動するまでリトライ）
        # WindowsのDocker Desktop (WSL2) ではScyllaDBの起動に3-5分かかる場合がある
        max_retries = 40
        for i in range(max_retries):
            try:
                response = requests.post(
                    f"{GATEWAY_URL}/api/scylla/test",
                    json={"action": "test"},
                    headers={"Authorization": f"Bearer {token}"},
                    verify=VERIFY_SSL,
                )

                if response.status_code == 200:
                    break

                print(f"Status: {response.status_code}, Body: {response.text}")

                # 500 (Application Error/DB Not Ready) or 502 (Bad Gateway) -> Retry
                if response.status_code not in [500, 502, 503, 504]:
                    break

            except requests.exceptions.ConnectionError:
                print(f"Connection error (Gateway restarting?)... ({i + 1}/{max_retries})")
                response = None  # Reset response

            print(f"Waiting for Lambda/ScyllaDB... ({i + 1}/{max_retries})")
            time.sleep(5)

        if response is None:
            pytest.fail("Lambda integration failed: No response received")

        if response.status_code != 200:
            print(f"Final Failure Response: {response.text}")

        assert response.status_code == 200
        data = response.json()
        print(f"Response Data: {data}")
        assert data["success"] is True
        assert "item_id" in data
        assert "retrieved_item" in data
        assert data["retrieved_item"]["id"]["S"] == data["item_id"]

    def test_function_invocation_sync(self, gateway_health):
        """E2E: 同期呼び出し検証 (invoke-test -> hello)"""
        token = get_auth_token()

        response = requests.post(
            f"{GATEWAY_URL}/api/invoke/test",
            json={
                "target": "lambda-hello",
                "type": "RequestResponse",
            },
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )

        # Check invoke-test execution
        if response.status_code != 200:
            print(f"Sync Invoke Failed: {response.text}")
        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        # Check inner response (hello function)
        inner_resp = data["response"]
        assert inner_resp["statusCode"] == 200
        inner_body = json.loads(inner_resp["body"])
        assert "Hello" in inner_body["message"]

    def test_function_invocation_async(self, gateway_health):
        """E2E: 非同期呼び出し検証 (invoke-test -> s3-test)"""
        token = get_auth_token()
        bucket = "async-test-bucket"
        key = f"test-{int(time.time())}.txt"

        # 1. Create bucket (Sync)
        requests.post(
            f"{GATEWAY_URL}/api/s3/test",
            json={"action": "create_bucket", "bucket": bucket},
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )

        # 2. Invoke Async (invoke-test -> s3-test)
        response = requests.post(
            f"{GATEWAY_URL}/api/invoke/test",
            json={
                "target": "lambda-s3-test",  # Target function name as registered in Gateway or just function name?
                # lambda_invoker uses name=function_name.
                # functions.yml keys are `lambda-s3-test`.
                # routing.yml mapped /api/s3/test to lambda-s3-test.
                # So target should be `lambda-s3-test`.
                # Wait, for sync test I used "hello". functions.yml has "lambda-hello".
                # routing.yml maps /api/hello -> lambda-hello.
                # invoke-test uses DIRECT API: /2015-03-31/functions/{function_name}/invocations
                # Gateway's invoke_lambda_api uses {function_name} to look up config.
                # So I MUST use the key from functions.yml. i.e., "lambda-hello" and "lambda-s3-test".
                "type": "Event",
                "payload": {
                    "body": {"action": "put", "bucket": bucket, "key": key, "data": "Async Data"}
                },
            },
            headers={"Authorization": f"Bearer {token}"},
            verify=VERIFY_SSL,
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        # Async invocation returns 202 from Gateway to invoke-test
        assert data["status_code"] == 202

        # 3. Verify Side Effect (Poll S3)
        print("Waiting for async execution...")
        time.sleep(2)  # Initial wait

        max_retries = 20
        found = False
        for i in range(max_retries):
            check_resp = requests.post(
                f"{GATEWAY_URL}/api/s3/test",
                json={"action": "get", "bucket": bucket, "key": key},
                headers={"Authorization": f"Bearer {token}"},
                verify=VERIFY_SSL,
            )
            check_data = check_resp.json()
            if check_data.get("success") is True:
                found = True
                assert check_data["content"] == "Async Data"
                break
            time.sleep(1)

        assert found, "Async execution failed: File not found in S3"


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
