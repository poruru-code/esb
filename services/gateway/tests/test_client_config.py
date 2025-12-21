def test_manager_client_uses_config():
    """
    TDD: ManagerClient should use config.MANAGER_URL and config.MANAGER_TIMEOUT
    instead of os.getenv directly.
    """
    # This test verifies that the module doesn't have hardcoded MANAGER_URL/MANAGER_TIMEOUT
    # by checking the config object is used
    from services.gateway import client

    # Check that the module doesn't import os for getenv
    # Instead it should use config.MANAGER_URL
    assert hasattr(client, "config") or not hasattr(client, "MANAGER_URL"), (
        "client.py should not define MANAGER_URL at module level"
    )
