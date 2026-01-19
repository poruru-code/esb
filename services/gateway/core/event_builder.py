import base64
import logging
import uuid
from abc import ABC, abstractmethod
from typing import Any, Dict

from services.common.core.request_context import get_request_id
from services.gateway.models.aws_v1 import (
    ApiGatewayAuthorizer,
    ApiGatewayIdentity,
    APIGatewayProxyEvent,
    ApiGatewayRequestContext,
)
from services.gateway.models.context import InputContext

logger = logging.getLogger("gateway.event_builder")


class EventBuilder(ABC):
    @abstractmethod
    def build(self, context: InputContext) -> Dict[str, Any]:
        """
        Build an event dictionary from an InputContext.
        """
        pass


class V1ProxyEventBuilder(EventBuilder):
    """API Gateway V1 (REST API) compatible event builder."""

    def build(self, context: InputContext) -> Dict[str, Any]:
        """
        Build an API Gateway Lambda Proxy Integration-compatible event object from context.
        """
        user_id = context.user_id or "anonymous"
        path_params = context.path_params
        route_path = context.route_path or context.path
        body = context.body

        # Check if gzip-compressed.
        is_base64 = "gzip" in context.headers.get("content-encoding", "").lower()

        # Process body.
        if is_base64:
            body_content = base64.b64encode(body).decode("utf-8")
        else:
            try:
                body_content = body.decode("utf-8")
            except UnicodeDecodeError:
                body_content = base64.b64encode(body).decode("utf-8")
                is_base64 = True

        # Query parameters standardizing (V1 expected multiValue support).
        query_params: Dict[str, str] = context.query_params
        multi_query_params = context.multi_query_params

        # Headers standardizing.
        headers: Dict[str, str] = context.headers
        multi_headers = context.multi_headers

        # Get RequestID (from context).
        aws_request_id = get_request_id() or str(uuid.uuid4())

        # Build event using Pydantic models.
        event_model = APIGatewayProxyEvent(
            resource=route_path,
            path=context.path,
            httpMethod=context.method,
            headers=headers,
            multiValueHeaders=multi_headers,
            queryStringParameters=query_params if query_params else None,
            multiValueQueryStringParameters=multi_query_params if multi_query_params else None,
            pathParameters=path_params if path_params else None,
            requestContext=ApiGatewayRequestContext(
                identity=ApiGatewayIdentity(
                    sourceIp=context.headers.get("x-forwarded-for", "unknown"),
                    userAgent=context.headers.get("user-agent"),
                ),
                authorizer=ApiGatewayAuthorizer(
                    claims={"cognito:username": user_id, "username": user_id},
                    cognito_username=user_id,  # type: ignore[unknown-argument]
                ),
                requestId=aws_request_id,
                path=context.path,
                stage="prod",
                protocol="HTTP/1.1",  # Simplified
            ),
            body=body_content if body_content else None,
            isBase64Encoded=is_base64,
        )

        return event_model.model_dump(exclude_none=True, by_alias=True)
