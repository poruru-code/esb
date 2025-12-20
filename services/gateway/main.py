"""
Lambda Gateway - API Gateway互換サーバー

AWS API GatewayとLambda Authorizerの挙動を再現し、
routing.ymlに基づいてリクエストをLambda RIEコンテナに転送します。
"""

from contextlib import asynccontextmanager
from fastapi import FastAPI, Request, HTTPException, Header, BackgroundTasks
from fastapi.responses import JSONResponse, Response
from typing import Optional
from datetime import datetime, timezone
import httpx
import logging
from .config import config
from .core.security import create_access_token, verify_token
from .core.proxy import build_event, proxy_to_lambda, parse_lambda_response
from .models.schemas import AuthRequest, AuthResponse, AuthenticationResult
from .services.route_matcher import load_routing_config, match_route
from .client import get_lambda_host
from .services.function_registry import load_functions_config

# Logger setup
logger = logging.getLogger("gateway.main")
logger.setLevel(logging.INFO)

# Suppress noisy library logs
logging.getLogger("httpx").setLevel(logging.WARNING)
logging.getLogger("urllib3").setLevel(logging.WARNING)
logging.getLogger("httpcore").setLevel(logging.WARNING)


@asynccontextmanager
async def lifespan(app: FastAPI):
    """アプリケーションのライフサイクル管理"""
    load_routing_config()
    load_functions_config()
    yield


app = FastAPI(
    title="Lambda Gateway", version="2.0.0", lifespan=lifespan, root_path=config.root_path
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
    from .services.lambda_invoker import invoke_function, get_function_config_or_none
    from .core.exceptions import (
        ContainerStartError,
        LambdaExecutionError,
    )

    # 関数存在チェック（404判定用）
    if get_function_config_or_none(function_name) is None:
        return JSONResponse(
            status_code=404,
            content={"message": f"Function not found: {function_name}"},
        )

    invocation_type = request.headers.get("X-Amz-Invocation-Type", "RequestResponse")
    body = await request.body()

    try:
        if invocation_type == "Event":
            # 非同期呼び出し：バックグラウンドで実行、即座に202を返す
            background_tasks.add_task(invoke_function, function_name, body)
            return Response(status_code=202, content=b"", media_type="application/json")
        else:
            # 同期呼び出し：結果を待って返す
            resp = await invoke_function(function_name, body)
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
async def gateway_handler(request: Request, path: str):
    """キャッチオールルート：routing.ymlに基づいてLambda RIEに転送"""
    request_path = f"/{path}"

    # ルーティングマッチング
    target_container, path_params, route_path, function_config = match_route(
        request_path, request.method
    )

    if not target_container:
        return JSONResponse(status_code=404, content={"message": "Not Found"})

    # 認証検証
    authorization = request.headers.get("authorization")
    if not authorization:
        return JSONResponse(status_code=401, content={"message": "Unauthorized"})

    user_id = verify_token(authorization, config.JWT_SECRET_KEY)
    if not user_id:
        return JSONResponse(status_code=401, content={"message": "Unauthorized"})

    # オンデマンドコンテナ起動
    try:
        container_host = await get_lambda_host(
            function_name=target_container,
            image=function_config.get("image"),
            env=function_config.get("environment", {}),
        )
    except Exception as e:
        logger.error(f"Failed to ensure container {target_container}: {e}", exc_info=True)
        return JSONResponse(
            status_code=503,
            content={"message": "Service Unavailable", "detail": "Cold start failed"},
        )

    # Lambda RIEに転送
    try:
        body = await request.body()
        event = build_event(request, body, user_id, path_params, route_path)
        lambda_response = await proxy_to_lambda(container_host, event)

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
