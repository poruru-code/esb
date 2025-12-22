import os
import sys
import unittest
from unittest.mock import MagicMock, patch


class TestSiteCustomize(unittest.TestCase):
    def setUp(self):
        # sitecustomize は一度インポートされるとメモリに残るため、
        # テストごとに確実にリロードされるよう削除する
        if "sitecustomize" in sys.modules:
            del sys.modules["sitecustomize"]

        # runtime パスの追加 (パスが存在する場合のみ)
        self.runtime_path = os.path.abspath(
            os.path.join(os.path.dirname(__file__), "../../runtime")
        )
        if self.runtime_path not in sys.path:
            sys.path.insert(0, self.runtime_path)

    def tearDown(self):
        if self.runtime_path in sys.path:
            sys.path.remove(self.runtime_path)
        # 後続のテストに影響を与えないよう、再度 sitecustomize を削除しておくのが安全です
        if "sitecustomize" in sys.modules:
            del sys.modules["sitecustomize"]

    def test_s3_redirection(self):
        """S3クライアント作成時にエンドポイントがローカルに向くか検証"""
        mock_boto3 = MagicMock()
        mock_client_creator = MagicMock()
        mock_boto3.client = mock_client_creator

        with patch.dict(sys.modules, {"boto3": mock_boto3, "botocore.config": MagicMock()}):
            target_endpoint = "http://localhost:9000"
            with patch.dict(os.environ, {"S3_ENDPOINT": target_endpoint}):
                import sitecustomize  # noqa: F401

                # sitecustomizeインポート時点で boto3.client がラップされているはず
                self.assertNotEqual(mock_boto3.client, mock_client_creator)

                # ユーザーコードの動作シミュレーション
                mock_boto3.client("s3")

                # オリジナルの boto3.client (mock_client_creator) が正しい引数で呼ばれたか確認
                args, kwargs = mock_client_creator.call_args
                self.assertEqual(args[0], "s3")
                self.assertEqual(kwargs.get("endpoint_url"), target_endpoint)
                self.assertFalse(kwargs.get("verify"))

    def test_lambda_redirection(self):
        """Lambdaクライアント作成時にエンドポイントがローカルに向くか検証"""
        mock_boto3 = MagicMock()
        mock_client_creator = MagicMock()
        mock_boto3.client = mock_client_creator

        with patch.dict(sys.modules, {"boto3": mock_boto3, "botocore.config": MagicMock()}):
            target_endpoint = "http://localhost:443"
            # NOTE: 既存実装に合わせて GATEWAY_INTERNAL_URL を使用する
            with patch.dict(os.environ, {"GATEWAY_INTERNAL_URL": target_endpoint}):
                import sitecustomize  # noqa: F401

                mock_boto3.client("lambda")

                args, kwargs = mock_client_creator.call_args
                self.assertEqual(args[0], "lambda")
                self.assertEqual(kwargs.get("endpoint_url"), target_endpoint)
                self.assertFalse(kwargs.get("verify"))

    def test_lambda_invoke(self):
        """Lambdaのinvoke呼び出しが意図した設定を持つクライアントで行われるか検証"""
        mock_boto3 = MagicMock()
        mock_client_instance = MagicMock()
        # invokeの戻り値をモック
        mock_client_instance.invoke.return_value = {"StatusCode": 200}

        mock_client_creator = MagicMock(return_value=mock_client_instance)
        mock_boto3.client = mock_client_creator

        with patch.dict(sys.modules, {"boto3": mock_boto3, "botocore.config": MagicMock()}):
            target_endpoint = "http://localhost:443"
            with patch.dict(os.environ, {"GATEWAY_INTERNAL_URL": target_endpoint}):
                import sitecustomize  # noqa: F401

                # クライアント作成
                client = mock_boto3.client("lambda")

                # invoke呼び出し
                response = client.invoke(FunctionName="test-func", Payload=b"{}")

                # 結果検証
                self.assertEqual(response["StatusCode"], 200)

                # クライアント作成時にエンドポイントが指定されたか確認
                args, kwargs = mock_client_creator.call_args
                self.assertEqual(args[0], "lambda")
                self.assertEqual(kwargs.get("endpoint_url"), target_endpoint)
                self.assertFalse(kwargs.get("verify"))

    def test_dynamodb_redirection(self):
        """DynamoDBクライアント作成時にエンドポイントがローカルに向くか検証"""
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
        """Logsクライアントの _make_api_call が差し替えられるか検証"""
        mock_boto3 = MagicMock()

        # オリジナルの boto3.client が返すクライアントインスタンス (Mock)
        mock_service_client_instance = MagicMock()

        # 差し替え前の _make_api_call
        original_api_call = MagicMock(return_value={"original": "response"})
        mock_service_client_instance._make_api_call = original_api_call

        # boto3.client() が呼ばれたら上記インスタンスを返すように設定
        mock_client_creator = MagicMock(return_value=mock_service_client_instance)
        mock_boto3.client = mock_client_creator

        with patch.dict(sys.modules, {"boto3": mock_boto3, "botocore.config": MagicMock()}):
            import sitecustomize  # noqa: F401

            # 1. クライアント生成 (ここで sitecustomize が _make_api_call を書き換えるはず)
            returned_client = mock_boto3.client("logs")

            # インスタンス自体は同一であることを確認
            self.assertEqual(returned_client, mock_service_client_instance)

            # 2. _make_api_call がオリジナルの Mock とは別物になっていることを確認
            self.assertNotEqual(returned_client._make_api_call, original_api_call)

            # 3. 【重要】put_log_events ではなく、差し替えられた _make_api_call を直接実行して検証する
            # MagicMock は内部でメソッド転送を行わないため、client.put_log_events() を呼んでも
            # client._make_api_call() は発火しません。

            with patch("builtins.print") as mock_print:
                # ユーザー定義のパッチ関数を直接テスト
                resp = returned_client._make_api_call(
                    "PutLogEvents",
                    {"logGroupName": "test", "logStreamName": "test", "logEvents": []},
                )

                # レスポンスがモックされたものか確認
                self.assertEqual(resp, {"nextSequenceToken": "mock-token"})

                # print が呼ばれたか確認（ログ出力を抑制して標準出力へ流す仕様の場合）
                mock_print.assert_called()


if __name__ == "__main__":
    unittest.main()
