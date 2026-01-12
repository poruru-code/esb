"""
Scheduler integration tests.
"""

import time

import pytest

from e2e.conftest import (
    call_api,
)


class TestScheduler:
    """Verify Scheduler functionality."""

    @pytest.mark.slow
    def test_schedule_trigger(self, auth_token):
        """E2E: Verify that scheduled function is triggered and writes to DynamoDB."""
        # The function 'lambda-scheduled' writes to 'e2e-test-table' with id='scheduled-run'
        # when triggered by 'rate(1 minute)'.

        print("Waiting for scheduled function to trigger (rate(1 minute))...")
        # We wait up to 90 seconds.
        # Usually it triggers around 60s if apscheduler starts at the same time as the test.
        max_wait = 90
        check_interval = 5
        elapsed = 0

        found = False
        while elapsed < max_wait:
            # Check DynamoDB via lambda-dynamo proxy
            # lambda-dynamo is already tested in test_dynamo.py and should work.
            response = call_api(
                "/api/dynamo",
                auth_token,
                {"action": "get", "id": "scheduled-run"},
            )

            if response.status_code == 200:
                data = response.json()
                if data.get("found"):
                    item = data["item"]
                    last_run = item.get("last_run", {}).get("S", "unknown")
                    print(f"Scheduled function execution confirmed at {last_run}")

                    # Verify input was passed
                    event = item.get("event", {}).get("M", {})
                    assert event.get("scheduled", {}).get("BOOL") is True

                    found = True
                    break

            time.sleep(check_interval)
            elapsed += check_interval
            if elapsed % 15 == 0:
                print(f"Still waiting for schedule... ({elapsed}/{max_wait}s)")

        if not found:
            pytest.fail(
                "Scheduled function 'lambda-scheduled' did not trigger within 90 seconds. Check gateway logs."
            )
