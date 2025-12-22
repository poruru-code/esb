import pytest
from unittest.mock import MagicMock, AsyncMock
from services.gateway.services.container_manager import HttpContainerManager
from services.gateway.config import GatewayConfig


@pytest.mark.asyncio
async def test_get_lambda_host_params():
    """Test that get_lambda_host sends correct parameters to manager"""
    # Arrange
    mock_client = AsyncMock()
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.json.return_value = {"host": "1.2.3.4", "port": 8080}
    mock_client.post.return_value = mock_response

    config = GatewayConfig()
    manager = HttpContainerManager(client=mock_client, config=config)

    function_name = "test-func"
    image = "test-image:latest"
    env = {"FOO": "BAR"}

    # Act
    host = await manager.get_lambda_host(function_name, image, env)

    # Assert
    assert host == "1.2.3.4"

    expected_url = f"{config.MANAGER_URL}/containers/ensure"
    mock_client.post.assert_called_once()
    call_args = mock_client.post.call_args
    assert call_args[0][0] == expected_url
    assert call_args[1]["json"] == {"function_name": function_name, "image": image, "env": env}


@pytest.mark.asyncio
async def test_get_lambda_host_failure():
    """Test behavior when manager returns error"""
    # Arrange
    mock_client = AsyncMock()
    mock_response = MagicMock()
    mock_response.status_code = 500
    mock_client.post.return_value = mock_response

    config = GatewayConfig()
    manager = HttpContainerManager(client=mock_client, config=config)

    # Act/Assert
    with pytest.raises(Exception):  # Adjust exception type as needed
        await manager.get_lambda_host("func", "img", {})
