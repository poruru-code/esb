import pytest
import uuid
from services.common.core import request_context


def test_generate_request_id_creates_uuid():
    """generate_request_id() が新しいUUIDv4を生成してコンテキストにセットすることを確認"""
    # Act
    req_id = request_context.generate_request_id()

    # Assert
    assert req_id is not None
    assert isinstance(req_id, str)
    # UUID形式であることを確認
    try:
        uuid_obj = uuid.UUID(req_id)
        assert str(uuid_obj) == req_id
    except ValueError:
        pytest.fail(f"Generated ID is not a valid UUID: {req_id}")

    # Contextにセットされているか確認
    assert request_context.get_request_id() == req_id


def test_generate_request_id_is_unique():
    """呼び出すたびに異なるIDが生成されることを確認"""
    id1 = request_context.generate_request_id()
    id2 = request_context.generate_request_id()

    assert id1 != id2


def test_trace_id_does_not_affect_request_id():
    """Trace IDをセットしてもRequest IDには影響しないことを確認 (分離の確認)"""
    # Contextをクリア
    request_context.clear_trace_id()

    # Arrange
    trace_val = "Root=1-67890abc-def1234567890abcdef12345"

    # Act
    # 1. Trace ID セット
    request_context.set_trace_id(trace_val)

    # Assert
    # Request IDは明示的に生成するまで None であるべき (または以前の値)
    # ここでは新規コンテキストを想定して None
    # TraceId.parse() はデフォルトで ;Sampled=1 を付与するため、それを含めて検証
    expected_trace = trace_val + ";Sampled=1"
    assert request_context.get_trace_id() == expected_trace
    # 古い実装ではここで Trace ID の Root ID がセットされていたが、
    # 今は独立しているべきなので None (もしくは generate_request_id を呼んでいないので None)

    # 注意: 実装前は `request_context.py` の現状によっては None かもしれないし、
    # もし既存コードに副作用が残っていれば Trace ID 由来の値が入るかもしれない。
    # 「分離されていること」をテストしたい。
    assert request_context.get_request_id() is None
