"""
Lambda Gateway - API Gateway compatible server

Replicates AWS API Gateway and Lambda Authorizer behavior and forwards
requests to Lambda RIE containers based on routing.yml.
"""

from contextlib import asynccontextmanager
from dataclasses import asdict
from fastapi import FastAPI, Request, HTTPException, Header, BackgroundTasks
from fastapi.responses import JSONResponse, Response
from fastapi.exceptions import RequestValidationError
from starlette.exceptions import HTTPException as StarletteHTTPException
from typing import Optional
from datetime import datetime, timezone
import asyncio
import httpx
import logging
import json
from .config import config
from .core.security import create_access_token
from .core.utils import parse_lambda_response
from .models import AuthRequest, AuthResponse, AuthenticationResult
from .core.event_builder import V1ProxyEventBuilder

# Services Imports
from .services.function_registry import FunctionRegistry
from .services.route_matcher import RouteMatcher
from .services.lambda_invoker import LambdaInvoker
from .services.pool_manager import PoolManager
from .services.janitor import HeartbeatJanitor

from .api.deps import (
    UserIdDep,
    LambdaTargetDep,
    LambdaInvokerDep,
    FunctionRegistryDep,
    EventBuilderDep,
    PoolManagerDep,
)
from .core.logging_config import setup_logging
from services.common.core.http_client import HttpClientFactory
from .core.exceptions import (
    global_exception_handler,
    http_exception_handler,
    validation_exception_handler,
    ContainerStartError,
    LambdaExecutionError,
    FunctionNotFoundError,
    ResourceExhaustedError,
)

# Logger setup
setup_logging()
logger = logging.getLogger("gateway.main")


