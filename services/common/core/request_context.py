"""
RequestContext コンテキスト管理
ContextVar を使用して、非同期処理間で TraceId を共有します。
"""

from contextvars import ContextVar
from typing import Optional
from .trace import TraceId


# Trace ID (フルヘッダー形式) を格納するコンテキスト変数
_trace_id_var: ContextVar[Optional[str]] = ContextVar("trace_id", default=None)
# Request ID (UUID) を格納するコンテキスト変数
_request_id_var: ContextVar[Optional[str]] = ContextVar("request_id", default=None)


def get_trace_id() -> Optional[str]:
    """現在の Trace ID を取得"""
    return _trace_id_var.get()


def get_request_id() -> Optional[str]:
    """現在の Request ID を取得"""
    return _request_id_var.get()


def generate_request_id() -> str:
    """
    現在のコンテキスト用に新しいRequest ID (UUID) を生成してセットする。
    """
    import uuid

    new_id = str(uuid.uuid4())
    _request_id_var.set(new_id)
    return new_id


def set_trace_id(trace_id_str: str) -> str:
    """
    Trace ID を設定する

    Args:
        trace_id_str: X-Amzn-Trace-Id ヘッダー形式の文字列

    Returns:
        設定されたフル Trace ID 文字列
    """
    try:
        trace = TraceId.parse(trace_id_str)
        _trace_id_var.set(str(trace))
        return str(trace)
    except Exception as e:
        raise e


def clear_trace_id() -> None:
    """Trace ID コンテキストをクリア"""
    _trace_id_var.set(None)
    _request_id_var.set(None)
