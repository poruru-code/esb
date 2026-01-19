import asyncio

import pytest

from services.common.models.internal import WorkerInfo
from services.gateway.services.container_pool import ContainerPool


@pytest.mark.asyncio
async def test_container_pool_stress_acquire_release():
    """
    Stress test for ContainerPool.
    Multiple tasks acquire and release workers concurrently.
    """
    max_capacity = 5
    pool = ContainerPool(
        function_name="stress-test", max_capacity=max_capacity, acquire_timeout=5.0
    )

    provision_count = 0

    async def provision_callback(fn):
        nonlocal provision_count
        provision_count += 1
        curr_id = provision_count
        await asyncio.sleep(0.01)  # brief delay
        return [WorkerInfo(id=f"w{curr_id}", name=f"worker-{curr_id}", ip_address="127.0.0.1")]

    async def worker_task(i):
        # Acquire
        worker = await pool.acquire(provision_callback)
        # "Use" worker
        await asyncio.sleep(0.02)
        # Release
        await pool.release(worker)
        return worker.id

    # Run many concurrent tasks
    num_tasks = 50
    tasks = [worker_task(i) for i in range(num_tasks)]

    results = await asyncio.gather(*tasks, return_exceptions=True)

    # Check for errors
    errors = [r for r in results if isinstance(r, Exception)]
    if errors:
        print(f"DEBUG: Pool stats: {pool.stats}")
        print(f"DEBUG: All workers: {list(pool._all_workers.keys())}")
        print(f"DEBUG: Idle workers: {[w.id for w in pool._idle_workers]}")
    assert not errors, f"Caught {len(errors)} errors: {errors}"

    # Verify pool state
    if len(pool._all_workers) > max_capacity:
        print(f"DEBUG: Pool stats: {pool.stats}")
        print(f"DEBUG: All workers: {list(pool._all_workers.keys())}")
        print(f"DEBUG: Idle workers: {[w.id for w in pool._idle_workers]}")
    assert len(pool._all_workers) <= max_capacity
    assert len(pool._idle_workers) == len(pool._all_workers)
    assert pool._provisioning_count == 0


@pytest.mark.asyncio
async def test_container_pool_waiter_notification():
    """
    Test specifically checking if waiters are correctly notified when slots open up.
    """
    pool = ContainerPool("test", max_capacity=1, acquire_timeout=1.0)

    async def provision_callback(fn):
        await asyncio.sleep(0.1)
        return [WorkerInfo(id="w1", name="w1", ip_address="1.1.1.1")]

    # Task A acquires and holds
    worker_a = await pool.acquire(provision_callback)

    # Task B tries to acquire (should wait)
    async def acquire_b():
        return await pool.acquire(provision_callback)

    task_b = asyncio.create_task(acquire_b())
    await asyncio.sleep(0.1)  # ensure B is waiting

    # Task A releases
    await pool.release(worker_a)

    # Task B should now succeed
    worker_b = await asyncio.wait_for(task_b, timeout=0.5)
    assert worker_b.id == "w1"


@pytest.mark.asyncio
async def test_container_pool_provision_failure_notification():
    """
    Test that failures in provision_callback notify other waiters.
    """
    pool = ContainerPool("test", max_capacity=1, acquire_timeout=1.0)

    async def failing_provision(fn):
        await asyncio.sleep(0.1)
        raise RuntimeError("failed")

    # Task A starts provisioning and fails
    async def acquire_a():
        try:
            await pool.acquire(failing_provision)
        except:
            pass

    task_a = asyncio.create_task(acquire_a())
    await asyncio.sleep(0.05)

    # Task B waits
    async def provision_b(fn):
        return [WorkerInfo(id="w2", name="w2", ip_address="2.2.2.2")]

    task_b = asyncio.create_task(pool.acquire(provision_b))

    await asyncio.wait_for(task_a, timeout=1.0)
    # Task B should succeed after Task A fails
    worker_b = await asyncio.wait_for(task_b, timeout=1.0)
    assert worker_b.id == "w2"
