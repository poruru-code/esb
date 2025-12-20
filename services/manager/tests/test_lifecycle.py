import pytest
from unittest.mock import MagicMock, patch
from services.manager.main import lifespan
import sys
import os

sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../")))


@pytest.mark.asyncio
async def test_startup_cleans_zombies():
    """Verify that startup logic prunes old containers"""

    # We mock services.manager.service.docker (or wherever main calls creation)
    # The logic is likely in Manager class or Main lifespan.
    # Plan says: main.py uses docker.from_env() directly or via Manager.
    # Logic: "prune_managed_containers" function called in lifespan.

    with patch("services.manager.main.manager") as mock_manager_instance:
        # We expect lifespan to call manager.prune_managed_containers()

        # Mock the method
        mock_manager_instance.prune_managed_containers = MagicMock()

        # Execute lifespan
        async with lifespan(None):
            pass  # App running

        # Verify call
        mock_manager_instance.prune_managed_containers.assert_called_once()


# Also we should test the Implementation of prune_managed_containers in service.py
# But that's unit test for service.py.
# Plan Step 3.1 code snippet showed integration-ish test mocking docker.from_env in main.py?
# Plan: with patch("services.manager.main.docker.from_env")
# But I implemented logic in `service.py`.
# So `main.py` should call `manager.prune_managed_containers()`.
# The test above verifies this delegation.
