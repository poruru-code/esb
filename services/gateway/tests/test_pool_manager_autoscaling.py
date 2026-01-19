from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from services.common.models.internal import WorkerInfo
from services.gateway.services.pool_manager import PoolManager


@pytest.mark.asyncio
async def test_pm_sync_with_manager():
    """Test sync_with_manager (Phase 5)"""
    from services.gateway.models.function import FunctionEntity, ScalingConfig

    mock_client = AsyncMock()
    mock_loader = MagicMock(
        return_value=FunctionEntity(
            name="func1", image="img", scaling=ScalingConfig(max_capacity=5)
        )
    )
    pm = PoolManager(mock_client, mock_loader)

    # Mock list_containers response
    w1 = WorkerInfo(id="c1", name="lambda-func1-c1", ip_address="1.1.1.1")
    mock_client.list_containers.return_value = [w1]

    # We need to ensure get_pool is called and adopt is called on the pool
    # Since get_pool is async and creates Pool, we can patch ContainerPool
    with patch("services.gateway.services.pool_manager.ContainerPool") as MockPoolCls:
        mock_pool_instance = MockPoolCls.return_value
        mock_pool_instance.adopt = AsyncMock()

        await pm.sync_with_manager()

        # Verify list_containers called
        mock_client.list_containers.assert_awaited_once()

        # Verify pool was retrieved/created for "func1"
        # We need to know how extract_function_name works.
        # Assuming name="func1-c1" -> func1 if logic exists.
        # If logic is missing in PM, this test will fail until we implement it or use simple names.
        # Current logic usually: name is just container name? Or does it parse?
        # The proposal said `_extract_function_name`.
        # We need to implement that helper too.

        # Verify adopt called
        mock_pool_instance.adopt.assert_awaited_with(w1)


@pytest.mark.asyncio
async def test_pm_shutdown_all():
    """Test shutdown_all (Phase 5)"""
    mock_client = AsyncMock()
    pm = PoolManager(mock_client, MagicMock())

    # Setup pools
    mock_pool = AsyncMock()
    w1 = WorkerInfo(id="c1", name="n1", ip_address="1.1.1.1")
    mock_pool.drain.return_value = [w1]
    pm._pools["func1"] = mock_pool

    await pm.shutdown_all()

    # Verify drain called
    mock_pool.drain.assert_awaited_once()
    # Verify delete_container called
    mock_client.delete_container.assert_awaited_with("c1")


@pytest.mark.asyncio
async def test_pm_prune_all_pools():
    """Test prune_all_pools (Phase 5)"""
    pm = PoolManager(AsyncMock(), MagicMock())

    mock_pool = AsyncMock()
    w1 = WorkerInfo(id="c1", name="n1", ip_address="1.1.1.1")
    mock_pool.prune_idle_workers.return_value = [w1]
    pm._pools["func1"] = mock_pool

    result = await pm.prune_all_pools(idle_timeout=60.0)

    assert "func1" in result
    assert result["func1"] == [w1]
    mock_pool.prune_idle_workers.assert_awaited_with(60.0)
