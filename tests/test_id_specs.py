"""
ID Specification Verification Test

Verify that:
1. Trace ID (X-Amzn-Trace-Id) is propagated consistently across components.
2. Request ID (aws_request_id) is present and is a valid UUID in both Gateway and Lambda logs.
"""

import time
import uuid
from tests.fixtures.conftest import (
    query_victorialogs_by_filter,
    call_api,
    LOG_WAIT_TIMEOUT,
)


class TestIDSpecs:
    """ID Specification Verification"""

    def test_id_propagation_and_format(self, auth_token):
        """
        Verify Trace ID propagation and Request ID format.
        """
        # 1. Prepare unique IDs
        unique_marker = uuid.uuid4().hex[:12]
        epoch_hex = hex(int(time.time()))[2:]
        # Format: Root=1-{time}-{id};Sampled=1
        trace_id_value = f"1-{epoch_hex}-{uuid.uuid4().hex[:24]}"
        trace_id_header = f"Root={trace_id_value};Sampled=1"

        print(f"Starting ID spec test with Trace ID: {trace_id_value}")
        print(f"Unique Marker: {unique_marker}")

        # 2. Invoke Lambda via Gateway
        # Using /api/echo (lambda-connectivity) as it's simple
        response = call_api(
            "/api/echo",
            auth_token,
            {"message": f"ID Verification {unique_marker}"},
            headers={"X-Amzn-Trace-Id": trace_id_header},
        )
        assert response.status_code == 200

        # 3. Wait for logs in VictoriaLogs
        print(f"Waiting for logs with trace_id: {trace_id_value} ...")

        # We need logs from both 'gateway' and 'lambda' jobs/containers
        # Filter by trace_id matches
        found_gateway_log = False
        found_lambda_log = False
        gateway_request_id = None
        lambda_request_id = None

        start_time = time.time()
        while time.time() - start_time < LOG_WAIT_TIMEOUT:
            # Query by trace_id matches
            result = query_victorialogs_by_filter(
                raw_query=f'trace_id:"{trace_id_value}"', timeout=2, limit=50
            )
            hits = result.get("hits", [])

            if hits:
                for log in hits:
                    job = log.get("job", "")
                    container = log.get("container_name", "")

                    # Identify Gateway logs
                    if "gateway" in container or job == "gateway":
                        found_gateway_log = True
                        if "aws_request_id" in log:
                            gateway_request_id = log["aws_request_id"]

                    # Identify Lambda logs
                    if "lambda" in container or job == "lambda":
                        found_lambda_log = True
                        if "aws_request_id" in log:
                            lambda_request_id = log["aws_request_id"]

            if found_gateway_log and found_lambda_log and gateway_request_id and lambda_request_id:
                break

            time.sleep(2)

        # 4. Verifications

        # 4.1 Trace ID Propagation
        assert found_gateway_log, "Gateway logs not found for the Trace ID"
        assert found_lambda_log, "Lambda logs not found for the Trace ID"

        # 4.2 Request ID Presence & Format (Gateway)
        print(f"Gateway Request ID: {gateway_request_id}")
        assert gateway_request_id, "Gateway log missing aws_request_id"
        assert self._is_valid_uuid(gateway_request_id), (
            f"Gateway Request ID is not a valid UUID: {gateway_request_id}"
        )

        # 4.3 Request ID Presence & Format (Lambda)
        print(f"Lambda Request ID: {lambda_request_id}")
        assert lambda_request_id, "Lambda log missing aws_request_id"
        assert self._is_valid_uuid(lambda_request_id), (
            f"Lambda Request ID is not a valid UUID: {lambda_request_id}"
        )

        # 4.4 Scope verification
        # Currently, Gateway generates a Request ID and Lambda Runtime generates/receives one.
        # Strict AWS behavior: They are different (Api Gateway Request ID vs Lambda Request ID).
        # Our implementation:
        # Gateway generates ID -> sends in context.
        # Lambda Runtime (sitecustomize) captures 'invoke_id'.
        # If sitecustomize captures the one passed from Gateway (via headers -> environment? or just invoke response headers?), they might align.
        # But commonly Lambda Runtime gets 'InvokeID' from the RIE/Slice API.

        if gateway_request_id == lambda_request_id:
            print("[INFO] Gateway and Lambda Request IDs are IDENTICAL.")
        else:
            print("[INFO] Gateway and Lambda Request IDs are DISTINCT (Independent scopes).")

    def _is_valid_uuid(self, val):
        try:
            uuid.UUID(str(val))
            return True
        except ValueError:
            return False
