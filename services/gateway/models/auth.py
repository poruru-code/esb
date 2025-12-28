"""
Pydantic models related to authentication.
"""

from pydantic import BaseModel


class AuthParameters(BaseModel):
    """Authentication parameters."""

    USERNAME: str
    PASSWORD: str


class AuthRequest(BaseModel):
    """Authentication request."""

    AuthParameters: AuthParameters


class AuthenticationResult(BaseModel):
    """Authentication result."""

    IdToken: str


class AuthResponse(BaseModel):
    """Authentication response."""

    AuthenticationResult: AuthenticationResult
