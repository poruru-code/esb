"""
Lambda invocation tests (E2E).

Scenarios:
1. Sync chained invocation: Client -> Gateway -> Chain (boto3) -> Echo (Sync)
2. Async chained invocation: Client -> Gateway -> Chain (boto3) -> Echo (Async)
"""

import json

from e2e.conftest import LOG_WAIT_TIMEOUT, call_api, wait_for_victorialogs_hits


class TestLambda:
    """Verify Lambda chained invocation functionality."""

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
        hits, found_echo = wait_for_victorialogs_hits(
            filters={
                "trace_id": root_trace_id,
                "container_name": "lambda-echo",
            },
            timeout=LOG_WAIT_TIMEOUT,
            min_hits=1,
            poll_interval=0.5,
            matcher=lambda hit: "Echo: from-chain" in hit.get("message", "")
            or "Echo: from-chain" in hit.get("_msg", ""),
        )

        assert len(hits) >= 1, f"Async execution log not found for trace_id: {root_trace_id}"
        assert found_echo is True, f"Echo message not found in logs: {hits}"
