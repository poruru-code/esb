"""
Dependency Injection for Gateway API

FastAPI の Depends を使用してリクエストハンドラの依存性を管理します。
"""

from typing import Annotated, Optional
from fastapi import Depends, Header, HTTPException, Request
from httpx import AsyncClient

from ..config import config
from ..core.security import verify_token
from ..models import TargetFunction
from ..core.event_builder import EventBuilder


from ..services.function_registry import FunctionRegistry
from ..services.route_matcher import RouteMatcher
from ..services.lambda_invoker import LambdaInvoker
from ..client import ManagerClient


# ==========================================
# 1. Service Accessors
# ==========================================


def get_http_client(request: Request) -> AsyncClient:
    return request.app.state.http_client


def get_function_registry(request: Request) -> FunctionRegistry:
    return request.app.state.function_registry


def get_route_matcher(request: Request) -> RouteMatcher:
    return request.app.state.route_matcher


def get_manager_client(request: Request) -> ManagerClient:
    return request.app.state.manager_client


def get_lambda_invoker(request: Request) -> LambdaInvoker:
    return request.app.state.lambda_invoker


def get_event_builder(request: Request) -> EventBuilder:
    return request.app.state.event_builder


# Service Dependency Type Aliases
FunctionRegistryDep = Annotated[FunctionRegistry, Depends(get_function_registry)]
RouteMatcherDep = Annotated[RouteMatcher, Depends(get_route_matcher)]
ManagerClientDep = Annotated[ManagerClient, Depends(get_manager_client)]
LambdaInvokerDep = Annotated[LambdaInvoker, Depends(get_lambda_invoker)]
HttpClientDep = Annotated[AsyncClient, Depends(get_http_client)]
EventBuilderDep = Annotated[EventBuilder, Depends(get_event_builder)]


# ==========================================
# 2. Logic Dependencies (Verification & Resolution)
# ==========================================


async def verify_authorization(authorization: Optional[str] = Header(None)) -> str:
    """
    JWT トークンを検証してユーザーIDを返す。

    Args:
        authorization: Authorization ヘッダー

    Returns:
        ユーザーID

    Raises:
        HTTPException: 認証失敗時に 401
    """
    if not authorization:
        raise HTTPException(status_code=401, detail="Unauthorized")

    user_id = verify_token(authorization, config.JWT_SECRET_KEY)
    if not user_id:
        raise HTTPException(status_code=401, detail="Unauthorized")

    return user_id


async def resolve_lambda_target(request: Request, route_matcher: RouteMatcherDep) -> TargetFunction:
    """
    リクエストパスから Lambda 関数ターゲット情報を解決する。

    Args:
        request: FastAPI Request オブジェクト
        route_matcher: RouteMatcher サービス (DI)

    Returns:
        TargetFunction: ターゲット関数の情報

    Raises:
        HTTPException: ルーティングマッチしない場合に 404
    """
    path = request.url.path
    method = request.method

    target_container, path_params, route_path, function_config = route_matcher.match_route(
        path, method
    )

    if not target_container:
        raise HTTPException(status_code=404, detail="Not Found")

    return TargetFunction(
        container_name=target_container,
        path_params=path_params,
        route_path=route_path,
        function_config=function_config,
    )


# Logic Dependency Type Aliases
UserIdDep = Annotated[str, Depends(verify_authorization)]
LambdaTargetDep = Annotated[TargetFunction, Depends(resolve_lambda_target)]
