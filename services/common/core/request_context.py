"""
RequestContext コンテキスト管理
ContextVar を使用して、非同期処理間で RequestId を共有します。
"""

from contextvars import ContextVar
from typing import Optional
import uuid

# リクエストIDを格納するコンテキスト変数
_request_id_var: ContextVar[Optional[str]] = ContextVar("request_id", default=None)


def get_request_id() -> Optional[str]:
    """現在のリクエストIDを取得"""
    return _request_id_var.get()


def set_request_id(request_id: Optional[str] = None) -> str:
    """
    リクエストIDを設定

    Args:
        request_id: 設定するリクエストID。Noneの場合は新規にUUIDを生成

    Returns:
        設定されたリクエストID
    """
    if request_id is None:
        request_id = str(uuid.uuid4())
    _request_id_var.set(request_id)
    return request_id


def clear_request_id() -> None:
    """リクエストIDをクリア"""
    _request_id_var.set(None)
