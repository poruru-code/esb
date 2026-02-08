"""
Where: services/gateway/tests/test_openapi_docs.py
What: Tests for OpenAPI injection of routing.yml routes.
Why: Ensure /docs can execute routed endpoints via Swagger UI after each deploy.
"""

import pytest


@pytest.mark.asyncio
async def test_openapi_description_does_not_include_routing_markdown(
    async_client, main_app, monkeypatch
):
    sample_routes = [
        {"path": "/api/hello", "method": "get", "function": "hello-func"},
        {"path": "/api/items/{id}", "method": "POST", "function": "items-func"},
    ]

    matcher = main_app.state.route_matcher
    monkeypatch.setattr(matcher, "list_routes", lambda: sample_routes)

    response = await async_client.get("/openapi.json")
    assert response.status_code == 200

    description = response.json()["info"].get("description") or ""
    assert "## Routing" not in description


@pytest.mark.asyncio
async def test_openapi_injects_routing_paths_as_operations(async_client, main_app, monkeypatch):
    sample_routes = [
        {"path": "/api/hello", "method": "get", "function": "hello-func"},
        {"path": "/api/items/{id}", "method": "POST", "function": "items-func"},
        # Should be ignored (scheduled events etc.)
        {"path": "", "method": "", "function": "lambda-scheduled"},
    ]

    matcher = main_app.state.route_matcher
    monkeypatch.setattr(matcher, "list_routes", lambda: sample_routes)

    response = await async_client.get("/openapi.json")
    assert response.status_code == 200

    schema = response.json()
    assert schema["components"]["securitySchemes"]["bearerAuth"]["scheme"] == "bearer"

    paths = schema["paths"]
    assert "/api/hello" in paths
    assert "get" in paths["/api/hello"]
    assert {"bearerAuth": []} in paths["/api/hello"]["get"]["security"]

    assert "/api/items/{id}" in paths
    post_op = paths["/api/items/{id}"]["post"]
    param_names = {p["name"] for p in post_op.get("parameters", [])}
    assert "id" in param_names
    assert {"bearerAuth": []} in post_op["security"]
