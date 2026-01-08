import json
import os

import pytest
import requests

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
            assert response.status_code in (200, 204), f"VictoriaLogs health check failed: {response.status_code}"
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
        """Lambda can connect to S3 (MinIO)."""
        # Ensure bucket exists
        import boto3
        s3_endpoint = "http://localhost:13900"  # Default test port
        
        # Try to infer S3 port from environment if available
        # Note: In full-matrix-v5-ctr, s3 is often on 13900 but check if overridden
        if "GATEWAY_PORT" in os.environ and os.environ["GATEWAY_PORT"] == "5343":
             s3_endpoint = "http://localhost:13900"
        
        try:
             access_key = os.environ.get("RUSTFS_ACCESS_KEY", "rustfsadmin")
             secret_key = os.environ.get("RUSTFS_SECRET_KEY", "rustfsadmin")
             
             s3 = boto3.client(
                 "s3", 
                 endpoint_url=s3_endpoint, 
                 aws_access_key_id=access_key, 
                 aws_secret_access_key=secret_key,
                 region_name="us-east-1"
             )
             s3.create_bucket(Bucket="e2e-test-bucket")
        except Exception as e:
             # If bucket exists, it throws... but checking the error properly is better
             # For smoke test, print error but don't hard fail yet, let lambda try
             print(f"Warning: Failed to create bucket {s3_endpoint}: {e}")

        response = requests.post(
            f"{GATEWAY_URL}/api/s3",
            json={
                "action": "put", 
                "key": "connectivity-test.txt", 
                "content": "ok",
                "bucket": "e2e-test-bucket"  # Must match created bucket
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
