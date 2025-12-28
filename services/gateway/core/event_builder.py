from abc import ABC, abstractmethod
from typing import Dict, Any
from fastapi import Request
import base64
import logging
import uuid
from ..models.aws_v1 import (
    APIGatewayProxyEvent,
    ApiGatewayRequestContext,
    ApiGatewayIdentity,
    ApiGatewayAuthorizer,
)

from services.common.core.request_context import get_request_id

logger = logging.getLogger("gateway.event_builder")


class EventBuilder(ABC):
    @abstractmethod
    async def build(self, request: Request, body: bytes, **kwargs) -> Dict[str, Any]:
        """
        Build an event dictionary from a FastAPI Request.
        kwargs can contain user_id, path_params, route_path, etc.
        """
        pass


class V1ProxyEventBuilder(EventBuilder):
    """API Gateway V1 (REST API) compatible event builder."""

    async def build(self, request: Request, body: bytes, **kwargs) -> Dict[str, Any]:
        """
        Build an API Gateway Lambda Proxy Integration-compatible event object.
        """
        user_id = kwargs.get("user_id", "anonymous")
        path_params = kwargs.get("path_params", {})
        route_path = kwargs.get("route_path", str(request.url.path))

        # Check if gzip-compressed.
        is_base64 = "gzip" in request.headers.get("content-encoding", "").lower()

        # Process body.
        if is_base64:
            body_content = base64.b64encode(body).decode("utf-8")
        else:
            try:
                body_content = body.decode("utf-8")
            except UnicodeDecodeError:
                body_content = base64.b64encode(body).decode("utf-8")
                is_base64 = True

        # Query parameters.
        query_params: Dict[str, str] = {}
        multi_query_params: Dict[str, list] = {}
        if request.query_params:
            for key in request.query_params.keys():
                values = request.query_params.getlist(key)
                query_params[key] = values[-1] if values else ""
                multi_query_params[key] = values

        # Headers.
        headers: Dict[str, str] = {}
        multi_headers: Dict[str, list] = {}
        for key in request.headers.keys():
            values = request.headers.getlist(key)
            headers[key] = values[-1] if values else ""
            multi_headers[key] = values

        # Get RequestID (from context).
        aws_request_id = get_request_id()

        # Fallback (middleware should usually generate it).
        if not aws_request_id:
            aws_request_id = str(uuid.uuid4())

        # Get HTTP version.
        http_version = (
            request.scope.get("http_version", "1.1") if hasattr(request, "scope") else "1.1"
        )

        # Build event using Pydantic models.
        event_model = APIGatewayProxyEvent(
            resource=route_path,
            path=str(request.url.path),
            httpMethod=request.method,
            headers=headers,
            multiValueHeaders=multi_headers,
            queryStringParameters=query_params if query_params else None,
            multiValueQueryStringParameters=multi_query_params if multi_query_params else None,
            pathParameters=path_params if path_params else None,
            requestContext=ApiGatewayRequestContext(
                identity=ApiGatewayIdentity(
                    sourceIp=request.client.host if request.client else "unknown",
                    userAgent=request.headers.get("user-agent"),
                ),
                authorizer=ApiGatewayAuthorizer(
                    claims={"cognito:username": user_id, "username": user_id},
                    cognito_username=user_id,
                ),
                requestId=aws_request_id,
                path=str(request.url.path),
                stage="prod",
                protocol=f"HTTP/{http_version}",
            ),
            body=body_content if body_content else None,
            isBase64Encoded=is_base64,
        )

        return event_model.model_dump(exclude_none=True, by_alias=True)
