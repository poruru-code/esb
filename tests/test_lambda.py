"""
Lambda 呼び出しテスト (E2E)

検証シナリオ:
1. 基本呼び出し: Client -> Gateway -> Echo
2. 同期連鎖呼び出し: Client -> Gateway -> Chain (boto3) -> Echo (Sync)
3. 非同期連鎖呼び出し: Client -> Gateway -> Chain (boto3) -> Echo (Async)
"""

import json
from tests.fixtures.conftest import (
    AUTH_USER,
    LOG_WAIT_TIMEOUT,
    query_victorialogs_by_filter,
    call_api,
)


class TestLambda:
    """Lambda 呼び出し機能の検証"""

    def test_basic_invocation(self, auth_token):
        """基本呼び出し: Client -> Gateway -> Echo"""
        response = call_api("/api/echo", auth_token, {"message": "hello-basic"})

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["message"] == "Echo: hello-basic"
        assert data["user"] == AUTH_USER

    def test_sync_chain_invoke(self, auth_token):
        """同期連鎖呼び出し: Client -> Gateway -> Chain (boto3 sync) -> Echo"""
        response = call_api("/api/call", auth_token, {"next_target": "lambda-echo"})

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        # 子(lambda-echo)のレスポンス検証
        child = data.get("child")
        assert child is not None
        assert child.get("statusCode") == 200

        child_body = json.loads(child.get("body", "{}"))
        assert child_body.get("success") is True
        assert child_body.get("message") == "Echo: from-chain"

    def test_async_chain_invoke(self, auth_token):
        """非同期連鎖呼び出し: Client -> Gateway -> Chain (boto3 async) -> Echo"""
        response = call_api("/api/call", auth_token, {"next_target": "lambda-echo", "async": True})

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        # 非同期呼び出しが開始されたことの確認
        child = data.get("child")
        assert child is not None
        assert child.get("status") == "async-started"
        assert child.get("status_code") == 202

        # Trace ID を取得して VictoriaLogs で実行を確認
        trace_id = data.get("trace_id")
        assert trace_id is not None
        root_trace_id = trace_id.split(";")[0].replace("Root=", "")

        # VictoriaLogs で lambda-echo のログを確認
        logs = query_victorialogs_by_filter(
            filters={
                "trace_id": root_trace_id,
                "container_name": "lambda-echo",
            },
            min_hits=1,
            timeout=LOG_WAIT_TIMEOUT,
        )

        assert len(logs["hits"]) >= 1, (
            f"Async execution log not found for trace_id: {root_trace_id}"
        )
        # ログに Echo メッセージが含まれるか確認 (フィールド名は message または _msg)
        found_echo = any(
            "Echo: from-chain" in hit.get("message", "")
            or "Echo: from-chain" in hit.get("_msg", "")
            for hit in logs["hits"]
        )
        assert found_echo is True, f"Echo message not found in logs: {logs['hits']}"
