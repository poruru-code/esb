"""
プロキシロジックモジュール

API Gateway Lambda Proxy Integration互換のイベント構築と
プロキシ機能: Lambda RIEへのリクエスト転送とレスポンス変換
"""

import json
import logging
from typing import Dict, Any

import httpx

from ..config import config

logger = logging.getLogger("gateway.proxy")


def resolve_container_ip(container_name: str) -> str:
    """
    コンテナ名からIPアドレスを解決

    Gatewayが内部ネットワーク(LAMBDA_NETWORK)に参加しているため、
    DockerのDNS機能によりコンテナ名で直接アクセス可能。
    そのため、基本的にはコンテナ名をそのまま返す。

    Args:
        container_name: Dockerコンテナ名

    Returns:
        アクセス可能なホスト名またはIPアドレス
    """
    # 既にIPアドレス形式の場合はそのまま返す
    if container_name.replace(".", "").isdigit():
        return container_name

    # 同一ネットワーク内なのでコンテナ名で名前解決可能
    return container_name


async def proxy_to_lambda(
    target_container: str, event: dict, client: httpx.AsyncClient
) -> httpx.Response:
    """
    Lambda RIEコンテナにリクエストを転送
    """
    # コンテナ名からIPを解決
    host = resolve_container_ip(target_container)

    rie_url = f"http://{host}:{config.LAMBDA_PORT}/2015-03-31/functions/function/invocations"

    headers = {"Content-Type": "application/json"}

    response = await client.post(
        rie_url, json=event, headers=headers, timeout=config.LAMBDA_INVOKE_TIMEOUT
    )

    return response


def parse_lambda_response(lambda_response: httpx.Response) -> Dict[str, Any]:
    """
    Lambda RIEからのレスポンスをパースしてFastAPI用のレスポンスデータに変換

    Args:
        lambda_response: Lambda RIEからの生レスポンス

    Returns:
        FastAPIレスポンス用の辞書:
        {
            "status_code": int,
            "content": Any,
            "headers": dict,
            "raw_content": bytes (JSONパース失敗時のみ)
        }
    """
    try:
        response_data = lambda_response.json()

        # Lambda応答がAPI Gateway形式の場合
        if isinstance(response_data, dict) and "statusCode" in response_data:
            status_code = response_data.get("statusCode", 200)
            response_headers = response_data.get("headers", {})
            response_body = response_data.get("body", "")

            # bodyがJSON文字列の場合はパース
            if isinstance(response_body, str):
                try:
                    response_body = json.loads(response_body)
                except json.JSONDecodeError:
                    logger.warning(
                        "Failed to parse Lambda response body as JSON. Returning as string.",
                        extra={
                            "snippet": response_body[:200] if response_body else "",
                            "status_code": status_code,
                        },
                    )

            return {
                "status_code": status_code,
                "content": response_body,
                "headers": response_headers,
            }
        else:
            return {"status_code": 200, "content": response_data, "headers": {}}

    except json.JSONDecodeError:
        return {
            "status_code": lambda_response.status_code,
            "raw_content": lambda_response.content,
            "headers": dict(lambda_response.headers),
        }
