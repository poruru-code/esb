"""
Gateway OpenAPI customization.

Where: services/gateway/api/openapi.py
What: Build OpenAPI descriptions with runtime routing info.
Why: Show routing.yml contents directly in /docs for each deploy.
"""

from typing import Any, Mapping, Sequence

from fastapi import FastAPI
from fastapi.openapi.utils import get_openapi


def configure_openapi(app: FastAPI) -> None:
    """Inject routing info into the OpenAPI description used by Swagger UI."""

    # NOTE: FastAPI's default /openapi.json handler calls `self.openapi()`.
    # When overriding `app.openapi` on an instance, Python does *not* bind the
    # function like a normal method, so the override must be a 0-arg callable.
    def custom_openapi() -> dict[str, Any]:
        return get_openapi(
            title=app.title,
            version=app.version,
            description=_build_description(app),
            routes=app.routes,
        )

    # FastAPI supports overriding app.openapi at runtime; type stubs are too strict.
    app.openapi = custom_openapi  # type: ignore[invalid-assignment]


def _build_description(app: FastAPI) -> str:
    base_description = app.description or ""
    routes_section = _build_routes_section(_get_routes(app))
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
