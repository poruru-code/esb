"""
Lambda Invoker - Lambda関数呼び出しの共通ロジック

main.py (FastAPI) と scheduler.py で再利用可能な純粋な関数を提供します。
非同期化は呼び出し元に委ねます（FastAPI の BackgroundTasks など）。
"""

import logging
from typing import Optional

import requests

from .function_registry import get_function_config
from .container import get_manager
from ..core.exceptions import FunctionNotFoundError, ContainerStartError, LambdaExecutionError

logger = logging.getLogger("gateway.lambda_invoker")


def invoke_function(function_name: str, payload: bytes, timeout: int = 300) -> requests.Response:
    """
    Lambda関数を呼び出す（純粋な関数）

    コンテナ起動 + RIE POST を行い、レスポンスを返します。
    エラー時はカスタム例外を送出します。

    Args:
        function_name: 関数名（コンテナ名）
        payload: リクエストペイロード（バイト列）
        timeout: タイムアウト秒数

    Returns:
        Lambda RIE からのレスポンス

    Raises:
        FunctionNotFoundError: 関数が見つからない場合
        ContainerStartError: コンテナ起動に失敗した場合
        LambdaExecutionError: Lambda実行に失敗した場合
    """
    # 関数設定を取得
    func_config = get_function_config(function_name)
    if func_config is None:
        raise FunctionNotFoundError(function_name)

    # 環境変数を準備
    env = func_config.get("environment", {}).copy()
    # Gatewayの内部URLを動的に解決
    # 解決できない場合は ContainerManager がエラーを吐くので、ここではキャッチしない（起動失敗させる）
    gateway_internal_url = get_manager().resolve_gateway_internal_url()
    env["GATEWAY_INTERNAL_URL"] = gateway_internal_url

    # コンテナを起動
    try:
        host = get_manager().ensure_container_running(
            name=function_name,
            image=func_config.get("image"),
            env=env,
        )
    except Exception as e:
        raise ContainerStartError(function_name, e) from e

    # Lambda RIE に POST
    rie_url = f"http://{host}:8080/2015-03-31/functions/function/invocations"
    logger.info(f"Invoking {function_name} at {rie_url}")

    try:
        response = requests.post(
            rie_url,
            data=payload,
            headers={"Content-Type": "application/json"},
            timeout=timeout,
        )
        return response
    except requests.exceptions.RequestException as e:
        raise LambdaExecutionError(function_name, e) from e


def get_function_config_or_none(function_name: str) -> Optional[dict]:
    """
    関数設定を取得（例外を送出しない版）

    invoke_lambda_api で404判定に使用
    """
    return get_function_config(function_name)
