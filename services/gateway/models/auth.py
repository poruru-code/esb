"""
認証に関連する Pydantic モデル定義
"""

from pydantic import BaseModel


class AuthParameters(BaseModel):
    """認証パラメータ"""

    USERNAME: str
    PASSWORD: str


class AuthRequest(BaseModel):
    """認証リクエスト"""

    AuthParameters: AuthParameters


class AuthenticationResult(BaseModel):
    """認証結果"""

    IdToken: str


class AuthResponse(BaseModel):
    """認証レスポンス"""

    AuthenticationResult: AuthenticationResult
