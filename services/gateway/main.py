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
import json
from .config import config
from .core.security import create_access_token
from .core.utils import parse_lambda_response
from .models import AuthRequest, AuthResponse, AuthenticationResult
from .client import ManagerClient
from .services.container_manager import HttpContainerManager
from .core.event_builder import V1ProxyEventBuilder

# Services Imports
from .services.function_registry import FunctionRegistry
from .services.route_matcher import RouteMatcher
from .services.lambda_invoker import LambdaInvoker

from .api.deps import (
    UserIdDep,
    LambdaTargetDep,
    LambdaInvokerDep,
    FunctionRegistryDep,
    ManagerClientDep,
    EventBuilderDep,
)
from .core.logging_config import setup_logging
from services.common.core.http_client import HttpClientFactory
from .core.exceptions import (
    global_exception_handler,
    http_exception_handler,
    validation_exception_handler,
    ContainerStartError,
    LambdaExecutionError,
    FunctionNotFoundError,
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
    factory = HttpClientFactory(config)
    factory.configure_global_settings()
    client = factory.create_async_client(timeout=config.LAMBDA_INVOKE_TIMEOUT)

    # Initialize Services
    function_registry = FunctionRegistry()
    route_matcher = RouteMatcher(function_registry)

    # Load initial configs
    function_registry.load_functions_config()
    route_matcher.load_routing_config()

    container_manager = HttpContainerManager(config, client)

    lambda_invoker = LambdaInvoker(
        client=client,
        registry=function_registry,
        container_manager=container_manager,
        config=config,
    )
    manager_client = ManagerClient(client)

    # Store in app.state for DI
    app.state.http_client = client
    app.state.function_registry = function_registry
    app.state.route_matcher = route_matcher
    app.state.lambda_invoker = lambda_invoker
    app.state.manager_client = manager_client
    app.state.container_manager = container_manager
    app.state.event_builder = V1ProxyEventBuilder()

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
async def trace_propagation_middleware(request: Request, call_next):
    """
    Middleware for Trace ID propagation and structured access logging.
    """
    import time
    from services.common.core.trace import TraceId
    from services.common.core.request_context import (
        set_trace_id,
        clear_trace_id,
        generate_request_id,
    )

    start_time = time.perf_counter()

    # Trace ID の取得または生成
    trace_id_str = request.headers.get("X-Amzn-Trace-Id")

    if trace_id_str:
        try:
            set_trace_id(trace_id_str)
        except Exception as e:
            logger.warning(
                f"Failed to parse incoming X-Amzn-Trace-Id: '{trace_id_str}', error: {e}"
            )
            # 形式が不正な場合は強制的に再生成
            trace = TraceId.generate()
            trace_id_str = str(trace)
            set_trace_id(trace_id_str)
    else:
        # 存在しない場合は新規生成
        trace = TraceId.generate()
        trace_id_str = str(trace)
        set_trace_id(trace_id_str)

    # Request ID の生成 (Trace IDとは独立)
    req_id = generate_request_id()

    # レスポンス待機
    try:
        response = await call_next(request)

        # レスポンスヘッダーへの付与
        response.headers["X-Amzn-Trace-Id"] = trace_id_str
        response.headers["x-amzn-RequestId"] = req_id

        # Calculate process time
        process_time = time.perf_counter() - start_time
        process_time_ms = round(process_time * 1000, 2)

        # Structured Access Log
        logger.info(
            f"{request.method} {request.url.path} {response.status_code}",
            extra={
                "trace_id": trace_id_str,
                "aws_request_id": req_id,
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
        clear_trace_id()


# 例外ハンドラの登録
app.add_exception_handler(Exception, global_exception_handler)
app.add_exception_handler(StarletteHTTPException, http_exception_handler)
app.add_exception_handler(RequestValidationError, validation_exception_handler)


@app.exception_handler(FunctionNotFoundError)
async def function_not_found_handler(request: Request, exc: FunctionNotFoundError):
    return JSONResponse(
        status_code=404,
        content={"message": str(exc)},
    )


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
    invoker: LambdaInvokerDep,
    registry: FunctionRegistryDep,
):
    """
    AWS Lambda Invoke API 互換エンドポイント
    boto3.client('lambda').invoke() からのリクエストを処理

    InvocationType:
      - RequestResponse（デフォルト）: 同期呼び出し、結果を返す
      - Event: 非同期呼び出し、即座に202を返す
    """
    # Retrieve dependencies (Now injected via DI)

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
            # RIEのレスポンスをそのままクライアント(boto3)へ中継
            return Response(
                content=resp.content,
                status_code=resp.status_code,
                headers=dict(resp.headers),
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
    manager_client: ManagerClientDep,
    event_builder: EventBuilderDep,
    invoker: LambdaInvokerDep,
):
    """
    キャッチオールルート：routing.ymlに基づいてLambda RIEに転送

    認証とルーティング解決は DI で自動的に行われる。
    """
    # Build Event and Invoke Lambda
    try:
        body = await request.body()
        event = await event_builder.build(
            request=request,
            body=body,
            user_id=user_id,
            path_params=target.path_params,
            route_path=target.route_path,
        )

        # Invoke Lambda via LambdaInvoker (handles container ensure & RIE req)
        payload = json.dumps(event).encode("utf-8")
        lambda_response = await invoker.invoke_function(target.container_name, payload)

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

    except httpx.RequestError as e:
        # Lambda 接続失敗時はキャッシュを無効化
        # 次回リクエストで Manager に再問い合わせし、コンテナを再起動
        logger.error(
            f"Lambda connection failed for {target.container_name}",
            extra={
                "container_name": target.container_name,
                "port": config.LAMBDA_PORT,
                "timeout": config.LAMBDA_INVOKE_TIMEOUT,
                "error_type": type(e).__name__,
                "error_detail": str(e),
            },
            exc_info=True,
        )
        # LambdaInvoker might have already logged, but we keep this for gateway context
        manager_client.invalidate_cache(target.container_name)
        return JSONResponse(status_code=502, content={"message": "Bad Gateway"})
    except ContainerStartError as e:
        return JSONResponse(status_code=503, content={"message": str(e)})
    except LambdaExecutionError as e:
        return JSONResponse(status_code=502, content={"message": str(e)})


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)
