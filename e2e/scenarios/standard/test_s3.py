"""
S3 compatibility tests (RustFS/MinIO).

- Verify S3 API compatibility
- Validate behavior with RustFS backend
"""

import uuid

import pytest

from e2e.conftest import call_api


class TestS3:
    """Verify S3 compatibility."""

    def test_put_get(self, auth_token):
        """E2E: S3 PutObject/GetObject compatibility."""
        test_key = f"test-object-{uuid.uuid4().hex[:8]}.txt"
        test_content = "Hello from E2E test!"

        # 1. PutObject
        put_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "put", "bucket": "e2e-test-bucket", "key": test_key, "body": test_content},
        )
        assert put_response.status_code == 200, f"PutObject failed: {put_response.text}"
        assert put_response.json()["success"] is True

        # 2. GetObject
        get_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "get", "bucket": "e2e-test-bucket", "key": test_key},
        )
        assert get_response.status_code == 200, f"GetObject failed: {get_response.text}"
        get_data = get_response.json()
        assert get_data["success"] is True
        assert get_data["content"] == test_content

    def test_list_objects(self, auth_token):
        """E2E: S3 ListObjects compatibility."""
        response = call_api(
            "/api/s3",
            auth_token,
            {"action": "list", "bucket": "e2e-test-bucket"},
        )
        assert response.status_code == 200, f"ListObjects failed: {response.text}"
        data = response.json()
        assert data["success"] is True
        assert "objects" in data

    def test_delete_object(self, auth_token):
        """E2E: S3 DeleteObject compatibility."""
        test_key = f"test-delete-{uuid.uuid4().hex[:8]}.txt"

        # 1. PutObject
        call_api(
            "/api/s3",
            auth_token,
            {
                "action": "put",
                "bucket": "e2e-test-bucket",
                "key": test_key,
                "body": "to be deleted",
            },
        )

        # 2. DeleteObject
        delete_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "delete", "bucket": "e2e-test-bucket", "key": test_key},
        )
        assert delete_response.status_code == 200, f"DeleteObject failed: {delete_response.text}"
        assert delete_response.json()["success"] is True

        # 3. GetObject -> error (NoSuchKey)
        get_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "get", "bucket": "e2e-test-bucket", "key": test_key},
        )
        assert get_response.status_code == 500  # NoSuchKey → 500 error

    def test_overwrite(self, auth_token):
        """E2E: S3 overwrite same key."""
        test_key = f"test-overwrite-{uuid.uuid4().hex[:8]}.txt"

        # 1. First PutObject
        call_api(
            "/api/s3",
            auth_token,
            {"action": "put", "bucket": "e2e-test-bucket", "key": test_key, "body": "original"},
        )

        # 2. Overwrite PutObject
        call_api(
            "/api/s3",
            auth_token,
            {"action": "put", "bucket": "e2e-test-bucket", "key": test_key, "body": "overwritten"},
        )

        # 3. GetObject -> verify overwritten content
        get_response = call_api(
            "/api/s3",
            auth_token,
            {"action": "get", "bucket": "e2e-test-bucket", "key": test_key},
        )
        assert get_response.status_code == 200
        assert get_response.json()["content"] == "overwritten"

    def test_list_with_prefix(self, auth_token):
        """E2E: S3 ListObjects with prefix."""
        prefix = f"prefix-test-{uuid.uuid4().hex[:8]}/"

        # Create test objects.
        for i in range(3):
            call_api(
                "/api/s3",
                auth_token,
                {
                    "action": "put",
                    "bucket": "e2e-test-bucket",
                    "key": f"{prefix}file{i}.txt",
                    "body": f"content{i}",
                },
            )

        # ListObjects with prefix
        response = call_api(
            "/api/s3",
            auth_token,
            {"action": "list", "bucket": "e2e-test-bucket", "prefix": prefix},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

    def test_bucket_lifecycle_configuration(self, auth_token):
        """E2E: S3 Bucket Lifecycle Configuration."""
        import os

        import boto3
        from botocore.config import Config

        # S3 Client for RustFS/MinIO
        storage_port = int(os.environ.get("ESB_PORT_S3", 9000))
        s3_client = boto3.client(
            "s3",
            endpoint_url=f"http://localhost:{storage_port}",
            aws_access_key_id=os.environ.get("RUSTFS_ACCESS_KEY", "esb"),
            aws_secret_access_key=os.environ.get("RUSTFS_SECRET_KEY", "esb"),
            config=Config(signature_version="s3v4"),
            verify=False,
        )

        bucket_name = "e2e-test-bucket"

        # Verify Lifecycle Configuration
        try:
            response = s3_client.get_bucket_lifecycle_configuration(Bucket=bucket_name)
            rules = response.get("Rules", [])

            assert len(rules) > 0, "Lifecycle rules should be configured"

            # Verify specific rule (e.g., Expiration: 7 days)
            found = False
            for rule in rules:
                if rule.get("Status") == "Enabled":
                    expiration = rule.get("Expiration", {})
                    if expiration.get("Days") == 7:
                        found = True
                        break

            assert found, "Expected lifecycle rule (Expiration: 7 days) not found"

        except Exception as e:
            pytest.fail(f"Failed to get bucket lifecycle configuration: {e}")
