"""
Custom exception classes.

Represent errors related to Lambda invocation.
"""

import logging
from fastapi import Request, status
from fastapi.responses import JSONResponse
from fastapi.exceptions import RequestValidationError
from starlette.exceptions import HTTPException as StarletteHTTPException

logger = logging.getLogger(__name__)


class LambdaInvokeError(Exception):
    """Base exception class for Lambda invocation."""

    pass


class FunctionNotFoundError(LambdaInvokeError):
    """Raised when a function is not found."""

    def __init__(self, function_name: str):
        self.function_name = function_name
        super().__init__(f"Function not found: {function_name}")


class ContainerStartError(LambdaInvokeError):
    """Raised when container startup fails."""

    def __init__(self, function_name: str, cause: Exception):
        self.function_name = function_name
        self.cause = cause
        super().__init__(f"Failed to start container {function_name}: {cause}")


class LambdaExecutionError(LambdaInvokeError):
    """Raised when Lambda execution fails."""

    def __init__(self, function_name: str, cause: Exception):
        self.function_name = function_name
        self.cause = cause

        super().__init__(f"Lambda execution failed for {function_name}: {cause}")


class OrchestratorError(LambdaInvokeError):
    """Error from the orchestrator service."""

    def __init__(self, status_code: int, detail: str):
        self.status_code = status_code
        self.detail = detail
        super().__init__(f"Orchestrator error ({status_code}): {detail}")


class OrchestratorTimeoutError(OrchestratorError):
    """Timeout error from the orchestrator service."""

    def __init__(self, detail: str = "Orchestrator request timed out"):
        super().__init__(408, detail)


class OrchestratorUnreachableError(LambdaInvokeError):
    """Failed to connect to the orchestrator service."""

    def __init__(self, cause: Exception):
        self.cause = cause
        super().__init__(f"Orchestrator unreachable: {cause}")


class ResourceExhaustedError(LambdaInvokeError):
    """Raised when resources are exhausted (queue full or timeout)."""

    def __init__(self, detail: str = "Request timed out in queue"):
        super().__init__(detail)


# ===========================================
# Exception Handlers
# ===========================================


async def global_exception_handler(request: Request, exc: Exception):
    """
    Catch-all handler for unhandled exceptions.
    """
    error_detail = str(exc)
    logger.error(
        f"Global exception handler caught: {exc}",
        exc_info=True,
        extra={
            "path": request.url.path,
            "method": request.method,
        },
    )

    return JSONResponse(
        status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
        content={"message": "Internal Server Error", "detail": error_detail},
    )


async def http_exception_handler(request: Request, exc: StarletteHTTPException):
    """
    Handler for HTTPException.
    """
    return JSONResponse(status_code=exc.status_code, content={"message": exc.detail})


async def validation_exception_handler(request: Request, exc: RequestValidationError):
    """
    Handler for validation errors.
    """
    return JSONResponse(
        status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
        content={"message": "Validation Error", "detail": str(exc.errors())},
    )
