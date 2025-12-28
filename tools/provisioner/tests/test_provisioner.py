from unittest.mock import MagicMock, patch
from tools.provisioner.main import provision_dynamodb, provision_s3


class TestProvisioner:
    """Unit tests for the provisioner."""

    @patch("tools.provisioner.main.get_dynamodb_client")
    def test_provision_dynamodb_creates_table_if_not_exists(self, mock_get_client):
        """Create a table when it does not exist."""
        mock_client = MagicMock()
        mock_get_client.return_value = mock_client

        # No existing tables.
        mock_client.list_tables.return_value = {"TableNames": []}

        tables = [
            {
                "TableName": "test-table",
                "KeySchema": [{"AttributeName": "id", "KeyType": "HASH"}],
                "AttributeDefinitions": [{"AttributeName": "id", "AttributeType": "S"}],
                "BillingMode": "PAY_PER_REQUEST",
            }
        ]

        provision_dynamodb(tables)

        mock_client.create_table.assert_called_once()
        args = mock_client.create_table.call_args[1]
        assert args["TableName"] == "test-table"

    @patch("tools.provisioner.main.get_dynamodb_client")
    def test_provision_dynamodb_skips_if_exists(self, mock_get_client):
        """Skip creation when the table already exists."""
        mock_client = MagicMock()
        mock_get_client.return_value = mock_client

        # Existing table present.
        mock_client.list_tables.return_value = {"TableNames": ["test-table"]}

        tables = [{"TableName": "test-table"}]

        provision_dynamodb(tables)

        mock_client.create_table.assert_not_called()

    @patch("tools.provisioner.main.get_s3_client")
    def test_provision_s3_creates_bucket_if_not_exists(self, mock_get_client):
        """Create a bucket when it does not exist."""
        mock_client = MagicMock()
        mock_get_client.return_value = mock_client

        # No existing buckets.
        mock_client.list_buckets.return_value = {"Buckets": []}

        buckets = [{"BucketName": "test-bucket"}]

        provision_s3(buckets)

        mock_client.create_bucket.assert_called_once_with(Bucket="test-bucket")

    @patch("tools.provisioner.main.get_s3_client")
    def test_provision_s3_skips_if_exists(self, mock_get_client):
        """Skip creation when the bucket already exists."""
        mock_client = MagicMock()
        mock_get_client.return_value = mock_client

        # Existing bucket present.
        mock_client.list_buckets.return_value = {"Buckets": [{"Name": "test-bucket"}]}

        buckets = [{"BucketName": "test-bucket"}]

        provision_s3(buckets)

        mock_client.create_bucket.assert_not_called()
