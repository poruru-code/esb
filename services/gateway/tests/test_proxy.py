import pytest
from unittest.mock import AsyncMock, patch
import httpx
from services.gateway.core.proxy import proxy_to_lambda


@pytest.mark.asyncio
async def test_proxy_to_lambda_uses_shared_client():
    mock_client = AsyncMock(spec=httpx.AsyncClient)
    mock_response = httpx.Response(200, json={"message": "ok"})
    mock_client.post.return_value = mock_response

    target_container = "test-container"
    event = {"key": "value"}

    # We mock resolve_container_ip using patch, assuming it is imported in proxy.py
    with patch("services.gateway.core.proxy.resolve_container_ip") as mock_resolve:
        mock_resolve.return_value = "1.2.3.4"

        # Call with client injected
        try:
            response = await proxy_to_lambda(target_container, event, client=mock_client)
        except TypeError:
            pytest.fail("proxy_to_lambda does not accept 'client' argument")

        assert response == mock_response
        mock_client.post.assert_called_once()
        args, kwargs = mock_client.post.call_args
        assert "http://1.2.3.4:8080" in args[0]


def test_build_event_propagates_request_id_from_context():
    from services.gateway.core.proxy import build_event
    from services.common.core.request_context import set_request_id, clear_request_id
    from fastapi import Request
    from unittest.mock import Mock

    # Setup
    clear_request_id()
    expected_rid = "test-trace-id-12345"
    set_request_id(expected_rid)

    # Mock Request
    mock_request = Mock(spec=Request)
    mock_request.url.path = "/test/path"
    mock_request.method = "POST"
    mock_request.headers = {}
    mock_request.query_params = {}
    mock_request.client.host = "127.0.0.1"

    # Execute
    event = build_event(
        request=mock_request,
        body=b"{}",
        user_id="test-user",
        path_params={},
        route_path="/test/path",
    )

    # Verify
    assert event["requestContext"]["requestId"] == expected_rid, (
        f"Expected {expected_rid}, but got {event['requestContext']['requestId']}"
    )

    # Verify fallback behavior (no context)
    clear_request_id()
    event_fallback = build_event(
        request=mock_request,
        body=b"{}",
        user_id="test-user",
        path_params={},
        route_path="/test/path",
    )
    assert event_fallback["requestContext"]["requestId"] is not None
    assert event_fallback["requestContext"]["requestId"] != expected_rid
    assert (
        event_fallback["requestContext"]["requestId"].startswith("req-")
        or len(event_fallback["requestContext"]["requestId"]) > 10
    )


# =============================================================================
# TDD Red: Pydantic モデル型検証テスト
# =============================================================================


class TestAPIGatewayProxyEventModel:
    """APIGatewayProxyEvent Pydantic モデルの型検証テスト"""

    def test_model_required_fields_raises_validation_error(self):
        """必須フィールドが欠落している場合 ValidationError が発生"""
        from pydantic import ValidationError
        from services.gateway.models.aws_v1 import APIGatewayProxyEvent

        # 必須フィールドなしでインスタンス化を試みる
        with pytest.raises(ValidationError) as exc_info:
            APIGatewayProxyEvent()

        # resource, path, httpMethod, headers, multiValueHeaders, requestContext は必須
        errors = exc_info.value.errors()
        missing_fields = {e["loc"][0] for e in errors if e["type"] == "missing"}
        assert "resource" in missing_fields
        assert "path" in missing_fields
        assert "httpMethod" in missing_fields

    def test_model_type_validation_raises_error_for_invalid_types(self):
        """不正な型が渡された場合 ValidationError が発生"""
        from pydantic import ValidationError
        from services.gateway.models.aws_v1 import (
            APIGatewayProxyEvent,
            ApiGatewayRequestContext,
            ApiGatewayIdentity,
        )

        # headers に int を渡す（str を期待）
        with pytest.raises(ValidationError):
            APIGatewayProxyEvent(
                resource="/test",
                path="/test",
                httpMethod="GET",
                headers={"Content-Type": 123},  # 不正: int
                multiValueHeaders={},
                requestContext=ApiGatewayRequestContext(
                    identity=ApiGatewayIdentity(sourceIp="127.0.0.1"),
                    requestId="req-123",
                ),
            )

    def test_model_valid_construction(self):
        """正しい型で構築した場合、モデルが正常にインスタンス化される"""
        from services.gateway.models.aws_v1 import (
            APIGatewayProxyEvent,
            ApiGatewayRequestContext,
            ApiGatewayIdentity,
            ApiGatewayAuthorizer,
        )

        event = APIGatewayProxyEvent(
            resource="/users/{id}",
            path="/users/123",
            httpMethod="GET",
            headers={"Content-Type": "application/json"},
            multiValueHeaders={"Content-Type": ["application/json"]},
            requestContext=ApiGatewayRequestContext(
                identity=ApiGatewayIdentity(sourceIp="192.168.1.1", userAgent="test-agent"),
                authorizer=ApiGatewayAuthorizer(claims={"cognito:username": "testuser"}),
                requestId="req-abc123",
                stage="prod",
                protocol="HTTP/1.1",
            ),
            body='{"key": "value"}',
            isBase64Encoded=False,
        )

        assert event.resource == "/users/{id}"
        assert event.path == "/users/123"
        assert event.httpMethod == "GET"
        assert event.requestContext.identity.sourceIp == "192.168.1.1"
        assert event.requestContext.identity.userAgent == "test-agent"
        assert event.requestContext.stage == "prod"

    def test_model_dump_excludes_none(self):
        """model_dump(exclude_none=True) で None フィールドが除外される"""
        from services.gateway.models.aws_v1 import (
            APIGatewayProxyEvent,
            ApiGatewayRequestContext,
            ApiGatewayIdentity,
        )

        event = APIGatewayProxyEvent(
            resource="/test",
            path="/test",
            httpMethod="GET",
            headers={},
            multiValueHeaders={},
            queryStringParameters=None,  # 明示的に None
            requestContext=ApiGatewayRequestContext(
                identity=ApiGatewayIdentity(sourceIp="127.0.0.1"),
                requestId="req-123",
            ),
        )

        dumped = event.model_dump(exclude_none=True)
        assert "queryStringParameters" not in dumped
        assert "body" not in dumped  # デフォルト None


