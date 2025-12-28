"""
Authentication and security module.

Generates and verifies JWT tokens.
"""

from datetime import datetime, timedelta, timezone
from typing import Optional
import jwt

# JWT algorithm.
ALGORITHM = "HS256"


def create_access_token(username: str, secret_key: str, expires_delta: int = 3600) -> str:
    """
    Generate a JWT token.

    Args:
        username: user name (set as token subject)
        secret_key: JWT signing secret key
        expires_delta: token validity in seconds

    Returns:
        Encoded JWT token
    """
    expire = datetime.now(timezone.utc) + timedelta(seconds=expires_delta)
    to_encode = {"sub": username, "exp": expire, "iat": datetime.now(timezone.utc)}
    encoded_jwt = jwt.encode(to_encode, secret_key, algorithm=ALGORITHM)
    return encoded_jwt


def verify_token(token: str, secret_key: str) -> Optional[str]:
    """
    Verify a JWT token and return the username.

    Args:
        token: Bearer token (with scheme or token only)
        secret_key: JWT signing secret key

    Returns:
        Username (None on verification failure)

    Note:
        This function provides pure verification logic only.
        Handle FastAPI Depends or HTTPException in the caller.
    """
    try:
        # Extract the token part for "Bearer token" format.
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
