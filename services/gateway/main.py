"""
Lambda Gateway - API Gateway互換サーバー

AWS API GatewayとLambda Authorizerの挙動を再現し、
routing.ymlに基づいてリクエストをLambda RIEコンテナに転送します。
"""

from contextlib import asynccontextmanager
from fastapi import FastAPI, Request, HTTPException, Header, BackgroundTasks
from fastapi.responses import JSONResponse, Response
from fastapi.exceptions import RequestValidationError
from starlette.exceptions import HTTPException as StarletteHTTPException
from typing import Optional
from datetime import datetime, timezone
import httpx
import logging
from .config import config
from .core.security import create_access_token
from .core.proxy import build_event, proxy_to_lambda, parse_lambda_response
from .models.schemas import AuthRequest, AuthResponse, AuthenticationResult
from .client import ManagerClient

# Services Imports
from .services.function_registry import FunctionRegistry
from .services.route_matcher import RouteMatcher
from .services.lambda_invoker import LambdaInvoker

from .api.deps import UserIdDep, LambdaTargetDep
from .core.logging_config import setup_logging
from services.common.core.request_context import set_request_id, clear_request_id
from .core.exceptions import (
    global_exception_handler,
    http_exception_handler,
    validation_exception_handler,
    ContainerStartError,
    LambdaExecutionError,
)

# Logger setup
setup_logging()
logger = logging.getLogger("gateway.main")


# ===========================================
# Middleware
# ===========================================


@asynccontextmanager
async def lifespan(app: FastAPI):
    """アプリケーションのライフサイクル管理"""
    # Initialize shared HTTP client
    # timeout config can be fine-tuned
    client = httpx.AsyncClient(timeout=30.0)

    # Initialize Services
    function_registry = FunctionRegistry()
    route_matcher = RouteMatcher(function_registry)

    # Load initial configs
    function_registry.load_functions_config()
    route_matcher.load_routing_config()

    lambda_invoker = LambdaInvoker(client, function_registry)
    manager_client = ManagerClient(client)

    # Store in app.state for DI
    app.state.http_client = client
    app.state.function_registry = function_registry
    app.state.route_matcher = route_matcher
    app.state.lambda_invoker = lambda_invoker
    app.state.manager_client = manager_client

    logger.info("Gateway initialized with shared resources.")

    yield

    # Cleanup
    logger.info("Gateway shutting down, clicking http client.")
    await client.aclose()


app = FastAPI(
    title="Lambda Gateway", version="2.0.0", lifespan=lifespan, root_path=config.root_path
)


# ミドルウェアの登録（デコレーター方式）
@app.middleware("http")
async def request_id_middleware(request: Request, call_next):
    """
    Middleware for Request ID tracing and structured access logging.
    """
    import time

    start_time = time.time()

    # X-Request-Id ヘッダーから取得、なければ生成
    request_id = request.headers.get("X-Request-Id")
    request_id = set_request_id(request_id)  # set_request_id は設定した ID を返す

    # レスポンスヘッダーにも付与するためにレスポンスを待機
    try:
        response = await call_next(request)
        response.headers["X-Request-Id"] = request_id

        # Calculate process time
        process_time = time.time() - start_time
        process_time_ms = round(process_time * 1000, 2)

        # Structured Access Log
        # uvicorn.access is disabled (WARNING level), so this is the main access log.
        logger.info(
            f"{request.method} {request.url.path} {response.status_code}",
            extra={
                "method": request.method,
                "path": request.url.path,
                "status": response.status_code,
                "latency_ms": process_time_ms,
                "user_agent": request.headers.get("user-agent"),
                "client_ip": request.client.host if request.client else None,
            },
        )

        return response
    finally:
        # クリーンアップ
        clear_request_id()


# 例外ハンドラの登録
app.add_exception_handler(Exception, global_exception_handler)
app.add_exception_handler(StarletteHTTPException, http_exception_handler)
app.add_exception_handler(RequestValidationError, validation_exception_handler)


# ===========================================
# エンドポイント定義
# ===========================================


