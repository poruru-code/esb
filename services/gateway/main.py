"""
Lambda Gateway - API Gateway compatible server.

Replicates AWS API Gateway and Lambda Authorizer behavior and forwards
requests to Lambda RIE containers based on routing.yml.
"""

import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI

from .api.openapi import configure_openapi
from .config import config
from .core.logging_config import setup_logging
from .exceptions import (
    function_not_found_handler,
    register_exception_handlers,
    resource_exhausted_handler,
)
from .lifecycle import manage_lifespan
from .middleware import trace_propagation_middleware
from .routes import (
    USER_AUTHORIZED_HEADER,
    authenticate_user,
    build_cors_headers,
    cors_preflight,
    gateway_handler,
    invoke_lambda_api,
    list_container_metrics,
    list_functions,
    list_pool_metrics,
    list_routes,
    register_routes,
    sanitize_proxy_headers,
)
from .routes import (
    health_check as _health_check_impl,
)

setup_logging()
logger = logging.getLogger("gateway.main")


@asynccontextmanager
async def lifespan(app: FastAPI):
    async with manage_lifespan(app, config):
        yield


app = FastAPI(
    title="Lambda Gateway",
    version="2.0.0",
    lifespan=lifespan,
    root_path=config.root_path,
)
configure_openapi(app)

app.middleware("http")(trace_propagation_middleware)
register_exception_handlers(app)


async def health_check():
    """Health check implementation."""
    return await _health_check_impl()


async def health_check_endpoint():
    """Health check endpoint."""
    return await health_check()


app.get("/health", include_in_schema=False)(health_check_endpoint)
register_routes(app, config)

__all__ = [
    "USER_AUTHORIZED_HEADER",
    "app",
    "authenticate_user",
    "build_cors_headers",
    "cors_preflight",
    "function_not_found_handler",
    "gateway_handler",
    "health_check",
    "health_check_endpoint",
    "invoke_lambda_api",
    "lifespan",
    "list_container_metrics",
    "list_functions",
    "list_pool_metrics",
    "list_routes",
    "resource_exhausted_handler",
    "sanitize_proxy_headers",
    "trace_propagation_middleware",
]
