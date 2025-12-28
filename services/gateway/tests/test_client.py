"""
ManagerClient tests (TDD Red phase).

Test error mapping and RequestId propagation.
These tests are expected to fail until implementation is complete.
"""

import pytest
from unittest.mock import AsyncMock, MagicMock
import httpx
from services.gateway.client import OrchestratorClient as ManagerClient


@pytest.fixture
def mock_client():
    client = AsyncMock(spec=httpx.AsyncClient)
    return client


@pytest.mark.asyncio
async def test_ensure_container_success(mock_client):
    """Happy path: container starts successfully."""
    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 200
    mock_response.json.return_value = {"host": "10.0.0.1", "port": 8080}
    mock_response.raise_for_status = MagicMock()

    mock_client.post.return_value = mock_response

    manager_client = ManagerClient(mock_client)
    host = await manager_client.ensure_container("test-func", "test-image")

    assert host == "10.0.0.1"
    mock_client.post.assert_called_once()


@pytest.mark.asyncio
async def test_ensure_container_network_failure(mock_client):
    """Connection failure to Manager -> OrchestratorUnreachableError."""
    from services.gateway.core.exceptions import OrchestratorUnreachableError

    mock_client.post.side_effect = httpx.RequestError("Connection failed", request=MagicMock())

    manager_client = ManagerClient(mock_client)

    with pytest.raises(OrchestratorUnreachableError):
        await manager_client.ensure_container("test-func")


@pytest.mark.asyncio
async def test_ensure_container_timeout(mock_client):
    """Manager timeout -> OrchestratorTimeoutError."""
    from services.gateway.core.exceptions import OrchestratorTimeoutError

    mock_client.post.side_effect = httpx.TimeoutException("Timeout", request=MagicMock())

    manager_client = ManagerClient(mock_client)

    with pytest.raises(OrchestratorTimeoutError):
        await manager_client.ensure_container("test-func")


@pytest.mark.asyncio
async def test_ensure_container_404_function_not_found(mock_client):
    """Manager 404 -> FunctionNotFoundError"""
    from services.gateway.core.exceptions import FunctionNotFoundError

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 404
    mock_response.text = "Image not found"

    mock_client.post.return_value = mock_response
    mock_client.post.return_value.raise_for_status.side_effect = httpx.HTTPStatusError(
        "404", request=MagicMock(), response=mock_response
    )

    manager_client = ManagerClient(mock_client)

    with pytest.raises(FunctionNotFoundError):
        await manager_client.ensure_container("test-func")


@pytest.mark.asyncio
async def test_ensure_container_400_docker_error(mock_client):
    """Manager 400 -> OrchestratorError"""
    from services.gateway.core.exceptions import OrchestratorError

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 400
    mock_response.text = "Docker API error"

    mock_client.post.return_value = mock_response
    mock_client.post.return_value.raise_for_status.side_effect = httpx.HTTPStatusError(
        "400", request=MagicMock(), response=mock_response
    )

    manager_client = ManagerClient(mock_client)

    with pytest.raises(OrchestratorError) as exc_info:
        await manager_client.ensure_container("test-func")

    assert exc_info.value.status_code == 400


@pytest.mark.asyncio
async def test_trace_id_propagation(mock_client):
    """TraceId propagates via X-Amzn-Trace-Id header."""
    from services.common.core.request_context import set_trace_id, clear_trace_id

    # Set TraceId.
    test_trace_id = "Root=1-abcdef01-1234567890abcdef12345678;Sampled=1"
    set_trace_id(test_trace_id)

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 200
    mock_response.json.return_value = {"host": "10.0.0.1", "port": 8080}
    mock_response.raise_for_status = MagicMock()
    mock_client.post.return_value = mock_response

    manager_client = ManagerClient(mock_client)
    await manager_client.ensure_container("test-func")

    # Verify X-Amzn-Trace-Id header is set.
    args, kwargs = mock_client.post.call_args
    assert "headers" in kwargs
    assert kwargs["headers"]["X-Amzn-Trace-Id"] == test_trace_id

    clear_trace_id()


# ===========================================
# Cache Integration Tests (TDD Red Phase 2)
# ===========================================


@pytest.mark.asyncio
async def test_ensure_container_cache_hit(mock_client):
    """Skip Manager HTTP request on cache hit."""
    from services.gateway.services.container_cache import ContainerHostCache

    cache = ContainerHostCache()
    cache.set("test-func", "cached-host")

    manager_client = ManagerClient(mock_client, cache=cache)
    host = await manager_client.ensure_container("test-func")

    assert host == "cached-host"
    mock_client.post.assert_not_called()  # No HTTP request.


