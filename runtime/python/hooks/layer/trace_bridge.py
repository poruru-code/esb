"""
Trace ID bridging decorator for the RIE environment.

In AWS Lambda production, the _X_AMZN_TRACE_ID environment variable is set automatically,
but RIE (Runtime Interface Emulator) lacks this, so we pass the Trace ID via ClientContext.
"""

import asyncio
import logging
import os
from functools import wraps

logger = logging.getLogger(__name__)


def _set_trace_id_from_context(context):
    """Extract Trace ID from ClientContext and set it in the environment."""
    if hasattr(context, "client_context") and context.client_context:
        custom = getattr(context.client_context, "custom", None)
        if custom and isinstance(custom, dict) and "trace_id" in custom:
            trace_id = custom["trace_id"]
            if not os.environ.get("_X_AMZN_TRACE_ID"):
                os.environ["_X_AMZN_TRACE_ID"] = trace_id
                logger.debug(f"Hydrated _X_AMZN_TRACE_ID from ClientContext: {trace_id}")


def hydrate_trace_id(handler):
    """
    Decorator for RIE that extracts a Trace ID from ClientContext and sets
    the _X_AMZN_TRACE_ID environment variable.

    Supports both sync and async handlers.

    Usage:
        @hydrate_trace_id
        def lambda_handler(event, context):
            ...

        @hydrate_trace_id
        async def lambda_handler(event, context):
            ...
    """
    if asyncio.iscoroutinefunction(handler):

        @wraps(handler)
        async def async_wrapper(event, context):
            _set_trace_id_from_context(context)
            return await handler(event, context)

        return async_wrapper
    else:

        @wraps(handler)
        def sync_wrapper(event, context):
            _set_trace_id_from_context(context)
            return handler(event, context)

        return sync_wrapper