# ===========================================
# Middleware
# ===========================================


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Manage application lifecycle."""
    # Initialize shared HTTP client
    # timeout config can be fine-tuned
    factory = HttpClientFactory(config)
    factory.configure_global_settings()
    client = factory.create_async_client(timeout=config.LAMBDA_INVOKE_TIMEOUT)

    # Initialize Services
    function_registry = FunctionRegistry()
    route_matcher = RouteMatcher(function_registry)

    # Load initial configs
    function_registry.load_functions_config()
    route_matcher.load_routing_config()

    # === Auto-Scaling: Pool Initialization ===
    def config_loader(function_name: str):
        """Load scaling config for a function"""
        func_config = function_registry.get_function_config(function_name) or {}
        return {
            "scaling": {
                "max_capacity": func_config.get("scaling", {}).get(
                    "max_capacity", config.DEFAULT_MAX_CAPACITY
                ),
                "min_capacity": func_config.get("scaling", {}).get(
                    "min_capacity", config.DEFAULT_MIN_CAPACITY
                ),
                "acquire_timeout": func_config.get("scaling", {}).get(
                    "acquire_timeout", config.POOL_ACQUIRE_TIMEOUT
                ),
            }
        }

    logger.info(f"Initializing Gateway with Go Agent gRPC Backend: {config.AGENT_GRPC_ADDRESS}")

    # New ARCH: PoolManager -> GrpcProvisionClient -> Agent
    from .services.grpc_provision import GrpcProvisionClient
    import grpc
    from .pb import agent_pb2_grpc

    # shared channel for lifecycle and info
    channel = grpc.aio.insecure_channel(config.AGENT_GRPC_ADDRESS)
    agent_stub = agent_pb2_grpc.AgentServiceStub(channel)

    grpc_provision_client = GrpcProvisionClient(agent_stub, function_registry)

    pool_manager = PoolManager(provision_client=grpc_provision_client, config_loader=config_loader)

    # Cleanup orphan containers from previous runs
    await pool_manager.cleanup_all_containers()

    invocation_backend = pool_manager

    janitor = HeartbeatJanitor(
        pool_manager,
        manager_client=None,  # gRPC mode doesn't need manager heartbeats
        interval=config.HEARTBEAT_INTERVAL,
        idle_timeout=config.GATEWAY_IDLE_TIMEOUT_SECONDS,
    )
    await janitor.start()

    # Create LambdaInvoker with chosen backend
    lambda_invoker = LambdaInvoker(
        client=client,
        registry=function_registry,
        config=config,
        backend=invocation_backend,
    )

    # Store in app.state for DI
    app.state.http_client = client
    app.state.function_registry = function_registry
    app.state.route_matcher = route_matcher
    app.state.lambda_invoker = lambda_invoker
    app.state.event_builder = V1ProxyEventBuilder()
    app.state.pool_manager = pool_manager

    logger.info("Gateway initialized with shared resources.")

    yield

    # Cleanup
    if janitor:
        await janitor.stop()

    if pool_manager:
        await pool_manager.shutdown_all()

    logger.info("Gateway shutting down, closing http client.")
    await client.aclose()


app = FastAPI(
    title="Lambda Gateway", version="2.0.0", lifespan=lifespan, root_path=config.root_path
)


# Register middleware (decorator style).
@app.middleware("http")
async def trace_propagation_middleware(request: Request, call_next):
    """
    Middleware for Trace ID propagation and structured access logging.
    """
    import time
    from services.common.core.trace import TraceId
    from services.common.core.request_context import (
        set_trace_id,
        clear_trace_id,
        generate_request_id,
    )

    start_time = time.perf_counter()

    # Get or generate Trace ID.
    trace_id_str = request.headers.get("X-Amzn-Trace-Id")

    if trace_id_str:
        try:
            set_trace_id(trace_id_str)
        except Exception as e:
            logger.warning(
                f"Failed to parse incoming X-Amzn-Trace-Id: '{trace_id_str}', error: {e}"
            )
            # Force regeneration on invalid format.
            trace = TraceId.generate()
            trace_id_str = str(trace)
            set_trace_id(trace_id_str)
    else:
        # Generate a new one if missing.
        trace = TraceId.generate()
        trace_id_str = str(trace)
        set_trace_id(trace_id_str)

    # Generate Request ID (independent of Trace ID).
    req_id = generate_request_id()

    # Await response.
    try:
        response = await call_next(request)

        # Attach to response headers.
        response.headers["X-Amzn-Trace-Id"] = trace_id_str
        response.headers["x-amzn-RequestId"] = req_id

        # Calculate process time
        process_time = time.perf_counter() - start_time
        process_time_ms = round(process_time * 1000, 2)

        # Structured Access Log
        logger.info(
            f"{request.method} {request.url.path} {response.status_code}",
            extra={
                "trace_id": trace_id_str,
                "aws_request_id": req_id,
                "method": request.method,
                "path": request.url.path,
                "status": response.status_code,
                "latency_ms": process_time_ms,
                "user_agent": request.headers.get("user-agent"),
                "client_ip": request.client.host if request.client else None,
            },
        )

        return response
    finally:
        # Cleanup.
        clear_trace_id()


# Register exception handlers.
app.add_exception_handler(Exception, global_exception_handler)
app.add_exception_handler(StarletteHTTPException, http_exception_handler)
app.add_exception_handler(RequestValidationError, validation_exception_handler)


@app.exception_handler(FunctionNotFoundError)
async def function_not_found_handler(request: Request, exc: FunctionNotFoundError):
    return JSONResponse(
        status_code=404,
        content={"message": str(exc)},
    )


@app.exception_handler(ResourceExhaustedError)
async def resource_exhausted_handler(request: Request, exc: ResourceExhaustedError):
    return JSONResponse(
        status_code=429,
        content={"message": "Too Many Requests", "detail": str(exc)},
    )


# ===========================================
# Endpoint definitions.
# ===========================================


@app.post(config.AUTH_ENDPOINT_PATH, response_model=AuthResponse)
async def authenticate_user(
    request: AuthRequest, response: Response, x_api_key: Optional[str] = Header(None)
):
    """User authentication endpoint."""
    if not x_api_key or x_api_key != config.X_API_KEY:
        logger.warning("Auth failed. Invalid API Key received.")
        raise HTTPException(status_code=401, detail="Unauthorized")

    response.headers["PADMA_USER_AUTHORIZED"] = "true"

    username = request.AuthParameters.USERNAME
    password = request.AuthParameters.PASSWORD

    if username == config.AUTH_USER and password == config.AUTH_PASS:
        id_token = create_access_token(
            username=username,
            secret_key=config.JWT_SECRET_KEY,
            expires_delta=config.JWT_EXPIRES_DELTA,
        )
        return AuthResponse(AuthenticationResult=AuthenticationResult(IdToken=id_token))

    return JSONResponse(
        status_code=401,
        content={"message": "Unauthorized"},
        headers={"PADMA_USER_AUTHORIZED": "true"},
    )


@app.get("/health")
async def health_check():
    """Health check endpoint."""
    return {"status": "healthy", "timestamp": datetime.now(timezone.utc).isoformat()}


@app.get("/metrics/containers")
async def list_container_metrics(user_id: UserIdDep, pool_manager: PoolManagerDep):
    """Agent からコンテナメトリクスを取得"""
    containers = await pool_manager.provision_client.list_containers()
    if not containers:
        return {"containers": []}

    tasks = [pool_manager.provision_client.get_container_metrics(c.id) for c in containers]
    results = await asyncio.gather(*tasks, return_exceptions=True)

    metrics_list = []
    failures = 0
    for container, result in zip(containers, results):
        if isinstance(result, Exception):
            logger.error(f"Failed to fetch metrics for container {container.id}: {result}")
            failures += 1
            metrics_list.append(
                {
                    "container_id": container.id,
                    "container_name": container.name,
                    "error": str(result),
                }
            )
            continue
        metrics_list.append(asdict(result))

    if failures == len(containers):
        raise HTTPException(
            status_code=503,
            detail="Container metrics are unavailable from Agent runtime",
        )

    return {"containers": metrics_list, "failures": failures}


# ===========================================
# AWS Lambda Service Compatible Endpoint
# ===========================================


@app.post("/2015-03-31/functions/{function_name}/invocations")
async def invoke_lambda_api(
    function_name: str,
    request: Request,
    background_tasks: BackgroundTasks,
    invoker: LambdaInvokerDep,
    registry: FunctionRegistryDep,
):
    """
    AWS Lambda Invoke API compatible endpoint.
    Handles requests from boto3.client('lambda').invoke().

    InvocationType:
      - RequestResponse (default): synchronous, return result
      - Event: asynchronous, return 202 immediately
    """
    # Retrieve dependencies (Now injected via DI)

    # Check function existence (for 404).
    if registry.get_function_config(function_name) is None:
        return JSONResponse(
            status_code=404,
            content={"message": f"Function not found: {function_name}"},
        )

    invocation_type = request.headers.get("X-Amz-Invocation-Type", "RequestResponse")
    body = await request.body()

    try:
        if invocation_type == "Event":
            # Async invoke: run in background, return 202 immediately.
            background_tasks.add_task(invoker.invoke_function, function_name, body)
            return Response(status_code=202, content=b"", media_type="application/json")
        else:
            # Sync invoke: wait for the result.
            resp = await invoker.invoke_function(function_name, body)
            # Pass through the RIE response to the client (boto3).
            return Response(
                content=resp.content,
                status_code=resp.status_code,
                headers=dict(resp.headers),
                media_type="application/json",
            )
    except ContainerStartError as e:
        return JSONResponse(status_code=503, content={"message": str(e)})
    except LambdaExecutionError as e:
        return JSONResponse(status_code=502, content={"message": str(e)})


@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "DELETE"])
async def gateway_handler(
    request: Request,
    path: str,
    user_id: UserIdDep,
    target: LambdaTargetDep,
    event_builder: EventBuilderDep,
    invoker: LambdaInvokerDep,
):
    """
    Catch-all route: forward to Lambda RIE based on routing.yml.

    Authentication and routing resolution are handled via DI.
    """
    # Build Event and Invoke Lambda
    try:
        body = await request.body()
        event = await event_builder.build(
            request=request,
            body=body,
            user_id=user_id,
            path_params=target.path_params,
            route_path=target.route_path,
        )

        # Invoke Lambda via LambdaInvoker (handles container ensure & RIE req)
        payload = json.dumps(event).encode("utf-8")
        lambda_response = await invoker.invoke_function(target.container_name, payload)

        # Transform response.
        result = parse_lambda_response(lambda_response)
        if "raw_content" in result:
            return Response(
                content=result["raw_content"],
                status_code=result["status_code"],
                headers=result["headers"],
            )
        return JSONResponse(
            status_code=result["status_code"], content=result["content"], headers=result["headers"]
        )

    except httpx.RequestError as e:
        # Invalidate cache on Lambda connection failure.
        # Next request re-queries the orchestrator and restarts the container.
        logger.error(
            f"Lambda connection failed for {target.container_name}",
            extra={
                "container_name": target.container_name,
                "port": config.LAMBDA_PORT,
                "timeout": config.LAMBDA_INVOKE_TIMEOUT,
                "error_type": type(e).__name__,
                "error_detail": str(e),
            },
            exc_info=True,
        )
        # LambdaInvoker might have already logged, but we keep this for gateway context
        return JSONResponse(status_code=502, content={"message": "Bad Gateway"})
    except ContainerStartError as e:
        return JSONResponse(status_code=503, content={"message": str(e)})
    except LambdaExecutionError as e:
        return JSONResponse(status_code=502, content={"message": str(e)})


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)
