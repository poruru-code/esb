import asyncio
import pytest
from services.gateway.core.concurrency import FunctionThrottle, ConcurrencyManager
from services.gateway.core.exceptions import ResourceExhaustedError


@pytest.mark.asyncio
async def test_fifo_ordering():
    """
    With limit=1, submit A, B, C and ensure B starts after A, and C after B.
    (Strict FIFO verification)
    """
    throttle = FunctionThrottle(limit=1)
    execution_order = []

    async def worker(name, delay):
        async with throttle:
            execution_order.append(f"{name}_start")
            await asyncio.sleep(delay)
            execution_order.append(f"{name}_end")

    # Start A (occupy the slot).
    task_a = asyncio.create_task(worker("A", 0.1))
    await asyncio.sleep(0.01)  # Wait for A to acquire.

    # Queue B and C.
    task_b = asyncio.create_task(worker("B", 0.01))
    task_c = asyncio.create_task(worker("C", 0.01))

    await asyncio.gather(task_a, task_b, task_c)

    # Expected order: A_start -> A_end -> B_start -> B_end -> C_start -> C_end
    expected = ["A_start", "A_end", "B_start", "B_end", "C_start", "C_end"]
    assert execution_order == expected


@pytest.mark.asyncio
async def test_timeout_raises_resource_exhausted():
    """
    Ensure ResourceExhaustedError is raised when no slot frees within timeout.
    """
    throttle = FunctionThrottle(limit=1)

    # Fill the slot.
    await throttle.acquire(timeout=1.0)

    # Second request should time out.
    with pytest.raises(ResourceExhaustedError) as excinfo:
        await throttle.acquire(timeout=0.1)

    assert "Request timed out in queue" in str(excinfo.value)


@pytest.mark.asyncio
async def test_cancel_removes_from_waiters():
    """
    Ensure a cancelled waiting task is removed from waiters.
    """
    throttle = FunctionThrottle(limit=1)
    await throttle.acquire(timeout=1.0)  # First

    # Put second into waiting state.
    waiter_task = asyncio.create_task(throttle.acquire(timeout=1.0))
    await asyncio.sleep(0.01)

    assert len(throttle.waiters) == 1

    # Cancel.
    waiter_task.cancel()
    try:
        await waiter_task
    except asyncio.CancelledError:
        pass

    assert len(throttle.waiters) == 0


@pytest.mark.asyncio
async def test_concurrency_manager_singleton_per_function():
    """
    Ensure ConcurrencyManager returns the same FunctionThrottle for the same function.
    """
    manager = ConcurrencyManager(default_limit=5, default_timeout=10)
    throttle1 = manager.get_throttle("func1")
    throttle2 = manager.get_throttle("func1")
    throttle3 = manager.get_throttle("func2")

    assert throttle1 is throttle2
    assert throttle1 is not throttle3
    assert throttle1.limit == 5


@pytest.mark.asyncio
async def test_concurrency_manager_with_registry():
    """
    Ensure ConcurrencyManager gets limits from FunctionRegistry.
    """
    from unittest.mock import MagicMock

    mock_registry = MagicMock()
    # Config equivalent to mock_registry.get_function_config return value.
    # 1. scaling.max_capacity present
    mock_registry.get_function_config.side_effect = lambda name: {
        "func_with_scaling": {"scaling": {"max_capacity": 2}},
        "func_with_reserved": {"ReservedConcurrentExecutions": 3},
        "func_default": {},
    }.get(name)

    manager = ConcurrencyManager(
        default_limit=10, default_timeout=10, function_registry=mock_registry
    )

    # 1. Prefer scaling.max_capacity.
    assert manager.get_throttle("func_with_scaling").limit == 2
    # 2. ReservedConcurrentExecutions
    assert manager.get_throttle("func_with_reserved").limit == 3
    # 3. Default
    assert manager.get_throttle("func_default").limit == 10
    # 4. Unknown
    assert manager.get_throttle("unknown").limit == 10
