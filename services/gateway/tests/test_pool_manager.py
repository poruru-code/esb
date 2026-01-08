"""
Tests for PoolManager class

TDD: RED phase - write tests first, then implement.
"""

from unittest.mock import AsyncMock, MagicMock

import pytest


class TestPoolManagerBasics:
    """Basic tests for PoolManager creation and pool access"""

    @pytest.fixture
    def mock_provision_client(self):
        """Mock provision client"""
        client = MagicMock()
        client.provision = AsyncMock()
        return client

    @pytest.fixture
    def mock_config_loader(self):
        """Mock config loader that returns scaling settings"""

        def loader(function_name):
            return {
                "scaling": {
                    "max_capacity": 3,
                    "min_capacity": 0,
                }
            }

        return loader

    @pytest.fixture
    def pool_manager(self, mock_provision_client, mock_config_loader):
        """Create a PoolManager for testing"""
        from services.gateway.services.pool_manager import PoolManager

        return PoolManager(
            provision_client=mock_provision_client,
            config_loader=mock_config_loader,
        )

    @pytest.mark.asyncio
    async def test_get_pool_creates_new_pool(self, pool_manager):
        """get_pool should create new pool for unknown function"""
        pool = await pool_manager.get_pool("test-function")

        assert pool is not None
        assert pool.function_name == "test-function"
        assert pool.max_capacity == 3

    @pytest.mark.asyncio
    async def test_get_pool_returns_same_pool(self, pool_manager):
        """get_pool should return same pool for same function"""
        pool1 = await pool_manager.get_pool("test-function")
        pool2 = await pool_manager.get_pool("test-function")

        assert pool1 is pool2

    @pytest.mark.asyncio
    async def test_get_pool_different_functions(self, pool_manager):
        """get_pool should create different pools for different functions"""
        pool1 = await pool_manager.get_pool("function-a")
        pool2 = await pool_manager.get_pool("function-b")

        assert pool1 is not pool2
        assert pool1.function_name == "function-a"
        assert pool2.function_name == "function-b"


class TestPoolManagerAcquireRelease:
    """Tests for PoolManager acquire/release/evict operations"""

    @pytest.fixture
    def mock_worker(self):
        from services.common.models.internal import WorkerInfo

        return WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")

    @pytest.fixture
    def mock_provision_client(self, mock_worker):
        """Mock provision client that returns a worker"""
        client = MagicMock()
        client.provision = AsyncMock(return_value=[mock_worker])
        return client

    @pytest.fixture
    def mock_config_loader(self):
        def loader(function_name):
            return {
                "scaling": {
                    "max_capacity": 2,
                    "min_capacity": 0,
                }
            }

        return loader

    @pytest.fixture
    def pool_manager(self, mock_provision_client, mock_config_loader):
        from services.gateway.services.pool_manager import PoolManager

        return PoolManager(
            provision_client=mock_provision_client,
            config_loader=mock_config_loader,
        )

    @pytest.mark.asyncio
    async def test_acquire_worker_provisions(self, pool_manager, mock_worker):
        """acquire_worker should call provision client"""
        worker = await pool_manager.acquire_worker("test-function")

        assert worker.id == "c1"

    @pytest.mark.asyncio
    async def test_release_worker(self, pool_manager, mock_worker):
        """release_worker should return worker to pool"""
        # First acquire
        await pool_manager.acquire_worker("test-function")

        # Then release
        await pool_manager.release_worker("test-function", mock_worker)

        pool = await pool_manager.get_pool("test-function")
        assert pool.stats["idle"] == 1

    @pytest.mark.asyncio
    async def test_evict_worker(self, pool_manager, mock_worker):
        """evict_worker should remove worker from pool tracking"""
        # Manually setup pool with worker
        await pool_manager.acquire_worker("test-function")
        await pool_manager.evict_worker("test-function", mock_worker)
        pool = await pool_manager.get_pool("test-function")
        names = pool.get_all_names()
        assert "w1" not in names


class TestPoolManagerHeartbeat:
    """Tests for PoolManager.get_all_worker_ids()"""

    @pytest.fixture
    def mock_provision_client(self):
        from services.common.models.internal import WorkerInfo

        workers = [
            WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1"),
            WorkerInfo(id="c2", name="w2", ip_address="10.0.0.2"),
        ]

        client = MagicMock()
        # Return different worker each time
        client.provision = AsyncMock(side_effect=[[w] for w in workers])
        return client

    @pytest.fixture
    def mock_config_loader(self):
        def loader(function_name):
            return {
                "scaling": {
                    "max_capacity": 5,
                    "min_capacity": 0,
                }
            }

        return loader

    @pytest.fixture
    def pool_manager(self, mock_provision_client, mock_config_loader):
        from services.gateway.services.pool_manager import PoolManager

        return PoolManager(
            provision_client=mock_provision_client,
            config_loader=mock_config_loader,
        )

    @pytest.mark.asyncio
    async def test_get_all_worker_names_empty(self, pool_manager):
        """get_all_worker_names should return empty dict initially"""
        names = pool_manager.get_all_worker_names()
        assert names == {}

    @pytest.mark.asyncio
    async def test_get_all_worker_names_with_workers(self, pool_manager):
        """get_all_worker_names should return all worker Names per function"""
        # Acquire workers for two functions
        await pool_manager.acquire_worker("function-a")
        await pool_manager.acquire_worker("function-b")

        names = pool_manager.get_all_worker_names()

        assert "function-a" in names
        assert "function-b" in names
        assert len(names["function-a"]) == 1
        assert len(names["function-b"]) == 1
        assert "w1" in names["function-a"]
        assert "w2" in names["function-b"]
