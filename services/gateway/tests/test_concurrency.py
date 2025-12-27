import asyncio
import pytest
from services.gateway.core.concurrency import FunctionThrottle, ConcurrencyManager
from services.gateway.core.exceptions import ResourceExhaustedError


@pytest.mark.asyncio
async def test_fifo_ordering():
    """
    limit=1 の状態で A, B, C を投入し、A 完了後に B が開始し、B 完了後に C が開始することを確認する。
    (厳密な FIFO の検証)
    """
    throttle = FunctionThrottle(limit=1)
    execution_order = []

    async def worker(name, delay):
        async with throttle:
            execution_order.append(f"{name}_start")
            await asyncio.sleep(delay)
            execution_order.append(f"{name}_end")

    # A を開始 (枠を占有)
    task_a = asyncio.create_task(worker("A", 0.1))
    await asyncio.sleep(0.01)  # A が確実に acquire するまで待機

    # B, C を待機列に投入
    task_b = asyncio.create_task(worker("B", 0.01))
    task_c = asyncio.create_task(worker("C", 0.01))

    await asyncio.gather(task_a, task_b, task_c)

    # 期待される順序: A開始 -> A終了 -> B開始 -> B終了 -> C開始 -> C終了
    expected = ["A_start", "A_end", "B_start", "B_end", "C_start", "C_end"]
    assert execution_order == expected


@pytest.mark.asyncio
async def test_timeout_raises_resource_exhausted():
    """
    枠が一杯の状態で、指定時間内に空きが出ない場合に ResourceExhaustedError が発生することを確認する。
    """
    throttle = FunctionThrottle(limit=1)

    # 枠を埋める
    await throttle.acquire(timeout=1.0)

    # 2つ目のリクエストはタイムアウトするはず
    with pytest.raises(ResourceExhaustedError) as excinfo:
        await throttle.acquire(timeout=0.1)

    assert "Request timed out in queue" in str(excinfo.value)


@pytest.mark.asyncio
async def test_cancel_removes_from_waiters():
    """
    待機中のタスクがキャンセルされた場合、waiters から正しく削除されることを確認する。
    """
    throttle = FunctionThrottle(limit=1)
    await throttle.acquire(timeout=1.0)  # 1つ目

    # 2つ目を待機状態にする
    waiter_task = asyncio.create_task(throttle.acquire(timeout=1.0))
    await asyncio.sleep(0.01)

    assert len(throttle.waiters) == 1

    # キャンセル
    waiter_task.cancel()
    try:
        await waiter_task
    except asyncio.CancelledError:
        pass

    assert len(throttle.waiters) == 0


@pytest.mark.asyncio
async def test_concurrency_manager_singleton_per_function():
    """
    ConcurrencyManager が同じ関数名に対して同じ FunctionThrottle を返すことを確認する。
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
    ConcurrencyManager が FunctionRegistry からリミットを取得することを確認する。
    """
    from unittest.mock import MagicMock

    mock_registry = MagicMock()
    # mock_registry.get_function_config.return_value に相当する設定
    # 1. scaling.max_capacity がある場合
    mock_registry.get_function_config.side_effect = lambda name: {
        "func_with_scaling": {"scaling": {"max_capacity": 2}},
        "func_with_reserved": {"ReservedConcurrentExecutions": 3},
        "func_default": {},
    }.get(name)

    manager = ConcurrencyManager(
        default_limit=10, default_timeout=10, function_registry=mock_registry
    )

    # 1. scaling.max_capacity 優先
    assert manager.get_throttle("func_with_scaling").limit == 2
    # 2. ReservedConcurrentExecutions
    assert manager.get_throttle("func_with_reserved").limit == 3
    # 3. Default
    assert manager.get_throttle("func_default").limit == 10
    # 4. Unknown
    assert manager.get_throttle("unknown").limit == 10
