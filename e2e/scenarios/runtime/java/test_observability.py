"""
Where: e2e/scenarios/runtime/java/test_observability.py
What: Java echo logging and trace E2E tests.
Why: Ensure Java runtime logs carry trace context and structured fields.
"""

import time
import uuid

from e2e.conftest import (
    LOG_WAIT_TIMEOUT,
    call_api,
    query_victorialogs_by_filter,
    wait_for_victorialogs_hits,
)


class TestJavaObservability:
    """Verify Java echo logging and trace propagation."""

    def test_java_echo_logs_and_trace(self, auth_token):
        """E2E: Java echo emits structured logs with trace id."""
        epoch_hex = hex(int(time.time()))[2:]
        unique_id = uuid.uuid4().hex[:24]
        trace_id = f"Root=1-{epoch_hex}-{unique_id};Sampled=1"
        root_trace_id = f"1-{epoch_hex}-{unique_id}"

        response = call_api(
            "/api/echo-java",
            auth_token,
            {"message": "Log quality test"},
            headers={"X-Amzn-Trace-Id": trace_id},
        )
        assert response.status_code == 200

        hits, found_echo = wait_for_victorialogs_hits(
            filters={
                "trace_id": root_trace_id,
                "container_name": "lambda-echo-java",
            },
            timeout=LOG_WAIT_TIMEOUT,
            min_hits=2,
            poll_interval=0.5,
            matcher=lambda hit: "Echo: Log quality test" in (hit.get("message") or "")
            or "Echo: Log quality test" in (hit.get("_msg") or ""),
        )
        assert hits, f"No logs found for trace_id: {root_trace_id}"
        found_debug = any(str(hit.get("level", "")).upper() == "DEBUG" for hit in hits)
        found_time = any("_time" in hit for hit in hits)

        assert found_echo, f"Echo log not found in logs: {hits}"
        assert found_debug, "DEBUG level log not found"
        assert found_time, "_time field not found in logs"

    def test_java_cloudwatch_logs_passthrough(self, auth_token):
        """E2E: Java CloudWatch Logs calls are redirected to VictoriaLogs."""
        response = call_api(
            "/api/connectivity/java",
            auth_token,
            {"action": "test_cloudwatch"},
        )
        assert response.status_code == 200, f"CloudWatch call failed: {response.text}"
        data = response.json()
        assert data.get("success") is True, f"CloudWatch returned error: {data}"

        log_group = data.get("log_group")
        log_stream = data.get("log_stream")
        assert log_group and log_stream, "Missing log_group/log_stream in response"

        time.sleep(5)

        result = query_victorialogs_by_filter(
            filters={
                "logger": "aws.logs",
                "log_group": log_group,
                "log_stream": log_stream,
            },
            timeout=LOG_WAIT_TIMEOUT,
            min_hits=4,
            limit=20,
        )
        hits = result.get("hits", [])
        assert len(hits) >= 4, f"Expected >=4 log entries, got {len(hits)}"

        for entry in hits:
            container_name = entry.get("container_name", "")
            assert container_name == "lambda-connectivity-java", (
                f"Expected container_name='lambda-connectivity-java', got '{container_name}'"
            )

        levels = [str(entry.get("level", "")).upper() for entry in hits]
        assert "DEBUG" in levels, "DEBUG level log not found"
        assert "ERROR" in levels, "ERROR level log not found"
        assert "INFO" in levels, "INFO level log not found"

        messages = [entry.get("message") or entry.get("_msg") or "" for entry in hits]
        assert any("CloudWatch Logs E2E verification successful!" in msg for msg in messages), (
            "Expected CloudWatch verification message not found"
        )
