import os

import boto3
from botocore.config import Config


class AWSUtils:
    """Helper class for creating AWS clients with consistent configuration."""

    @staticmethod
    def create_s3_client(s3_port=None):
        """Create a configured S3 client for RustFS."""
        if s3_port is None:
            s3_port = int(os.environ.get("ESB_PORT_S3", 9000))

        return boto3.client(
            "s3",
            endpoint_url=f"http://localhost:{s3_port}",
            aws_access_key_id=os.environ.get("RUSTFS_ACCESS_KEY", "esb"),
            aws_secret_access_key=os.environ.get("RUSTFS_SECRET_KEY", "esb"),
            config=Config(signature_version="s3v4"),
            verify=False,
        )

    @staticmethod
    def create_dynamodb_client(db_port=None):
        """Create a configured DynamoDB client."""
        if db_port is None:
            db_port = int(os.environ.get("ESB_PORT_DATABASE", 8001))

        return boto3.client(
            "dynamodb",
            endpoint_url=f"http://localhost:{db_port}",
            aws_access_key_id="dummy",
            aws_secret_access_key="dummy",
            region_name="us-east-1",
        )
