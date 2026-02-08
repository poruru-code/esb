"""
Where: services/gateway/middleware.py
What: Gateway HTTP middleware for trace propagation and access logging.
Why: Isolate cross-cutting request concerns from app assembly.
"""

import logging
import time

from fastapi import Request

from services.common.core.request_context import clear_trace_id, generate_request_id, set_trace_id
from services.common.core.trace import TraceId

logger = logging.getLogger("gateway.main")


async def trace_propagation_middleware(request: Request, call_next):
    """Middleware for Trace ID propagation and structured access logging."""
    start_time = time.perf_counter()

    trace_id_str = request.headers.get("X-Amzn-Trace-Id")
    if trace_id_str:
        try:
            set_trace_id(trace_id_str)
        except Exception as exc:
            logger.warning(
                "Failed to parse incoming X-Amzn-Trace-Id: '%s', error: %s",
                trace_id_str,
                exc,
            )
            trace = TraceId.generate()
            trace_id_str = str(trace)
            set_trace_id(trace_id_str)
    else:
        trace = TraceId.generate()
        trace_id_str = str(trace)
        set_trace_id(trace_id_str)

    req_id = generate_request_id()

    try:
        response = await call_next(request)
        response.headers["X-Amzn-Trace-Id"] = trace_id_str
        response.headers["x-amzn-RequestId"] = req_id

        process_time = time.perf_counter() - start_time
        process_time_ms = round(process_time * 1000, 2)

        logger.info(
            f"{request.method} {request.url.path} {response.status_code}",
            extra={
                "trace_id": trace_id_str,
                "aws_request_id": req_id,
                "method": request.method,
                "path": request.url.path,
                "query_params": str(request.query_params),
                "status": response.status_code,
                "latency_ms": process_time_ms,
                "user_agent": request.headers.get("user-agent"),
                "client_ip": request.client.host if request.client else None,
            },
        )

        return response
    finally:
        clear_trace_id()
