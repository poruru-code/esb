from fastapi.testclient import TestClient
import respx
from httpx import Response
from unittest.mock import patch
import sys
import os

# Add the project root to sys.path
sys.path.append(os.path.abspath(os.path.join(os.path.dirname(__file__), "../../../")))

try:
    from services.gateway.main import app
except ImportError:
    from unittest.mock import MagicMock

    app = MagicMock()

# Mock dependencies to avoid file loading issues
with (
    patch("services.gateway.main.load_routing_config"),
    patch("services.gateway.main.load_functions_config"),
):
    client = TestClient(app)


@respx.mock
def test_gateway_delegates_to_manager():
    """Gateway verifies that it hits Manager API to get Lambda host info"""

    # Inject config directly into registry
    from services.gateway.services import function_registry

    function_registry._function_registry["lambda-hello"] = {
        "image": "lambda-hello:latest",
        "environment": {},
    }

    # Manager API mock
    manager_route = respx.post("http://manager:8081/containers/ensure").mock(
        return_value=Response(200, json={"host": "10.0.0.5", "port": 8080})
    )

    # Lambda RIE mock (Proxy destination)
    # Note: URL pattern might be specific to implementation (httpx proxy)
    # We expect Gateway to forward to http://10.0.0.5:8080/...
    respx.post("http://10.0.0.5:8080/2015-03-31/functions/function/invocations").mock(
        return_value=Response(200, json={"message": "hello from lambda"})
    )

    # Execute Request to Gateway
    response = client.post("/2015-03-31/functions/lambda-hello/invocations", json={})

    # Verify
    assert response.status_code == 200
    assert response.json() == {"message": "hello from lambda"}

    # Verify Manager was called
    import json

    content = json.loads(manager_route.calls.last.request.content)
    assert content["function_name"] == "lambda-hello"
    assert content["image"] == "lambda-hello:latest"
