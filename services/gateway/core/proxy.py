"""
プロキシロジックモジュール

API Gateway Lambda Proxy Integration互換のイベント構築と
Lambda RIEへのリクエスト転送を行います。
"""

import base64
import json
import time
from typing import Dict, Any

from fastapi import Request
import httpx


def build_event(
    request: Request, body: bytes, user_id: str, path_params: Dict[str, str], route_path: str
) -> Dict[str, Any]:
    """
    API Gateway Lambda Proxy Integration互換のeventオブジェクトを構築

    Args:
        request: FastAPIリクエストオブジェクト
        body: リクエストボディ（バイト列）
        user_id: 認証済みユーザーID
        path_params: パスパラメータ
        route_path: マッチしたルートパターン

    Returns:
        Lambda関数に渡すeventオブジェクト
    """
    # gzip圧縮されているか確認
    is_base64 = "gzip" in request.headers.get("content-encoding", "").lower()

    # ボディの処理
    if is_base64:
        body_content = base64.b64encode(body).decode("utf-8")
    else:
        try:
            body_content = body.decode("utf-8")
        except UnicodeDecodeError:
            body_content = base64.b64encode(body).decode("utf-8")
            is_base64 = True

    # クエリパラメータ
    query_params = {}
    multi_query_params = {}
    if request.query_params:
        for key in request.query_params.keys():
            values = request.query_params.getlist(key)
            query_params[key] = values[-1] if values else ""
            multi_query_params[key] = values

    # ヘッダー
    headers = {}
    multi_headers = {}
    # FastAPI/Starletteのheaders.items()は正規化されており、
    # 同一名のヘッダーは通常カンマ区切りで取得されるか、個別に取得する必要がある。
    # ここではgetlist相当の処理を自前で行うか、標準的なitems()から構築
    for key in request.headers.keys():
        # Starlette Headers.getlist(key) は全ての値を取得する
        values = request.headers.getlist(key)
        headers[key] = values[-1] if values else ""
        multi_headers[key] = values

    event = {
        "resource": route_path or str(request.url.path),
        "path": str(request.url.path),
        "httpMethod": request.method,
        "headers": headers,
        "multiValueHeaders": multi_headers,
        "queryStringParameters": query_params if query_params else None,
        "multiValueQueryStringParameters": multi_query_params if multi_query_params else None,
        "pathParameters": path_params if path_params else None,
        "requestContext": {
            "identity": {"sourceIp": request.client.host if request.client else "unknown"},
            "authorizer": {"claims": {"cognito:username": user_id}, "cognito:username": user_id},
            "requestId": f"req-{int(time.time() * 1000)}",
        },
        "body": body_content,
        "isBase64Encoded": is_base64,
    }

    return event


def resolve_container_ip(container_name: str) -> str:
    """
    コンテナ名からIPアドレスを解決

    Gatewayが内部ネットワーク（LAMBDA_NETWORK）に参加しているため、
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


async def proxy_to_lambda(target_container: str, event: dict) -> httpx.Response:
    """
    Lambda RIEコンテナにリクエストを転送
    """
    # コンテナ名からIPを解決
    host = resolve_container_ip(target_container)

    rie_url = f"http://{host}:8080/2015-03-31/functions/function/invocations"

    headers = {"Content-Type": "application/json"}

    async with httpx.AsyncClient() as client:
        response = await client.post(rie_url, json=event, headers=headers, timeout=30.0)

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
                    pass

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
