"""
カスタム例外クラス

Lambda呼び出しに関するエラーを表現します。
"""

import logging
from fastapi import Request, status
from fastapi.responses import JSONResponse
from fastapi.exceptions import RequestValidationError
from starlette.exceptions import HTTPException as StarletteHTTPException

logger = logging.getLogger(__name__)


class LambdaInvokeError(Exception):
    """Lambda呼び出しの基底例外クラス"""

    pass


class FunctionNotFoundError(LambdaInvokeError):
    """関数が見つからない場合の例外"""

    def __init__(self, function_name: str):
        self.function_name = function_name
        super().__init__(f"Function not found: {function_name}")


class ContainerStartError(LambdaInvokeError):
    """コンテナ起動に失敗した場合の例外"""

    def __init__(self, function_name: str, cause: Exception):
        self.function_name = function_name
        self.cause = cause
        super().__init__(f"Failed to start container {function_name}: {cause}")


class LambdaExecutionError(LambdaInvokeError):
    """Lambda実行に失敗した場合の例外"""

    def __init__(self, function_name: str, cause: Exception):
        self.function_name = function_name
        self.cause = cause

        super().__init__(f"Lambda execution failed for {function_name}: {cause}")


# ===========================================
# Exception Handlers
# ===========================================


async def global_exception_handler(request: Request, exc: Exception):
    """
    未処理の例外を一元的にキャッチするハンドラ
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
    HTTPException のハンドラ
    """
    return JSONResponse(status_code=exc.status_code, content={"message": exc.detail})


async def validation_exception_handler(request: Request, exc: RequestValidationError):
    """
    バリデーションエラーのハンドラ
    """
    return JSONResponse(
        status_code=status.HTTP_422_UNPROCESSABLE_ENTITY,
        content={"message": "Validation Error", "detail": str(exc.errors())},
    )
