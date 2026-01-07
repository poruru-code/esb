"""
ID Specification Verification Test

Verify that:
1. Trace ID (X-Amzn-Trace-Id) is propagated consistently across components.
2. Request ID (aws_request_id) is present and is a valid UUID in both Gateway and Lambda logs.
"""

import time
import uuid
from tests.conftest import (
    query_victorialogs_by_filter,
    call_api,
    LOG_WAIT_TIMEOUT,
)


class TestIDSpecs:
    """ID Specification Verification"""

    # Unskipped for Phase 4 verification
    # @pytest.mark.skip(
    #     reason="TODO: VictoriaLogs log propagation timing issue - not Go Agent related"
    # )
    def test_id_propagation_with_chain(self, auth_token):
        """
        Verify Trace ID propagation and Request ID independence across a call chain.
        Chain: Client -> Gateway -> Lambda(integration) -> Gateway -> Lambda(echo)
        """
        # 1. Prepare unique IDs
        unique_marker = uuid.uuid4().hex[:12]
        epoch_hex = hex(int(time.time()))[2:]
        # Format: Root=1-{time}-{id};Sampled=1
        trace_id_value = f"1-{epoch_hex}-{uuid.uuid4().hex[:24]}"
        trace_id_header = f"Root={trace_id_value};Sampled=1"

        print(f"Starting Chain ID spec test with Trace ID: {trace_id_value}")

        # 2. Invoke Lambda Chain
        # Calls /api/lambda (lambda-integration), which calls lambda-echo
        response = call_api(
            "/api/lambda",
            auth_token,
            {"next_target": "lambda-echo", "message": f"Chain Verification {unique_marker}"},
            headers={"X-Amzn-Trace-Id": trace_id_header},
        )
        assert response.status_code == 200
        data = response.json()
        assert data["success"] is True

        # 3. Wait for logs in VictoriaLogs
        print(f"Waiting for chain logs with trace_id: {trace_id_value} ...")

        logs_found = {"gateway": [], "lambda-integration": [], "lambda-echo": []}

        start_time = time.time()
        while time.time() - start_time < LOG_WAIT_TIMEOUT:
            # Query by trace_id matches
            result = query_victorialogs_by_filter(
                raw_query=f'trace_id:"{trace_id_value}"', timeout=2, limit=100
            )
            hits = result.get("hits", [])

            if hits:
                for log in hits:
                    container = log.get("container_name", "")
                    job = log.get("job", "")

                    if "gateway" in container or job == "gateway":
                        logs_found["gateway"].append(log)
                    elif "lambda-integration" in container:
                        logs_found["lambda-integration"].append(log)
                    elif "lambda-echo" in container:
                        logs_found["lambda-echo"].append(log)

            # Check if we have at least one log from each component
            if (
                len(logs_found["gateway"]) > 0
                and len(logs_found["lambda-integration"]) > 0
                and len(logs_found["lambda-echo"]) > 0
            ):
                break

            time.sleep(2)

        # 4. Verifications

        # 4.1 Trace ID Propagation (Global)
        assert len(logs_found["gateway"]) > 0, "Gateway logs missing for Chain Trace ID"
        assert len(logs_found["lambda-integration"]) > 0, (
            "Lambda-integration logs missing for Chain Trace ID"
        )
        assert len(logs_found["lambda-echo"]) > 0, "Lambda-echo logs missing for Chain Trace ID"

        # 4.2 Request ID Scope (Local)
        # Extract Request IDs
        integration_req_ids = {
            log.get("aws_request_id")
            for log in logs_found["lambda-integration"]
            if log.get("aws_request_id")
        }
        echo_req_ids = {
            log.get("aws_request_id")
            for log in logs_found["lambda-echo"]
            if log.get("aws_request_id")
        }

        print(f"Integration Request IDs: {integration_req_ids}")
        print(f"Echo Request IDs: {echo_req_ids}")

        assert integration_req_ids, "Lambda-integration missing aws_request_id"
        assert echo_req_ids, "Lambda-echo missing aws_request_id"

        # Validation: Valid UUIDs
        for rid in integration_req_ids | echo_req_ids:
            assert self._is_valid_uuid(rid), f"Invalid UUID format: {rid}"

        # Validation: Independence
        # The Request ID for integration should be different from echo
        # (Since they are separate invocations)
        assert integration_req_ids.isdisjoint(echo_req_ids), (
            f"Request ID collision detected between hops! Integration: {integration_req_ids}, Echo: {echo_req_ids}"
        )

    def _is_valid_uuid(self, val):
        try:
            uuid.UUID(str(val))
            return True
        except ValueError:
            return False
