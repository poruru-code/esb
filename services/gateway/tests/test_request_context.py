"""
RequestContext のテスト (TDD Red フェーズ)

まだ実装がないため、このテストは失敗します。
"""

import pytest


def test_get_request_id_default_is_none():
    """初期状態では RequestId は None"""
    from services.common.core.request_context import get_request_id, clear_request_id

    clear_request_id()
    assert get_request_id() is None


def test_set_request_id_with_value():
    """RequestId を明示的に設定できる"""
    from services.common.core.request_context import (
        set_request_id,
        get_request_id,
        clear_request_id,
    )

    test_id = "test-request-id-123"
    result = set_request_id(test_id)
    assert result == test_id
    assert get_request_id() == test_id
    clear_request_id()


def test_set_request_id_generates_uuid():
    """RequestId を None で設定すると UUID が生成される"""
    from services.common.core.request_context import (
        set_request_id,
        get_request_id,
        clear_request_id,
    )

    result = set_request_id(None)
    assert result is not None
    assert len(result) == 36  # UUID v4 format
    assert get_request_id() == result
    clear_request_id()


def test_clear_request_id():
    """RequestId をクリアできる"""
    from services.common.core.request_context import (
        set_request_id,
        get_request_id,
        clear_request_id,
    )

    set_request_id("test-id")
    assert get_request_id() == "test-id"
    clear_request_id()
    assert get_request_id() is None


@pytest.mark.asyncio
async def test_request_id_isolation_in_async_context():
    """非同期タスク間で RequestId が分離される (ContextVar の動作確認)"""
    import asyncio
    from services.common.core.request_context import (
        set_request_id,
        get_request_id,
        clear_request_id,
    )

    async def task_a():
        set_request_id("task-a-id")
        await asyncio.sleep(0.01)
        return get_request_id()

    async def task_b():
        set_request_id("task-b-id")
        await asyncio.sleep(0.01)
        return get_request_id()

    result_a, result_b = await asyncio.gather(task_a(), task_b())
    assert result_a == "task-a-id"
    assert result_b == "task-b-id"
    clear_request_id()
