"""
Tests for ContainerPool class

TDD: RED phase - write tests first, then implement.
"""

import pytest
import asyncio
from unittest.mock import AsyncMock


class TestContainerPoolBasics:
    """Basic tests for ContainerPool creation and properties"""

    @pytest.fixture
    def pool(self):
        """Create a ContainerPool for testing"""
        from services.gateway.services.container_pool import ContainerPool

        return ContainerPool(
            function_name="test-function",
            max_capacity=3,
            min_capacity=0,
            acquire_timeout=2.0,
        )

    def test_pool_creation(self, pool):
        """ContainerPool should be created with correct settings"""
        assert pool.function_name == "test-function"
        assert pool.max_capacity == 3
        assert pool.min_capacity == 0
        assert pool.acquire_timeout == 2.0

    def test_pool_stats_initial(self, pool):
        """Stats should reflect initial empty state"""
        stats = pool.stats
        assert stats["function_name"] == "test-function"
        assert stats["max_capacity"] == 3
        assert stats["idle"] == 0
        assert stats["total_workers"] == 0

    def test_pool_get_all_names_empty(self, pool):
        """get_all_names should return empty list when no workers"""
        assert pool.get_all_names() == []


class TestContainerPoolAcquire:
    """Tests for ContainerPool.acquire() method"""

    @pytest.fixture
    def pool(self):
        from services.gateway.services.container_pool import ContainerPool

        return ContainerPool(
            function_name="test-function",
            max_capacity=2,
            acquire_timeout=1.0,
        )

    @pytest.fixture
    def mock_worker(self):
        from services.common.models.internal import WorkerInfo

        return WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")

    @pytest.mark.asyncio
    async def test_acquire_triggers_provision(self, pool, mock_worker):
        """acquire() should call provision callback when pool is empty"""
        provision_callback = AsyncMock(return_value=[mock_worker])

        worker = await pool.acquire(provision_callback)

        provision_callback.assert_called_once_with("test-function")
        assert worker.id == "c1"

    @pytest.mark.asyncio
    async def test_acquire_registers_worker(self, pool, mock_worker):
        """acquire() should register new worker in _all_workers"""
        provision_callback = AsyncMock(return_value=[mock_worker])

        await pool.acquire(provision_callback)

        assert mock_worker.id in pool._all_workers
        assert pool.get_all_names() == ["w1"]

    @pytest.mark.asyncio
    async def test_acquire_returns_idle_first(self, pool, mock_worker):
        """acquire() should return idle worker without provisioning"""

        # Pre-populate idle queue
        pool._idle_workers.append(mock_worker)
        pool._all_workers[mock_worker.id] = mock_worker

        provision_callback = AsyncMock()

        worker = await pool.acquire(provision_callback)

        # Should not call provision
        provision_callback.assert_not_called()
        assert worker.id == "c1"

    @pytest.mark.asyncio
    async def test_acquire_timeout_when_at_capacity(self, pool, mock_worker):
        """acquire() should timeout when at max_capacity and no release"""
        from services.common.models.internal import WorkerInfo

        # Fill up capacity (simulate 2 workers acquired)
        pool._all_workers[mock_worker.id] = mock_worker
        pool._all_workers["c2"] = WorkerInfo(id="c2", name="w2", ip_address="10.0.0.2")
        
        # In Condition-based pool, fill _all_workers is enough to prevent new provision
        # but to prevent acquire from returning, we don't put them in _idle_workers.

        provision_callback = AsyncMock()

        with pytest.raises(asyncio.TimeoutError):
            await pool.acquire(provision_callback)


