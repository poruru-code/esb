import pytest
import time
from services.gateway.services.container_pool import ContainerPool
from services.common.models.internal import WorkerInfo


@pytest.mark.asyncio
async def test_pool_prune_idle_workers():
    """Test prune_idle_workers method (Phase 4)"""
    pool = ContainerPool("test-func", max_capacity=10)

    # Create workers
    w1 = WorkerInfo(id="c1", name="n1", ip_address="1.1.1.1", last_used_at=time.time() - 100)
    w2 = WorkerInfo(
        id="c2", name="n2", ip_address="1.1.1.2", last_used_at=time.time() - 10
    )  # Active

    # Manually populate pool (simulating state)
    await pool.adopt(w1)
    await pool.adopt(w2)

    # Overwrite timestamp for testing (since adopt resets it)
    w1.last_used_at = time.time() - 100
    w2.last_used_at = time.time() - 10

    # Prune (timeout=50s) -> w1 should be pruned
    pruned = await pool.prune_idle_workers(idle_timeout=50.0)

    assert len(pruned) == 1
    assert pruned[0].id == "c1"

    # Check remaining
    assert pool.size == 1
    assert w2.id in pool._all_workers


@pytest.mark.asyncio
async def test_pool_adopt():
    """Test adopt method (Phase 4)"""
    pool = ContainerPool("test-func")
    w1 = WorkerInfo(id="c1", name="n1", ip_address="1.1.1.1")

    await pool.adopt(w1)

    assert pool.size == 1
    assert w1.id in pool._all_workers
    # Check semaphore logic (adopt should decrease available slots? No, adopt adds existing resource)
    # Actually adopt adds to _all_workers and _idle_workers (makes it available)
    # But usually pool starts with capacity 0?

    # Verify it can be acquired
    # acquire requires a callback, but since we adopted, it should be in idle queue
    # so callback won't be called.
    async def mock_provision(fname):
        return []

    worker = await pool.acquire(mock_provision)
    assert worker == w1


@pytest.mark.asyncio
async def test_pool_drain():
    """Test drain method (Phase 4)"""
    pool = ContainerPool("test-func", max_capacity=10)
    w1 = WorkerInfo(id="c1", name="n1", ip_address="1.1.1.1")
    await pool.adopt(w1)

    drained = await pool.drain()

    assert len(drained) == 1
    assert drained[0] == w1
    assert pool.size == 0


# =============================================================================
# Phase 8: Concurrency Fix Tests
# =============================================================================


@pytest.mark.asyncio
async def test_adopt_respects_max_capacity():
    """
    Test that adopt consumes semaphore slots.

    Scenario:
    - Pool has max_capacity=2
    - Adopt 2 workers
    - Try to acquire a 3rd worker (should fail/timeout because no capacity left)

    Current bug: adopt doesn't consume semaphore, so 3rd acquire would succeed
    by provisioning a new container, exceeding max_capacity.
    """
    import asyncio

    pool = ContainerPool("test-func", max_capacity=2, acquire_timeout=0.5)

    # Adopt 2 workers (should consume both capacity slots)
    w1 = WorkerInfo(id="c1", name="n1", ip_address="1.1.1.1")
    w2 = WorkerInfo(id="c2", name="n2", ip_address="1.1.1.2")
    await pool.adopt(w1)
    await pool.adopt(w2)

    async def mock_list_empty(fn):
        return []

    # Acquire both workers (they're in idle queue)
    acquired1 = await pool.acquire(mock_list_empty)
    acquired2 = await pool.acquire(mock_list_empty)

    assert acquired1.id == "c1"
    assert acquired2.id == "c2"

    # Now try to acquire a 3rd (should timeout - no capacity left)
    async def mock_provision_should_not_be_called(fn):
        pytest.fail("Provision should NOT be called - capacity is full")
        return []

    with pytest.raises(asyncio.TimeoutError):
        await pool.acquire(mock_provision_should_not_be_called)


@pytest.mark.asyncio
async def test_prune_and_acquire_no_race_condition():
    """
    Test that prune and acquire don't have race conditions.

    Scenario:
    - Pool has an idle worker
    - Concurrently: prune (removes it) and acquire (tries to get it)
    - Should not result in inconsistent state or errors

    Current bug: No locking between prune and acquire operations.
    """
    import asyncio

    pool = ContainerPool("test-func", max_capacity=3, acquire_timeout=1.0)

    # Setup: Add workers to pool
    workers = []
    for i in range(3):
        w = WorkerInfo(id=f"c{i}", name=f"n{i}", ip_address=f"1.1.1.{i}")
        await pool.adopt(w)
        workers.append(w)

    # Make all workers idle (old timestamp for pruning)
    for w in workers:
        w.last_used_at = time.time() - 1000  # Very old

    # Track provision calls
    provision_called = 0

    async def mock_provision(fn):
        nonlocal provision_called
        provision_called += 1
        return [
            WorkerInfo(
                id=f"new-{provision_called}", name=f"new-{provision_called}", ip_address="2.2.2.2"
            )
        ]

    # Concurrent prune and acquire
    async def do_prune():
        await pool.prune_idle_workers(idle_timeout=100.0)  # Should prune all

    async def do_acquire():
        return await pool.acquire(mock_provision)

    # Run concurrently
    results = await asyncio.gather(
        do_prune(),
        do_acquire(),
        do_acquire(),
        return_exceptions=True,
    )

    # Verify no exceptions occurred
    for r in results:
        if isinstance(r, Exception):
            pytest.fail(f"Unexpected exception during concurrent operation: {r}")

    # After prune + 2 acquires, pool state should be consistent
    # The exact outcome depends on timing, but there should be no crash
    assert pool.size >= 0  # Basic sanity check
