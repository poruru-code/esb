"""
Data model definitions package.

Aggregates Pydantic models for use in other modules.
"""

from .auth import (
    AuthParameters,
    AuthRequest,
    AuthenticationResult,
    AuthResponse,
)
from .aws_v1 import APIGatewayProxyEvent
from .target_function import TargetFunction

__all__ = [
    "AuthParameters",
    "AuthRequest",
    "AuthenticationResult",
    "AuthResponse",
    "APIGatewayProxyEvent",
    "TargetFunction",
]
