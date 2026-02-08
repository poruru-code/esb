"""
Where: services/gateway/exceptions.py
What: Gateway exception handler registration and custom HTTP mappings.
Why: Keep error handling setup isolated from route and lifecycle concerns.
"""

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse
from starlette.exceptions import HTTPException as StarletteHTTPException

from .core.exceptions import (
    FunctionNotFoundError,
    ResourceExhaustedError,
    global_exception_handler,
    http_exception_handler,
    validation_exception_handler,
)


async def function_not_found_handler(request: Request, exc: FunctionNotFoundError):
    return JSONResponse(
        status_code=404,
        content={"message": str(exc)},
    )


async def resource_exhausted_handler(request: Request, exc: ResourceExhaustedError):
    return JSONResponse(
        status_code=429,
        content={"message": "Too Many Requests", "detail": str(exc)},
    )


def register_exception_handlers(app: FastAPI) -> None:
    app.add_exception_handler(Exception, global_exception_handler)  # ty: ignore[invalid-argument-type]  # Starlette type stubs incomplete
    app.add_exception_handler(StarletteHTTPException, http_exception_handler)  # ty: ignore[invalid-argument-type]  # Starlette type stubs incomplete
    app.add_exception_handler(RequestValidationError, validation_exception_handler)  # ty: ignore[invalid-argument-type]  # Starlette type stubs incomplete
    app.add_exception_handler(FunctionNotFoundError, function_not_found_handler)  # ty: ignore[invalid-argument-type]  # Starlette type stubs incomplete
    app.add_exception_handler(ResourceExhaustedError, resource_exhausted_handler)  # ty: ignore[invalid-argument-type]  # Starlette type stubs incomplete
