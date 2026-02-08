import importlib
import os
import sys
import urllib.request
from pathlib import Path
from unittest.mock import AsyncMock, patch

import httpx
import pytest


# Avoid starting background queue listeners in tests; we don't need async log shipping here.
def _noop_configure_queue_logging(*_args, **_kwargs):
    return None


# Add project root to sys.path to allow imports like 'services.gateway...'
project_root = str(Path(__file__).parent.parent.parent.parent.resolve())
if project_root not in sys.path:
    sys.path.insert(0, project_root)

# Patch at import time so tests that import gateway.main at module scope don't start listeners.
gateway_logging_config = importlib.import_module("services.gateway.core.logging_config")
gateway_logging_config.configure_queue_logging = _noop_configure_queue_logging

# Set Mock Environment Variables for Testing
os.environ.setdefault("GATEWAY_INTERNAL_URL", "http://test-gateway:8000")
os.environ.setdefault("JWT_SECRET_KEY", "test-secret-key-must-be-very-long-for-security")
os.environ.setdefault("X_API_KEY", "test-api-key")
os.environ.setdefault("AUTH_USER", "test-user")
os.environ.setdefault("AUTH_PASS", "test-pass")
os.environ.setdefault("CONTAINERS_NETWORK", "test-net")
os.environ.setdefault("LAMBDA_NETWORK", "test-lambda-net")


class _DummyVictoriaResponse:
    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, tb):
        return False

    def read(self):
        return b"ok"


@pytest.fixture(autouse=True)
def _mock_victorialogs(monkeypatch):
    monkeypatch.setattr(urllib.request, "urlopen", lambda *args, **kwargs: _DummyVictoriaResponse())


@pytest.fixture
def main_app():
    # Lazy import to ensure env vars are set first
    # Patch grpc and lifespan-related blocking calls
    with (
        patch("grpc.aio.insecure_channel"),
        patch(
            "services.gateway.services.pool_manager.PoolManager.cleanup_all_containers",
            return_value=0,
        ),
        patch(
            "services.gateway.services.pool_manager.PoolManager.shutdown_all",
            new_callable=AsyncMock,
        ),
        patch("services.gateway.services.janitor.HeartbeatJanitor.start", new_callable=AsyncMock),
        patch("services.gateway.services.janitor.HeartbeatJanitor.stop", new_callable=AsyncMock),
        patch("services.gateway.services.scheduler.SchedulerService.start", new_callable=AsyncMock),
        patch("services.gateway.services.scheduler.SchedulerService.stop", new_callable=AsyncMock),
        patch("services.gateway.lifecycle.init_reloader", return_value=None),
        patch("services.gateway.lifecycle.start_reloader", return_value=None),
        patch("services.gateway.lifecycle.stop_reloader", return_value=None),
    ):
        from services.gateway.main import app

        yield app


@pytest.fixture
async def async_client(main_app):
    async with main_app.router.lifespan_context(main_app):
        transport = httpx.ASGITransport(app=main_app)
        async with httpx.AsyncClient(transport=transport, base_url="http://testserver") as client:
            yield client