@app.post(config.AUTH_ENDPOINT_PATH, response_model=AuthResponse)
async def authenticate_user(
    request: AuthRequest, response: Response, x_api_key: Optional[str] = Header(None)
):
    """ユーザー認証エンドポイント"""
    if not x_api_key or x_api_key != config.X_API_KEY:
        logger.warning("Auth failed. Invalid API Key received.")
        raise HTTPException(status_code=401, detail="Unauthorized")

    response.headers["PADMA_USER_AUTHORIZED"] = "true"

    username = request.AuthParameters.USERNAME
    password = request.AuthParameters.PASSWORD

    if username == config.AUTH_USER and password == config.AUTH_PASS:
        id_token = create_access_token(
            username=username,
            secret_key=config.JWT_SECRET_KEY,
            expires_delta=config.JWT_EXPIRES_DELTA,
        )
        return AuthResponse(AuthenticationResult=AuthenticationResult(IdToken=id_token))

    return JSONResponse(
        status_code=401,
        content={"message": "Unauthorized"},
        headers={"PADMA_USER_AUTHORIZED": "true"},
    )


@app.get("/health")
async def health_check():
    """ヘルスチェックエンドポイント"""
    return {"status": "healthy", "timestamp": datetime.now(timezone.utc).isoformat()}


# ===========================================
# AWS Lambda Service Compatible Endpoint
# ===========================================


@app.post("/2015-03-31/functions/{function_name}/invocations")
async def invoke_lambda_api(
    function_name: str,
    request: Request,
    background_tasks: BackgroundTasks,
):
    """
    AWS Lambda Invoke API 互換エンドポイント
    boto3.client('lambda').invoke() からのリクエストを処理

    InvocationType:
      - RequestResponse（デフォルト）: 同期呼び出し、結果を返す
      - Event: 非同期呼び出し、即座に202を返す
    """
    # Retrieve dependencies
    invoker: LambdaInvoker = request.app.state.lambda_invoker
    registry: FunctionRegistry = request.app.state.function_registry

    # 関数存在チェック（404判定用）
    if registry.get_function_config(function_name) is None:
        return JSONResponse(
            status_code=404,
            content={"message": f"Function not found: {function_name}"},
        )

    invocation_type = request.headers.get("X-Amz-Invocation-Type", "RequestResponse")
    body = await request.body()

    try:
        if invocation_type == "Event":
            # 非同期呼び出し：バックグラウンドで実行、即座に202を返す
            background_tasks.add_task(invoker.invoke_function, function_name, body)
            return Response(status_code=202, content=b"", media_type="application/json")
        else:
            # 同期呼び出し：結果を待って返す
            resp = await invoker.invoke_function(function_name, body)
            return Response(
                content=resp.content,
                status_code=resp.status_code,
                media_type="application/json",
            )
    except ContainerStartError as e:
        return JSONResponse(status_code=503, content={"message": str(e)})
    except LambdaExecutionError as e:
        return JSONResponse(status_code=502, content={"message": str(e)})


@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "DELETE"])
async def gateway_handler(
    request: Request,
    path: str,
    user_id: UserIdDep,
    target: LambdaTargetDep,
):
    """
    キャッチオールルート：routing.ymlに基づいてLambda RIEに転送

    認証とルーティング解決は DI で自動的に行われる。
    """
    # オンデマンドコンテナ起動
    try:
        container_host = await request.app.state.manager_client.ensure_container(
            function_name=target.container_name,
            image=target.function_config.get("image"),
            env=target.function_config.get("environment", {}),
        )
    except Exception as e:
        logger.error(f"Failed to ensure container {target.container_name}: {e}", exc_info=True)
        return JSONResponse(
            status_code=503,
            content={"message": "Service Unavailable", "detail": "Cold start failed"},
        )

    # Lambda RIEに転送
    try:
        body = await request.body()
        event = build_event(request, body, user_id, target.path_params, target.route_path)

        # Inject shared client
        lambda_response = await proxy_to_lambda(
            container_host, event, client=request.app.state.http_client
        )

        # レスポンス変換
        result = parse_lambda_response(lambda_response)
        if "raw_content" in result:
            return Response(
                content=result["raw_content"],
                status_code=result["status_code"],
                headers=result["headers"],
            )
        return JSONResponse(
            status_code=result["status_code"], content=result["content"], headers=result["headers"]
        )

    except httpx.RequestError:
        return JSONResponse(status_code=502, content={"message": "Bad Gateway"})


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)
