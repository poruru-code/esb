"""
Pydanticスキーマ定義

API リクエスト/レスポンスのデータモデルを定義します。
"""

from typing import Dict, Any
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


class TargetFunction(BaseModel):
    """
    ルーティング解決によって特定された Lambda 関数の情報
    """

    container_name: str
    path_params: Dict[str, str]
    route_path: str
    function_config: Dict[str, Any]
