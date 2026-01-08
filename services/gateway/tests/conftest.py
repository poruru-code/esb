import os
import sys
from pathlib import Path

import pytest
from starlette.testclient import TestClient

# Add project root to sys.path to allow imports like 'services.gateway...'
project_root = str(Path(__file__).parent.parent.parent.parent.resolve())
if project_root not in sys.path:
    sys.path.insert(0, project_root)

# Set Mock Environment Variables for Testing
# These must be set before 'services.gateway.config' is imported by any test
os.environ.setdefault("GATEWAY_INTERNAL_URL", "http://test-gateway:8000")
os.environ.setdefault("JWT_SECRET_KEY", "test-secret-key-must-be-very-long-for-security")
os.environ.setdefault("X_API_KEY", "test-api-key")
os.environ.setdefault("AUTH_USER", "test-user")
os.environ.setdefault("AUTH_PASS", "test-pass")
os.environ.setdefault("CONTAINERS_NETWORK", "test-net")
os.environ.setdefault("LAMBDA_NETWORK", "test-lambda-net")

# You can also add shared fixtures here if needed


@pytest.fixture
def main_app():
    # Lazy import to ensure env vars are set first
    from services.gateway.main import app

    return app


@pytest.fixture
def client(main_app):
    with TestClient(main_app) as client:
        yield client
