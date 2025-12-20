"""
認証・セキュリティモジュール

JWT トークンの生成と検証を行います。
"""

from datetime import datetime, timedelta, timezone
from typing import Optional
import jwt

# JWT アルゴリズム
ALGORITHM = "HS256"


def create_access_token(username: str, secret_key: str, expires_delta: int = 3600) -> str:
    """
    JWTトークンを生成

    Args:
        username: ユーザー名（トークンのsubjectに設定）
        secret_key: JWT署名用シークレットキー
        expires_delta: トークン有効期間（秒）

    Returns:
        エンコードされたJWTトークン
    """
    expire = datetime.now(timezone.utc) + timedelta(seconds=expires_delta)
    to_encode = {"sub": username, "exp": expire, "iat": datetime.now(timezone.utc)}
    encoded_jwt = jwt.encode(to_encode, secret_key, algorithm=ALGORITHM)
    return encoded_jwt


def verify_token(token: str, secret_key: str) -> Optional[str]:
    """
    JWTトークンを検証してユーザー名を返す

    Args:
        token: Bearer トークン（スキーム付きまたはトークンのみ）
        secret_key: JWT署名用シークレットキー

    Returns:
        ユーザー名（検証失敗時はNone）

    Note:
        この関数は純粋な検証ロジックのみを提供します。
        FastAPIのDependsやHTTPExceptionは呼び出し側で処理してください。
    """
    try:
        # "Bearer token" 形式の場合はトークン部分を抽出
        if " " in token:
            scheme, token = token.split(None, 1)
            if scheme.lower() != "bearer":
                return None

        payload = jwt.decode(token, secret_key, algorithms=[ALGORITHM])
        username = payload.get("sub")
        return username if username else None

    except jwt.ExpiredSignatureError:
        return None
    except (jwt.exceptions.DecodeError, jwt.exceptions.PyJWTError):
        return None
    except ValueError:
        return None
