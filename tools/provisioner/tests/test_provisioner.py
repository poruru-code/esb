from unittest.mock import MagicMock, patch
from tools.provisioner.main import provision_dynamodb, provision_s3


class TestProvisioner:
    """Provisioner の単体テスト"""

    @patch("tools.provisioner.main.get_dynamodb_client")
    def test_provision_dynamodb_creates_table_if_not_exists(self, mock_get_client):
        """テーブルが存在しない場合、作成を試みる"""
        mock_client = MagicMock()
        mock_get_client.return_value = mock_client

        # 既存テーブルなし
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
        """テーブルが既に存在する場合、作成をスキップする"""
        mock_client = MagicMock()
        mock_get_client.return_value = mock_client

        # 既存テーブルあり
        mock_client.list_tables.return_value = {"TableNames": ["test-table"]}

        tables = [{"TableName": "test-table"}]

        provision_dynamodb(tables)

        mock_client.create_table.assert_not_called()

    @patch("tools.provisioner.main.get_s3_client")
    def test_provision_s3_creates_bucket_if_not_exists(self, mock_get_client):
        """バケットが存在しない場合、作成を試みる"""
        mock_client = MagicMock()
        mock_get_client.return_value = mock_client

        # 既存バケットなし
        mock_client.list_buckets.return_value = {"Buckets": []}

        buckets = [{"BucketName": "test-bucket"}]

        provision_s3(buckets)

        mock_client.create_bucket.assert_called_once_with(Bucket="test-bucket")

    @patch("tools.provisioner.main.get_s3_client")
    def test_provision_s3_skips_if_exists(self, mock_get_client):
        """バケットが既に存在する場合、作成をスキップする"""
        mock_client = MagicMock()
        mock_get_client.return_value = mock_client

        # 既存バケットあり
        mock_client.list_buckets.return_value = {"Buckets": [{"Name": "test-bucket"}]}

        buckets = [{"BucketName": "test-bucket"}]

        provision_s3(buckets)

        mock_client.create_bucket.assert_not_called()
