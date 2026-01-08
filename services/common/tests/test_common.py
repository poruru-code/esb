import asyncio
import json
import logging

import pytest

from services.common.core.logging_config import CustomJsonFormatter
from services.common.core.request_context import (
    clear_trace_id,
    get_trace_id,
    set_trace_id,
)


def test_trace_context_basic():
    clear_trace_id()
    assert get_trace_id() is None

    tid = "Root=1-6789abcd-1234567890abcdef12345678;Sampled=1"
    result = set_trace_id(tid)
    assert result == tid
    assert get_trace_id() == tid

    clear_trace_id()
    assert get_trace_id() is None


@pytest.mark.asyncio
async def test_trace_context_isolation():
    async def task(tid, delay):
        set_trace_id(tid)
        await asyncio.sleep(delay)
        return get_trace_id()

    tid1 = "Root=1-aaaaaaaa-aaaaaaaaaaaaaaaaaaaaaaaa;Sampled=1"
    tid2 = "Root=1-bbbbbbbb-bbbbbbbbbbbbbbbbbbbbbbbb;Sampled=1"

    results = await asyncio.gather(task(tid1, 0.02), task(tid2, 0.01))
    assert results[0] == tid1
    assert results[1] == tid2


def test_custom_json_formatter_with_trace():
    formatter = CustomJsonFormatter()
    log_record = logging.LogRecord(
        name="test-logger",
        level=logging.INFO,
        pathname="test.py",
        lineno=10,
        msg="Test message",
        args=None,
        exc_info=None,
    )

    # Without TraceID
    clear_trace_id()
    output = json.loads(formatter.format(log_record))
    assert output["message"] == "Test message"
    assert "trace_id" not in output

    # With TraceID
    tid = "Root=1-12345678-abcdef123456789012345678;Sampled=1"
    set_trace_id(tid)
    output = json.loads(formatter.format(log_record))
    assert output["trace_id"] == tid

    clear_trace_id()
