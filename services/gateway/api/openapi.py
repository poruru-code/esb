"""
Gateway OpenAPI customization.

Where: services/gateway/api/openapi.py
What: Build OpenAPI descriptions with runtime routing info.
Why: Show routing.yml contents directly in /docs for each deploy.
"""

import re
from typing import Any, Mapping, Sequence

from fastapi import FastAPI
from fastapi.openapi.utils import get_openapi

_ROUTING_OPENAPI_METHODS = {"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD"}
_ROUTING_OPENAPI_BODY_METHODS = {"POST", "PUT", "PATCH", "DELETE"}
_PATH_PARAM_RE = re.compile(r"{(\w+)}")


def configure_openapi(app: FastAPI) -> None:
    """Inject routing info into the OpenAPI description used by Swagger UI."""

    # NOTE: FastAPI's default /openapi.json handler calls `self.openapi()`.
    # When overriding `app.openapi` on an instance, Python does *not* bind the
    # function like a normal method, so the override must be a 0-arg callable.
    def custom_openapi() -> dict[str, Any]:
        runtime_routes = _get_routes(app)

        schema = get_openapi(
            title=app.title,
            version=app.version,
            description=_build_description(app, runtime_routes),
            routes=app.routes,
        )
        _inject_bearer_auth_security_scheme(schema)
        _inject_routing_paths(schema, runtime_routes)
        return schema

    # FastAPI supports overriding app.openapi at runtime; type stubs are too strict.
    app.openapi = custom_openapi  # type: ignore[invalid-assignment]


def _build_description(app: FastAPI, routes: Sequence[Mapping[str, Any]]) -> str:
    base_description = app.description or ""
    routes_section = _build_routes_section(routes)
    if base_description:
        return f"{base_description}\n\n{routes_section}"
    return routes_section


def _get_routes(app: FastAPI) -> Sequence[Mapping[str, Any]]:
    matcher = getattr(app.state, "route_matcher", None)
    if matcher is None:
        return ()
    try:
        return matcher.list_routes()
    except Exception:
        return ()


def _inject_bearer_auth_security_scheme(schema: dict[str, Any]) -> None:
    # NOTE: FastAPI generates the OpenAPI document as a plain dict. We mutate it in-place.
    components = schema.setdefault("components", {})
    if not isinstance(components, dict):
        return

    security_schemes = components.setdefault("securitySchemes", {})
    if not isinstance(security_schemes, dict):
        return

    security_schemes.setdefault(
        "bearerAuth",
        {"type": "http", "scheme": "bearer", "bearerFormat": "JWT"},
    )


def _inject_routing_paths(schema: dict[str, Any], routes: Sequence[Mapping[str, Any]]) -> None:
    """
    Add "virtual" path operations for routing.yml entries.

    These paths are served by the gateway catch-all route at runtime, but are not visible to
    FastAPI's router (and therefore not included in OpenAPI) unless we inject them ourselves.
    """
    if not routes:
        return

    paths = schema.setdefault("paths", {})
    if not isinstance(paths, dict):
        return

    for route in routes:
        method = _format_method(route)
        path = _format_path(route)
        function = _format_function(route)

        if method not in _ROUTING_OPENAPI_METHODS:
            continue
        if not path or path == "-":
            continue

        path_item = paths.setdefault(path, {})
        if not isinstance(path_item, dict):
            continue

        method_key = method.lower()
        if method_key in path_item:
            # Don't override existing endpoints (if any).
            continue

        description = f"Target function: `{function}`\n\n_Source: routing.yml (runtime config)._"
        operation: dict[str, Any] = {
            "tags": ["Routing"],
            "summary": f"{method} {path}",
            "description": description,
            "operationId": _build_operation_id(function=function, method=method_key, path=path),
            "security": [{"bearerAuth": []}],
            "responses": {
                "200": {"description": "Lambda response"},
                "401": {"description": "Unauthorized"},
                "404": {"description": "Not Found"},
            },
        }

        params = _build_path_parameters(path)
        if params:
            operation["parameters"] = params

        if method in _ROUTING_OPENAPI_BODY_METHODS:
            operation["requestBody"] = {
                "required": False,
                "content": {"application/json": {"schema": {}}},
            }

        path_item[method_key] = operation


def _build_path_parameters(path: str) -> list[dict[str, Any]]:
    return [
        {
            "name": name,
            "in": "path",
            "required": True,
            "schema": {"type": "string"},
        }
        for name in _PATH_PARAM_RE.findall(path)
    ]


def _build_operation_id(*, function: str, method: str, path: str) -> str:
    safe_function = re.sub(r"[^a-zA-Z0-9_]+", "_", function).strip("_") or "unknown"
    safe_path = re.sub(r"[^a-zA-Z0-9_]+", "_", path).strip("_") or "root"
    return f"routing_{safe_function}_{method}_{safe_path}"


def _build_routes_section(routes: Sequence[Mapping[str, Any]]) -> str:
    header = "## Routing"
    if not routes:
        return f"{header}\n\nNo routes loaded from routing.yml."

    lines = [
        header,
        "",
        "| Method | Path | Function |",
        "| --- | --- | --- |",
    ]

    for route in routes:
        method = _escape_cell(_format_method(route))
        path = _escape_cell(_format_path(route))
        function = _escape_cell(_format_function(route))
        lines.append(f"| {method} | {path} | {function} |")

    lines.append("")
    lines.append("_Source: routing.yml (runtime config)._")
    return "\n".join(lines)


def _format_method(route: Mapping[str, Any]) -> str:
    value = route.get("method")
    if isinstance(value, str) and value.strip():
        return value.strip().upper()
    return "-"


def _format_path(route: Mapping[str, Any]) -> str:
    value = route.get("path")
    if isinstance(value, str) and value.strip():
        return value.strip()
    return "-"


def _format_function(route: Mapping[str, Any]) -> str:
    value = route.get("function")
    if isinstance(value, str) and value.strip():
        return value.strip()
    if isinstance(value, dict):
        container = value.get("container")
        if isinstance(container, str) and container.strip():
            return container.strip()
    return "-"


def _escape_cell(value: str) -> str:
    return value.replace("|", "\\|").replace("\n", " ").strip() or "-"
