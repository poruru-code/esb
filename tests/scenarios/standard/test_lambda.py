"""
Lambda invocation tests (E2E).

Scenarios:
1. Basic invocation: Client -> Gateway -> Echo
2. Sync chained invocation: Client -> Gateway -> Chain (boto3) -> Echo (Sync)
3. Async chained invocation: Client -> Gateway -> Chain (boto3) -> Echo (Async)
"""

import json
from tests.conftest import (
    AUTH_USER,
    LOG_WAIT_TIMEOUT,
    query_victorialogs_by_filter,
    call_api,
)


class TestLambda:
    """Verify Lambda invocation functionality."""

    def test_basic_invocation(self, auth_token):
        """Basic invocation: Client -> Gateway -> Echo."""
        response = call_api("/api/echo", auth_token, {"message": "hello-basic"})

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True
        assert data["message"] == "Echo: hello-basic"
        assert data["user"] == AUTH_USER

    def test_sync_chain_invoke(self, auth_token):
        """Sync chained invocation: Client -> Gateway -> Chain (boto3 sync) -> Echo."""
        response = call_api("/api/lambda", auth_token, {"next_target": "lambda-echo"})

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        # Verify child (lambda-echo) response.
        child = data.get("child")
        assert child is not None
        assert child.get("statusCode") == 200

        child_body = json.loads(child.get("body", "{}"))
        assert child_body.get("success") is True
        assert child_body.get("message") == "Echo: from-chain"

    # VictoriaLogs is now working - unskipped
    def test_async_chain_invoke(self, auth_token):
        """Async chained invocation: Client -> Gateway -> Chain (boto3 async) -> Echo."""
        response = call_api(
            "/api/lambda", auth_token, {"next_target": "lambda-echo", "async": True}
        )

        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        # Confirm async invocation started.
        child = data.get("child")
        assert child is not None
        assert child.get("status") == "async-started"
        assert child.get("status_code") == 202

        # Get Trace ID and confirm execution in VictoriaLogs.
        trace_id = data.get("trace_id")
        assert trace_id is not None
        root_trace_id = trace_id.split(";")[0].replace("Root=", "")

        # Check lambda-echo logs in VictoriaLogs.
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
        # Ensure Echo message appears in logs (field name message or _msg).
        found_echo = any(
            "Echo: from-chain" in hit.get("message", "")
            or "Echo: from-chain" in hit.get("_msg", "")
            for hit in logs["hits"]
        )
        assert found_echo is True, f"Echo message not found in logs: {logs['hits']}"
