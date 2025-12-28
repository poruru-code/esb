import os
import sys
import unittest
from unittest.mock import MagicMock, patch


class TestSiteCustomize(unittest.TestCase):
    def setUp(self):
        # sitecustomize persists in memory once imported, so remove it to ensure reloads.
        if "sitecustomize" in sys.modules:
            del sys.modules["sitecustomize"]

        # Add runtime/site-packages to the path.
        self.runtime_path = os.path.abspath(
            os.path.join(os.path.dirname(__file__), "../../runtime/site-packages")
        )
        if self.runtime_path not in sys.path:
            sys.path.insert(0, self.runtime_path)

    def tearDown(self):
        if self.runtime_path in sys.path:
            sys.path.remove(self.runtime_path)
        # Remove sitecustomize again to avoid impacting subsequent tests.
        if "sitecustomize" in sys.modules:
            del sys.modules["sitecustomize"]

    def test_s3_redirection(self):
        """Verify that S3 client endpoints point to local."""
        mock_boto3 = MagicMock()
        mock_client_creator = MagicMock()
        mock_boto3.client = mock_client_creator

        with patch.dict(sys.modules, {"boto3": mock_boto3, "botocore.config": MagicMock()}):
            target_endpoint = "http://localhost:9000"
            with patch.dict(os.environ, {"S3_ENDPOINT": target_endpoint}):
                import sitecustomize  # noqa: F401

                # boto3.client should be wrapped once sitecustomize is imported.
                self.assertNotEqual(mock_boto3.client, mock_client_creator)

                # Simulate user code behavior.
                mock_boto3.client("s3")

                # Ensure the original boto3.client was called with correct args.
                args, kwargs = mock_client_creator.call_args
                self.assertEqual(args[0], "s3")
                self.assertEqual(kwargs.get("endpoint_url"), target_endpoint)
                self.assertFalse(kwargs.get("verify"))

    def test_lambda_redirection(self):
        """Verify that Lambda client endpoints point to local."""
        mock_boto3 = MagicMock()
        mock_client_creator = MagicMock()
        mock_boto3.client = mock_client_creator

        with patch.dict(sys.modules, {"boto3": mock_boto3, "botocore.config": MagicMock()}):
            target_endpoint = "http://localhost:443"
            # NOTE: Use GATEWAY_INTERNAL_URL to match the existing implementation.
            with patch.dict(os.environ, {"GATEWAY_INTERNAL_URL": target_endpoint}):
                import sitecustomize  # noqa: F401

                mock_boto3.client("lambda")

                args, kwargs = mock_client_creator.call_args
                self.assertEqual(args[0], "lambda")
                self.assertEqual(kwargs.get("endpoint_url"), target_endpoint)
                self.assertFalse(kwargs.get("verify"))

    def test_lambda_invoke(self):
        """Verify Lambda invoke uses a client with the intended settings."""
        mock_boto3 = MagicMock()
        mock_client_instance = MagicMock()
        # Mock invoke return value.
        mock_client_instance.invoke.return_value = {"StatusCode": 200}

        mock_client_creator = MagicMock(return_value=mock_client_instance)
        mock_boto3.client = mock_client_creator

        with patch.dict(sys.modules, {"boto3": mock_boto3, "botocore.config": MagicMock()}):
            target_endpoint = "http://localhost:443"
            with patch.dict(os.environ, {"GATEWAY_INTERNAL_URL": target_endpoint}):
                import sitecustomize  # noqa: F401

                # Create client.
                client = mock_boto3.client("lambda")

                # Invoke.
                response = client.invoke(FunctionName="test-func", Payload=b"{}")

                # Verify result.
                self.assertEqual(response["StatusCode"], 200)

                # Ensure endpoint was specified during client creation.
                args, kwargs = mock_client_creator.call_args
                self.assertEqual(args[0], "lambda")
                self.assertEqual(kwargs.get("endpoint_url"), target_endpoint)
                self.assertFalse(kwargs.get("verify"))

    def test_dynamodb_redirection(self):
        """Verify that DynamoDB client endpoints point to local."""
        mock_boto3 = MagicMock()
        mock_client_creator = MagicMock()
        mock_boto3.client = mock_client_creator

        with patch.dict(sys.modules, {"boto3": mock_boto3, "botocore.config": MagicMock()}):
            target_endpoint = "http://localhost:8000"
            with patch.dict(os.environ, {"DYNAMODB_ENDPOINT": target_endpoint}):
                import sitecustomize  # noqa: F401

                mock_boto3.client("dynamodb")

                args, kwargs = mock_client_creator.call_args
                self.assertEqual(args[0], "dynamodb")
                self.assertEqual(kwargs.get("endpoint_url"), target_endpoint)

    def test_logs_patching(self):
        """Verify that _make_api_call is patched for the Logs client."""
        mock_boto3 = MagicMock()

        # Client instance (mock) returned by the original boto3.client.
        mock_service_client_instance = MagicMock()

        # _make_api_call before patching.
        original_api_call = MagicMock(return_value={"original": "response"})
        mock_service_client_instance._make_api_call = original_api_call

        # Configure boto3.client() to return the instance above.
        mock_client_creator = MagicMock(return_value=mock_service_client_instance)
        mock_boto3.client = mock_client_creator

        with patch.dict(sys.modules, {"boto3": mock_boto3, "botocore.config": MagicMock()}):
            import sitecustomize  # noqa: F401

            # 1. Create client (sitecustomize should patch _make_api_call here).
            returned_client = mock_boto3.client("logs")

            # Ensure it is the same instance.
            self.assertEqual(returned_client, mock_service_client_instance)

            # 2. Ensure _make_api_call is no longer the original mock.
            self.assertNotEqual(returned_client._make_api_call, original_api_call)

            # 3. IMPORTANT: validate by calling patched _make_api_call directly, not put_log_events.
            # MagicMock does not forward internally, so client.put_log_events() will not trigger it.

            with patch("builtins.print") as mock_print:
                # Directly test the user-defined patch function.
                resp = returned_client._make_api_call(
                    "PutLogEvents",
                    {
                        "logGroupName": "test",
                        "logStreamName": "test",
                        "logEvents": [{"timestamp": 1234567890000, "message": "test log"}],
                    },
                )

                # Verify the mocked response.
                self.assertEqual(resp, {"nextSequenceToken": "mock-token"})

                # Ensure print was called (if logs are redirected to stdout).
                mock_print.assert_called()


if __name__ == "__main__":
    unittest.main()
