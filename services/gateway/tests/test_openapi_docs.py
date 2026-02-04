"""
Where: services/gateway/tests/test_openapi_docs.py
What: Tests for OpenAPI description rendering of routing info.
Why: Ensure /docs shows routing.yml contents on each deploy.
"""


def test_openapi_description_includes_routes(client, monkeypatch):
    sample_routes = [
        {"path": "/api/hello", "method": "get", "function": "hello-func"},
        {"path": "/api/items/{id}", "method": "POST", "function": "items-func"},
    ]

    matcher = client.app.state.route_matcher
    monkeypatch.setattr(matcher, "list_routes", lambda: sample_routes)

    response = client.get("/openapi.json")
    assert response.status_code == 200

    description = response.json()["info"]["description"]
    assert "## Routing" in description
    assert "/api/hello" in description
    assert "GET" in description
    assert "items-func" in description


def test_openapi_injects_routing_paths_as_operations(client, monkeypatch):
    sample_routes = [
        {"path": "/api/hello", "method": "get", "function": "hello-func"},
        {"path": "/api/items/{id}", "method": "POST", "function": "items-func"},
        # Should be ignored (scheduled events etc.)
        {"path": "", "method": "", "function": "lambda-scheduled"},
    ]

    matcher = client.app.state.route_matcher
    monkeypatch.setattr(matcher, "list_routes", lambda: sample_routes)

    response = client.get("/openapi.json")
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
