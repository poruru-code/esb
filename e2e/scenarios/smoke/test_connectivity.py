import json
import os

import pytest
import requests

try:
    from e2e.helpers.aws_utils import AWSUtils
except ImportError:
    import sys

    sys.path.append(os.getcwd())
    from e2e.runner.aws_utils import AWSUtils

from e2e.conftest import (
    DEFAULT_REQUEST_TIMEOUT,
    GATEWAY_URL,
    VERIFY_SSL,
    VICTORIALOGS_URL,
)


class TestConnectivity:
    """Verify basic connectivity between all ESB components."""

    def test_gateway_health(self):
        """Gateway is up and responding."""
        response = requests.get(
            f"{GATEWAY_URL}/health",
            timeout=DEFAULT_REQUEST_TIMEOUT,
            verify=VERIFY_SSL,
        )
        assert response.status_code == 200, f"Gateway health check failed: {response.text}"

    def test_victorialogs_health(self):
        """VictoriaLogs is up and responding."""
        try:
            response = requests.get(
                f"{VICTORIALOGS_URL}/health",
                timeout=DEFAULT_REQUEST_TIMEOUT,
            )
            # VictoriaLogs returns 200 or empty for /health
            assert response.status_code in (200, 204), (
                f"VictoriaLogs health check failed: {response.status_code}"
            )
        except Exception as e:
            pytest.fail(f"VictoriaLogs connection failed ({VICTORIALOGS_URL}): {e}")

    def test_lambda_basic_invoke(self, gateway_health, auth_token):
        """Lambda can be invoked via Gateway (Echo function)."""
        response = requests.post(
            f"{GATEWAY_URL}/api/echo",
            json={"message": "connectivity-test"},
            headers={"Authorization": f"Bearer {auth_token}"},
            timeout=30,  # First invocation may cold-start
            verify=VERIFY_SSL,
        )
        assert response.status_code == 200, f"Lambda invocation failed: {response.text}"
        data = response.json()
        assert data.get("success") is True, f"Lambda returned error: {data}"

    def test_dynamodb_connectivity(self, gateway_health, auth_token):
        """Lambda can connect to DynamoDB (ScyllaDB)."""
        response = requests.post(
            f"{GATEWAY_URL}/api/dynamo",
            json={"action": "put", "key": "connectivity-test", "value": "ok"},
            headers={"Authorization": f"Bearer {auth_token}"},
            timeout=30,
            verify=VERIFY_SSL,
        )
        assert response.status_code == 200, f"DynamoDB connectivity failed: {response.text}"

    def test_s3_connectivity(self, gateway_health, auth_token):
        """Lambda can connect to S3 (RustFS)."""
        # Ensure bucket exists

        try:
            # Use helper to create client. Note: AWSUtils defaults to PORT_S3 (9000).
            # If we need custom logic for 13900/5343, we might need to adjust,
            # but usually PORT_S3 should be set correctly in env by now.
            # However, looking at the original code, it had specific logic for 13900.
            # Let's see if we can use AWSUtils with explicit port/endpoint if needed,
            # or if AWSUtils defaults (from env) are sufficient.
            # The original code manually constructed endpoint_url.
            # Let's try to trust AWSUtils which reads PORT_S3 from env.
            # If PORT_S3 is set, AWSUtils uses it.

            s3 = AWSUtils.create_s3_client()
            s3.create_bucket(Bucket="e2e-test-bucket")
        except Exception as e:
            # If bucket exists, it throws... but checking the error properly is better
            # For smoke test, print error but don't hard fail yet, let lambda try
            print(f"Warning: Failed to create bucket: {e}")

        response = requests.post(
            f"{GATEWAY_URL}/api/s3",
            json={
                "action": "put",
                "key": "connectivity-test.txt",
                "content": "ok",
                "bucket": "e2e-test-bucket",  # Must match created bucket
            },
            headers={"Authorization": f"Bearer {auth_token}"},
            timeout=30,
            verify=VERIFY_SSL,
        )
        assert response.status_code == 200, f"S3 connectivity failed: {response.text}"

    def test_chain_invoke(self, gateway_health, auth_token):
        """Lambda can invoke another Lambda (chain invocation)."""
        # Use lambda-integration to call lambda-echo
        response = requests.post(
            f"{GATEWAY_URL}/api/lambda",
            json={"next_target": "lambda-echo"},
            headers={"Authorization": f"Bearer {auth_token}"},
            timeout=30,
            verify=VERIFY_SSL,
        )
        assert response.status_code == 200, f"Chain invoke failed: {response.text}"
        data = response.json()
        assert data.get("success") is True, f"Chain invoke returned error: {data}"

        # Child response is nested and might be a string body that needs parsing
        child = data.get("child", {})
        if isinstance(child.get("body"), str):
            child_body = json.loads(child["body"])
            assert child_body.get("success") is True, f"Child invocation failed: {child_body}"
        else:
            assert child.get("success") is True, f"Child invocation failed: {child}"
