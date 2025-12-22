"""
Dependency Injection for Gateway API

FastAPI の Depends を使用してリクエストハンドラの依存性を管理します。
"""

from typing import Annotated, Optional
from fastapi import Depends, Header, HTTPException, Request
from ..config import config
from ..core.security import verify_token

from ..models import TargetFunction


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


async def resolve_lambda_target(request: Request) -> TargetFunction:
    """
    リクエストパスから Lambda 関数ターゲット情報を解決する。

    Args:
        request: FastAPI Request オブジェクト

    Returns:
        TargetFunction: ターゲット関数の情報

    Raises:
        HTTPException: ルーティングマッチしない場合に 404
    """
    path = request.url.path
    method = request.method

    target_container, path_params, route_path, function_config = (
        request.app.state.route_matcher.match_route(path, method)
    )

    if not target_container:
        raise HTTPException(status_code=404, detail="Not Found")

    return TargetFunction(
        container_name=target_container,
        path_params=path_params,
        route_path=route_path,
        function_config=function_config,
    )


# 型エイリアス定義（Annotated を使用した DI）
UserIdDep = Annotated[str, Depends(verify_authorization)]
LambdaTargetDep = Annotated[TargetFunction, Depends(resolve_lambda_target)]
