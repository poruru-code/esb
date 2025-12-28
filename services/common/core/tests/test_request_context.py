import pytest
import uuid
from services.common.core import request_context


def test_generate_request_id_creates_uuid():
    """Ensure generate_request_id() creates a UUIDv4 and sets it in context."""
    # Act.
    req_id = request_context.generate_request_id()

    # Assert.
    assert req_id is not None
    assert isinstance(req_id, str)
    # Verify UUID format.
    try:
        uuid_obj = uuid.UUID(req_id)
        assert str(uuid_obj) == req_id
    except ValueError:
        pytest.fail(f"Generated ID is not a valid UUID: {req_id}")

    # Verify it is set in context.
    assert request_context.get_request_id() == req_id


def test_generate_request_id_is_unique():
    """Ensure a different ID is generated each call."""
    id1 = request_context.generate_request_id()
    id2 = request_context.generate_request_id()

    assert id1 != id2


def test_trace_id_does_not_affect_request_id():
    """Ensure setting Trace ID does not affect Request ID (isolation)."""
    # Clear context.
    request_context.clear_trace_id()

    # Arrange
    trace_val = "Root=1-67890abc-def1234567890abcdef12345"

    # Act.
    # 1. Set Trace ID.
    request_context.set_trace_id(trace_val)

    # Assert.
    # Request ID should remain None until explicitly generated.
    # Assume new context here.
    # TraceId.parse() appends ;Sampled=1 by default, so include it in expectations.
    expected_trace = trace_val + ";Sampled=1"
    assert request_context.get_trace_id() == expected_trace
    # Older implementations set Root ID here, but now it should be independent.

    # Note: if side effects remain, a value derived from Trace ID might appear.
    # This test ensures separation.
    assert request_context.get_request_id() is None
