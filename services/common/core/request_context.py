"""
RequestContext management.
Use ContextVar to share TraceId across async execution.
"""

from contextvars import ContextVar
from typing import Optional
from .trace import TraceId


# Context variable for Trace ID (full header format).
_trace_id_var: ContextVar[Optional[str]] = ContextVar("trace_id", default=None)
# Context variable for Request ID (UUID).
_request_id_var: ContextVar[Optional[str]] = ContextVar("request_id", default=None)


def get_trace_id() -> Optional[str]:
    """Get the current Trace ID."""
    return _trace_id_var.get()


def get_request_id() -> Optional[str]:
    """Get the current Request ID."""
    return _request_id_var.get()


def generate_request_id() -> str:
    """
    Generate and set a new Request ID (UUID) for the current context.
    """
    import uuid

    new_id = str(uuid.uuid4())
    _request_id_var.set(new_id)
    return new_id


def set_trace_id(trace_id_str: str) -> str:
    """
    Set the Trace ID.

    Args:
        trace_id_str: X-Amzn-Trace-Id header string

    Returns:
        The full Trace ID string that was set
    """
    try:
        trace = TraceId.parse(trace_id_str)
        _trace_id_var.set(str(trace))
        return str(trace)
    except Exception as e:
        raise e


def clear_trace_id() -> None:
    """Clear the Trace ID context."""
    _trace_id_var.set(None)
    _request_id_var.set(None)