@pytest.mark.asyncio
async def test_ensure_container_cache_miss_then_cache(mock_client):
    """On cache miss, call Manager and cache the result."""
    from services.gateway.services.container_cache import ContainerHostCache

    cache = ContainerHostCache()

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 200
    mock_response.json.return_value = {"host": "new-host", "port": 8080}
    mock_response.raise_for_status = MagicMock()
    mock_client.post.return_value = mock_response

    manager_client = ManagerClient(mock_client, cache=cache)

    # First call - cache miss, should call Manager
    host1 = await manager_client.ensure_container("test-func")
    assert host1 == "new-host"
    assert mock_client.post.call_count == 1

    # Second call - cache hit, should NOT call Manager
    host2 = await manager_client.ensure_container("test-func")
    assert host2 == "new-host"
    assert mock_client.post.call_count == 1  # Still 1, no new call


@pytest.mark.asyncio
async def test_invalidate_cache_clears_entry(mock_client):
    """ManagerClient.invalidate_cache() clears the cache."""
    from services.gateway.services.container_cache import ContainerHostCache

    cache = ContainerHostCache()
    cache.set("test-func", "cached-host")

    manager_client = ManagerClient(mock_client, cache=cache)

    # Invalidate cache
    manager_client.invalidate_cache("test-func")

    # Cache should be cleared
    assert cache.get("test-func") is None


@pytest.mark.asyncio
async def test_cache_miss_retry_after_invalidation(mock_client):
    """After cache invalidation, query Manager again."""
    from services.gateway.services.container_cache import ContainerHostCache

    cache = ContainerHostCache()
    cache.set("test-func", "old-host")

    mock_response = MagicMock(spec=httpx.Response)
    mock_response.status_code = 200
    mock_response.json.return_value = {"host": "new-host", "port": 8080}
    mock_response.raise_for_status = MagicMock()
    mock_client.post.return_value = mock_response

    manager_client = ManagerClient(mock_client, cache=cache)

    # First call - cache hit, no Manager call
    host1 = await manager_client.ensure_container("test-func")
    assert host1 == "old-host"
    assert mock_client.post.call_count == 0

    # Invalidate cache (simulating Lambda connection failure)
    manager_client.invalidate_cache("test-func")

    # Second call - cache miss, should call Manager
    host2 = await manager_client.ensure_container("test-func")
    assert host2 == "new-host"
    assert mock_client.post.call_count == 1  # Now called Manager


# ===========================================
# Singleflight Tests (TDD Red Phase)
# ===========================================


@pytest.mark.asyncio
async def test_singleflight_coalesces_concurrent_requests(mock_client):
    """Coalesce concurrent requests into a single Manager call (thundering herd)."""
    import asyncio
    from services.gateway.services.container_cache import ContainerHostCache

    cache = ContainerHostCache()
    call_count = 0

    async def slow_manager_response(*args, **kwargs):
        nonlocal call_count
        call_count += 1
        await asyncio.sleep(0.1)  # Simulate Manager processing.
        mock_response = MagicMock(spec=httpx.Response)
        mock_response.status_code = 200
        mock_response.json.return_value = {"host": "coalesced-host", "port": 8080}
        mock_response.raise_for_status = MagicMock()
        return mock_response

    mock_client.post.side_effect = slow_manager_response

    manager_client = ManagerClient(mock_client, cache=cache)

    # Issue three concurrent requests.
    results = await asyncio.gather(
        manager_client.ensure_container("test-func"),
        manager_client.ensure_container("test-func"),
        manager_client.ensure_container("test-func"),
    )

    # All receive the same result.
    assert results == ["coalesced-host", "coalesced-host", "coalesced-host"]

    # Only one call to Manager.
    assert call_count == 1


@pytest.mark.asyncio
async def test_singleflight_propagates_error_to_all_waiters(mock_client):
    """Errors propagate to all waiters."""
    import asyncio
    from services.gateway.services.container_cache import ContainerHostCache
    from services.gateway.core.exceptions import OrchestratorUnreachableError

    cache = ContainerHostCache()

    async def failing_manager_response(*args, **kwargs):
        await asyncio.sleep(0.1)
        raise httpx.RequestError("Manager down", request=MagicMock())

    mock_client.post.side_effect = failing_manager_response

    manager_client = ManagerClient(mock_client, cache=cache)

    # Issue three concurrent requests (collect errors with return_exceptions=True).
    results = await asyncio.gather(
        manager_client.ensure_container("test-func"),
        manager_client.ensure_container("test-func"),
        manager_client.ensure_container("test-func"),
        return_exceptions=True,
    )

    # Everyone receives the same error.
    assert len(results) == 3
    for err in results:
        assert isinstance(err, OrchestratorUnreachableError)
