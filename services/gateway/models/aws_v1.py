# services/gateway/models/aws_v1.py

"""
Pydantic models for AWS API Gateway v1 (REST API) event structure.

Reference: https://docs.aws.amazon.com/apigateway/latest/developerguide/set-up-lambda-proxy-integrations.html#api-gateway-simple-proxy-for-lambda-input-format

This module provides Pydantic models to build API Gateway Lambda Proxy Integration
event structures in a type-safe manner.
"""

from typing import Dict, Any, Optional, List
from pydantic import BaseModel, Field, ConfigDict


class ApiGatewayIdentity(BaseModel):
    """API Gateway Identity object."""

    sourceIp: str
    userAgent: Optional[str] = None


class ApiGatewayAuthorizer(BaseModel):
    """API Gateway Authorizer object."""

    claims: Dict[str, Any] = Field(default_factory=dict)
    # Sometimes placed at top level for compatibility.
    cognito_username: Optional[str] = Field(None, alias="cognito:username")

    model_config = ConfigDict(populate_by_name=True)


class ApiGatewayRequestContext(BaseModel):
    """API Gateway Request Context object."""

    identity: ApiGatewayIdentity
    authorizer: Optional[ApiGatewayAuthorizer] = None
    requestId: str
    stage: str = "prod"
    path: Optional[str] = None
    protocol: str = "HTTP/1.1"


class APIGatewayProxyEvent(BaseModel):
    """
    AWS API Gateway Proxy Integration (v1) Event Structure

    Defines the structure of the event object received by Lambda functions.
    Use model_dump(exclude_none=True) to convert to a dict.
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
