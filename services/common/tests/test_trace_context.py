"""
TraceContext tests (unified Trace ID)
"""

import pytest
from services.common.core.request_context import (
    get_trace_id,
    set_trace_id,
    clear_trace_id,
)


def test_get_trace_id_default_is_none():
    """TraceId is None by default."""
    clear_trace_id()
    assert get_trace_id() is None


def test_set_trace_id_with_valid_string():
    """TraceId (Root=...) can be set explicitly."""
    trace_str = "Root=1-676b8f34-e432f8314483756f7098e60b;Sampled=1"
    result = set_trace_id(trace_str)
    assert result == trace_str
    assert get_trace_id() == trace_str
    clear_trace_id()


def test_clear_trace_id():
    """TraceId can be cleared."""
    trace_str = "Root=1-676b8f34-e432f8314483756f7098e60b;Sampled=1"
    set_trace_id(trace_str)
    assert get_trace_id() == trace_str

    clear_trace_id()
    assert get_trace_id() is None


@pytest.mark.asyncio
async def test_trace_id_isolation_in_async_context():
    """TraceId is isolated across async tasks (ContextVar behavior)."""
    import asyncio

    trace_a = "Root=1-aaaaaaaa-aaaaaaaaaaaaaaaaaaaaaaaa;Sampled=1"
    trace_b = "Root=1-bbbbbbbb-bbbbbbbbbbbbbbbbbbbbbbbb;Sampled=1"

    async def task_a():
        set_trace_id(trace_a)
        await asyncio.sleep(0.01)
        return get_trace_id()

    async def task_b():
        set_trace_id(trace_b)
        await asyncio.sleep(0.01)
        return get_trace_id()

    result_a, result_b = await asyncio.gather(task_a(), task_b())
    assert result_a == trace_a
    assert result_b == trace_b
    clear_trace_id()
