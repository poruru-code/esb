"""
オブザーバビリティ機能テスト

- ログ品質とレベル制御
- CloudWatch Logs 透過的リダイレクト
"""

import json
import time
import uuid

import requests

from tests.conftest import (
    GATEWAY_URL,
    VERIFY_SSL,
    LOG_WAIT_TIMEOUT,
    query_victorialogs,
    query_victorialogs_by_filter,
    call_api,
)
import pytest


class TestObservability:
    """ロギング・オブザーバビリティ機能の検証"""

    # Unskipped for Phase 4 verification
    @pytest.mark.skip(reason="TODO: Go Agent log driver config not implemented")
    def test_structured_log_format(self, auth_token):
        """
        E2E: ロギングの品質と環境変数によるレベル制御の検証

        Echo Lambda を使用してシンプルにログ品質を確認。
        """

        # 検証用のユニークな Trace ID とメッセージ
        epoch_hex = hex(int(time.time()))[2:]
        unique_id = uuid.uuid4().hex[:24]
        trace_id = f"Root=1-{epoch_hex}-{unique_id};Sampled=1"
        root_id = f"1-{epoch_hex}-{unique_id}"

        # Echo Lambda を呼び出し (S3 依存なし)
        response = call_api(
            "/api/echo",
            auth_token,
            {"message": "Log quality test"},
            headers={"X-Amzn-Trace-Id": trace_id},
        )
        assert response.status_code == 200

        # Gateway コンテナのログを検索
        print(f"Waiting for logs with Root ID: {root_id} ...")

        start_time = time.time()
        found_structured_log = False
        found_debug_log = False
        found_time_field = False

        while time.time() - start_time < LOG_WAIT_TIMEOUT:
            logs = query_victorialogs(root_id, timeout=1)

            hits = logs.get("hits", [])
            if hits:
                for log in hits:
                    # 1. 構造化ログ（JSON）であることの確認
                    if "level" in log and ("message" in log or "_msg" in log):
                        found_structured_log = True

                    # 2. _time フィールドの確認
                    if "_time" in log:
                        ts = log["_time"]
                        if isinstance(ts, (int, float)) or (
                            isinstance(ts, str) and ts.replace(".", "").isdigit()
                        ):
                            found_time_field = True
                        elif isinstance(ts, str):
                            found_time_field = True

                    # 3. DEBUG レベルのログ確認
                    if log.get("level") == "DEBUG" or log.get("level") == "debug":
                        found_debug_log = True

            if found_structured_log and found_time_field and found_debug_log:
                break

            time.sleep(2)

        assert found_structured_log, "Structured logs (JSON) not found"
        assert found_time_field, "_time field not found or invalid"
        assert found_debug_log, "DEBUG level log not found. Check LOG_LEVEL env var."

        # UNKNOWN コンテナ名がないことを確認（Lambda 環境変数が正しく設定されていることの検証）
        unknown_logs = [log for log in hits if log.get("container_name") == "UNKNOWN"]
        assert len(unknown_logs) == 0, (
            f"Found {len(unknown_logs)} logs with container_name='UNKNOWN'. "
            "AWS_LAMBDA_FUNCTION_NAME environment variable may not be set correctly."
        )

    # Unskipped for Phase 4 verification
    @pytest.mark.skip(reason="TODO: Go Agent log driver config not implemented")
    def test_cloudwatch_logs_passthrough(self, gateway_health):
        """
        E2E: CloudWatch Logs API 透過的リダイレクト検証
        """
        # 1. Lambda 呼び出し (action=test_cloudwatch)
        invoke_url = f"{GATEWAY_URL}/2015-03-31/functions/lambda-connectivity/invocations"
        payload = {"body": '{"action": "test_cloudwatch"}'}

        response = requests.post(invoke_url, json=payload, verify=VERIFY_SSL, timeout=30)
        assert response.status_code == 200, f"Lambda invocation failed: {response.text}"

        resp_data = response.json()
        resp_body = json.loads(resp_data.get("body", "{}"))
        assert resp_body.get("success") is True, f"CloudWatch test failed: {resp_body.get('error')}"

        log_group = resp_body.get("log_group")
        log_stream = resp_body.get("log_stream")
        print(f"CloudWatch test: log_group={log_group}, log_stream={log_stream}")

        # 2. ログが VictoriaLogs に伝搬するまで待機
        time.sleep(5)

        # 3. VictoriaLogs でログを検索 (共通ヘルパーを使用)
        result = query_victorialogs_by_filter(
            raw_query=f'logger:boto3.mock AND log_group:"{log_group}" AND log_stream:"{log_stream}"',
            timeout=30,
            limit=20,
            min_hits=4,
            poll_interval=2.0,
        )
        log_entries = result.get("hits", [])
        found_logs = len(log_entries) >= 4

        assert found_logs, (
            f"CloudWatch Logs not found in VictoriaLogs for log_group={log_group}. "
            f"Found only {len(log_entries)}/4 logs. "
            "Check Gateway /aws/logs endpoint and Fluent Bit configuration."
        )

        print(f"Found {len(log_entries)} log entries in VictoriaLogs")

        for entry in log_entries:
            container_name = entry.get("container_name", "")
            assert container_name == "lambda-connectivity", (
                f"Expected container_name='lambda-connectivity', got '{container_name}'. "
                "CloudWatch Logs should be attributed to Lambda container, not Gateway."
            )

        # 5. ログレベルが正しく設定されていることを検証
        levels = [entry.get("level", "") for entry in log_entries]
        print(f"Detected levels in VictoriaLogs: {levels}")

        assert "DEBUG" in levels, "DEBUG level log not found in VictoriaLogs"
        assert "ERROR" in levels, "ERROR level log not found in VictoriaLogs"
        assert "INFO" in levels, "INFO level log not found in VictoriaLogs"

        # 6. メッセージの内容が Lambda から送信されたものか検証
        messages = [entry.get("_msg", "") for entry in log_entries]
        expected_message = "CloudWatch Logs E2E verification successful!"
        found_expected_message = any(expected_message in msg for msg in messages)
        assert found_expected_message, (
            f"Expected message '{expected_message}' not found in logs. Got messages: {messages}"
        )
