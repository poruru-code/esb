import os

import boto3
from botocore.config import Config

from e2e.runner.utils import env_key


class AWSUtils:
    """Helper class for creating AWS clients with consistent configuration."""

    @staticmethod
    def _required_env(key: str) -> str:
        value = os.environ.get(key, "").strip()
        if value == "":
            raise RuntimeError(f"{key} is required")
        return value

    @staticmethod
    def create_s3_client(s3_port=None):
        """Create a configured S3 client for RustFS."""
        if s3_port is None:
            s3_port = int(os.environ.get(env_key("PORT_S3"), 9000))

        return boto3.client(
            "s3",
            endpoint_url=f"http://localhost:{s3_port}",
            aws_access_key_id=AWSUtils._required_env("RUSTFS_ACCESS_KEY"),
            aws_secret_access_key=AWSUtils._required_env("RUSTFS_SECRET_KEY"),
            config=Config(signature_version="s3v4"),
            verify=False,
        )

    @staticmethod
    def create_dynamodb_client(db_port=None):
        """Create a configured DynamoDB client."""
        if db_port is None:
            db_port = int(os.environ.get(env_key("PORT_DATABASE"), 8001))

        return boto3.client(
            "dynamodb",
            endpoint_url=f"http://localhost:{db_port}",
            aws_access_key_id="dummy",
            aws_secret_access_key="dummy",
            region_name="us-east-1",
        )
