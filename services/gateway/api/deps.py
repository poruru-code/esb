"""
Dependency Injection for Gateway API.

Manage request handler dependencies using FastAPI Depends.
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
from ..services.pool_manager import PoolManager
from ..client import OrchestratorClient
from ..services.container_cache import ContainerHostCache


# ==========================================
# 1. Service Accessors
# ==========================================


def get_http_client(request: Request) -> AsyncClient:
    return request.app.state.http_client


def get_function_registry(request: Request) -> FunctionRegistry:
    return request.app.state.function_registry


def get_route_matcher(request: Request) -> RouteMatcher:
    return request.app.state.route_matcher


def get_lambda_invoker(request: Request) -> LambdaInvoker:
    return request.app.state.lambda_invoker


def get_event_builder(request: Request) -> EventBuilder:
    return request.app.state.event_builder


def get_pool_manager(request: Request) -> PoolManager:
    return request.app.state.pool_manager


def get_orchestrator_client(request: Request) -> OrchestratorClient:
    client = getattr(request.app.state, "orchestrator_client", None)
    if client:
        return client

    cache = getattr(request.app.state, "container_cache", None)
    if cache is None:
        cache = ContainerHostCache()
        request.app.state.container_cache = cache

    orchestrator_client = OrchestratorClient(request.app.state.http_client, cache=cache)
    request.app.state.orchestrator_client = orchestrator_client
    return orchestrator_client


# Service Dependency Type Aliases
FunctionRegistryDep = Annotated[FunctionRegistry, Depends(get_function_registry)]
RouteMatcherDep = Annotated[RouteMatcher, Depends(get_route_matcher)]
LambdaInvokerDep = Annotated[LambdaInvoker, Depends(get_lambda_invoker)]
HttpClientDep = Annotated[AsyncClient, Depends(get_http_client)]
EventBuilderDep = Annotated[EventBuilder, Depends(get_event_builder)]
PoolManagerDep = Annotated[PoolManager, Depends(get_pool_manager)]


# ==========================================
# 2. Logic Dependencies (Verification & Resolution)
# ==========================================


async def verify_authorization(authorization: Optional[str] = Header(None)) -> str:
    """
    Verify a JWT token and return the user ID.

    Args:
        authorization: Authorization header

    Returns:
        User ID

    Raises:
        HTTPException: 401 on authentication failure
    """
    if not authorization:
        raise HTTPException(status_code=401, detail="Unauthorized")

    user_id = verify_token(authorization, config.JWT_SECRET_KEY)
    if not user_id:
        raise HTTPException(status_code=401, detail="Unauthorized")

    return user_id


async def resolve_lambda_target(request: Request, route_matcher: RouteMatcherDep) -> TargetFunction:
    """
    Resolve the Lambda function target from the request path.

    Args:
        request: FastAPI Request object
        route_matcher: RouteMatcher service (DI)

    Returns:
        TargetFunction: target function info

    Raises:
        HTTPException: 404 when no route matches
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
