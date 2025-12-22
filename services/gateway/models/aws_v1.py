# services/gateway/models/aws_v1.py

"""
AWS API Gateway v1 (REST API) イベント構造の Pydantic モデル定義

参照: https://docs.aws.amazon.com/apigateway/latest/developerguide/set-up-lambda-proxy-integrations.html#api-gateway-simple-proxy-for-lambda-input-format

このモジュールは、API Gateway Lambda Proxy Integration のイベント構造を
型安全に構築するための Pydantic モデルを提供します。
"""

from typing import Dict, Any, Optional, List
from pydantic import BaseModel, Field, ConfigDict


class ApiGatewayIdentity(BaseModel):
    """API Gateway Identity オブジェクト"""

    sourceIp: str
    userAgent: Optional[str] = None


class ApiGatewayAuthorizer(BaseModel):
    """API Gateway Authorizer オブジェクト"""

    claims: Dict[str, Any] = Field(default_factory=dict)
    # 互換性のためにトップレベルにも配置することがある
    cognito_username: Optional[str] = Field(None, alias="cognito:username")

    model_config = ConfigDict(populate_by_name=True)


class ApiGatewayRequestContext(BaseModel):
    """API Gateway Request Context オブジェクト"""

    identity: ApiGatewayIdentity
    authorizer: Optional[ApiGatewayAuthorizer] = None
    requestId: str
    stage: str = "prod"
    path: Optional[str] = None
    protocol: str = "HTTP/1.1"


class APIGatewayProxyEvent(BaseModel):
    """
    AWS API Gateway Proxy Integration (v1) Event Structure

    Lambda 関数が受け取るイベントオブジェクトの構造を定義。
    model_dump(exclude_none=True) で辞書に変換して使用する。
    """

    resource: str
    path: str
    httpMethod: str
    headers: Dict[str, str]
    multiValueHeaders: Dict[str, List[str]]
    queryStringParameters: Optional[Dict[str, str]] = None
    multiValueQueryStringParameters: Optional[Dict[str, List[str]]] = None
    pathParameters: Optional[Dict[str, str]] = None
    stageVariables: Optional[Dict[str, str]] = None
    requestContext: ApiGatewayRequestContext
    body: Optional[str] = None
    isBase64Encoded: bool = False