# =============================================================================
# TDD Red: build_event AWS 互換構造テスト
# =============================================================================


class TestBuildEventAWSCompatibility:
    """build_event 関数が AWS API Gateway 互換の構造を返すことをテスト"""

    def _create_mock_request(self):
        """テスト用のモック Request を作成"""
        from fastapi import Request
        from unittest.mock import Mock

        mock_request = Mock(spec=Request)
        mock_request.url.path = "/api/users/123"
        mock_request.method = "POST"
        mock_request.headers = Mock()
        mock_request.headers.keys.return_value = ["content-type", "user-agent"]
        mock_request.headers.getlist.side_effect = lambda k: {
            "content-type": ["application/json"],
            "user-agent": ["TestAgent/1.0"],
        }.get(k, [])
        mock_request.headers.get.side_effect = lambda k, d=None: {
            "content-type": "application/json",
            "user-agent": "TestAgent/1.0",
            "content-encoding": "",
        }.get(k, d)
        mock_request.query_params = Mock()
        mock_request.query_params.__bool__ = Mock(return_value=False)
        mock_request.client.host = "192.168.1.100"
        mock_request.scope = {"http_version": "1.1"}
        return mock_request

    def test_build_event_returns_all_required_fields(self):
        """build_event が AWS 必須フィールドをすべて含む"""
        from services.gateway.core.proxy import build_event
        from services.common.core.request_context import clear_request_id

        clear_request_id()
        mock_request = self._create_mock_request()

        event = build_event(
            request=mock_request,
            body=b'{"name": "test"}',
            user_id="user-456",
            path_params={"id": "123"},
            route_path="/api/users/{id}",
        )

        # 必須フィールドの存在確認
        assert "resource" in event
        assert "path" in event
        assert "httpMethod" in event
        assert "headers" in event
        assert "multiValueHeaders" in event
        assert "requestContext" in event
        assert "body" in event
        assert "isBase64Encoded" in event

    def test_build_event_includes_enhanced_request_context(self):
        """build_event が拡張された requestContext フィールドを含む"""
        from services.gateway.core.proxy import build_event
        from services.common.core.request_context import clear_request_id

        clear_request_id()
        mock_request = self._create_mock_request()

        event = build_event(
            request=mock_request,
            body=b"{}",
            user_id="user-789",
            path_params={},
            route_path="/test",
        )

        rc = event["requestContext"]

        # 既存フィールド
        assert "identity" in rc
        assert "authorizer" in rc
        assert "requestId" in rc

        # 新規追加フィールド（Pydantic リファクタリング後に追加される）
        assert "stage" in rc, "requestContext に stage フィールドがありません"
        assert "protocol" in rc, "requestContext に protocol フィールドがありません"
        assert rc["stage"] == "prod"
        assert rc["protocol"] == "HTTP/1.1"

        # identity の拡張
        assert "sourceIp" in rc["identity"]
        assert "userAgent" in rc["identity"], "identity に userAgent フィールドがありません"
        assert rc["identity"]["userAgent"] == "TestAgent/1.0"

    def test_build_event_authorizer_structure(self):
        """build_event の authorizer が正しい構造を持つ"""
        from services.gateway.core.proxy import build_event
        from services.common.core.request_context import clear_request_id

        clear_request_id()
        mock_request = self._create_mock_request()

        event = build_event(
            request=mock_request,
            body=b"{}",
            user_id="test-user",
            path_params={},
            route_path="/test",
        )

        authorizer = event["requestContext"]["authorizer"]

        # claims 内に cognito:username が含まれる
        assert "claims" in authorizer, "authorizer に claims フィールドがありません"
        assert "cognito:username" in authorizer["claims"]
        assert authorizer["claims"]["cognito:username"] == "test-user"

        # 互換性のためにトップレベルにも存在することを確認
        assert "cognito:username" in authorizer, (
            "authorizer のトップレベルに cognito:username がありません"
        )
        assert authorizer["cognito:username"] == "test-user"