class TestContainerPoolRelease:
    """Tests for ContainerPool.release() method"""

    @pytest.fixture
    def pool(self):
        from services.gateway.services.container_pool import ContainerPool

        return ContainerPool(function_name="test-function", max_capacity=2)

    @pytest.mark.asyncio
    async def test_release_adds_to_idle(self, pool):
        """release() should add worker to idle queue"""
        from services.common.models.internal import WorkerInfo

        worker = WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")

        # Simulate acquire (add to all_workers)
        pool._all_workers[worker.id] = worker

        # Release
        await pool.release(worker)

        # Should be in idle queue
        assert len(pool._idle_workers) == 1
        assert pool.stats["idle"] == 1

    @pytest.mark.asyncio
    async def test_release_unlocks_waiting_acquire(self, pool):
        """release() should unblock a waiting acquire()"""
        from services.common.models.internal import WorkerInfo

        worker = WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")

        # Fill capacity
        pool._all_workers[worker.id] = worker
        pool._all_workers["c2"] = WorkerInfo(id="c2", name="w2", ip_address="10.0.0.2")

        # Start waiting acquire
        async def wait_and_acquire():
            return await pool.acquire(AsyncMock())

        acquire_task = asyncio.create_task(wait_and_acquire())

        # Give it time to start waiting
        await asyncio.sleep(0.1)

        # Release should unblock
        await pool.release(worker)

        result = await asyncio.wait_for(acquire_task, timeout=1.0)
        assert result.id == "c1"


class TestContainerPoolEvict:
    """Tests for ContainerPool.evict() method"""

    @pytest.fixture
    def pool(self):
        from services.gateway.services.container_pool import ContainerPool

        return ContainerPool(function_name="test-function", max_capacity=2)

    @pytest.mark.asyncio
    async def test_evict_removes_from_all_workers(self, pool):
        """evict() should remove worker from _all_workers"""
        from services.common.models.internal import WorkerInfo

        worker = WorkerInfo(id="c1", name="w1", ip_address="10.0.0.1")
        pool._all_workers[worker.id] = worker

        await pool.evict(worker)

        assert worker.id not in pool._all_workers
        assert pool.get_all_names() == []

    @pytest.mark.asyncio
    async def test_evict_unlocks_waiting_acquire(self, pool):
        """evict() should unblock a waiting acquire() for new provisioning"""
        from services.common.models.internal import WorkerInfo

        dead_worker = WorkerInfo(id="c_dead", name="dead", ip_address="10.0.0.99")
        new_worker = WorkerInfo(id="c_new", name="new", ip_address="10.0.0.100")

        pool._all_workers[dead_worker.id] = dead_worker
        pool._all_workers["c2"] = WorkerInfo(id="c2", name="w2", ip_address="10.0.0.2")

        # Start waiting acquire
        provision_callback = AsyncMock(return_value=[new_worker])

        async def wait_and_acquire():
            return await pool.acquire(provision_callback)

        acquire_task = asyncio.create_task(wait_and_acquire())

        # Give it time to start waiting
        await asyncio.sleep(0.1)

        # Evict should unblock and trigger provision
        await pool.evict(dead_worker)

        result = await asyncio.wait_for(acquire_task, timeout=1.0)
        assert result.id == "c_new"


class TestContainerPoolConcurrency:
    """Tests for concurrent access to ContainerPool"""

    @pytest.fixture
    def pool(self):
        from services.gateway.services.container_pool import ContainerPool

        return ContainerPool(function_name="test-function", max_capacity=3)

    @pytest.mark.asyncio
    async def test_concurrent_acquire_respects_max_capacity(self, pool):
        """Concurrent acquires should not exceed max_capacity"""
        from services.common.models.internal import WorkerInfo

        pool.acquire_timeout = 0.5

        call_count = 0

        async def provision_callback(fn):
            nonlocal call_count
            call_count += 1
            await asyncio.sleep(0.1)  # Simulate provisioning delay
            return [WorkerInfo(id=f"c{call_count}", name=f"w{call_count}", ip_address=f"10.0.0.{call_count}")]

        # Launch 5 concurrent acquires (max_capacity=3)
        tasks = [pool.acquire(provision_callback) for _ in range(5)]

        # First 3 should complete, 2 should timeout
        results = await asyncio.gather(*tasks, return_exceptions=True)

        # Count successes and timeouts
        successes = [r for r in results if not isinstance(r, Exception)]
        timeouts = [r for r in results if isinstance(r, asyncio.TimeoutError)]

        assert len(successes) == 3
        assert len(timeouts) == 2
