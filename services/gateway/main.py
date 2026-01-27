"""
Lambda Gateway - API Gateway compatible server

Replicates AWS API Gateway and Lambda Authorizer behavior and forwards
requests to Lambda RIE containers based on routing.yml.
"""

import asyncio
import logging
from contextlib import asynccontextmanager
from dataclasses import asdict
from datetime import datetime, timezone
from typing import Optional

from fastapi import BackgroundTasks, FastAPI, Header, HTTPException, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse, Response
from starlette.exceptions import HTTPException as StarletteHTTPException

from services.common.core.http_client import HttpClientFactory

from .api.deps import (
    FunctionRegistryDep,
    InputContextDep,
    LambdaInvokerDep,
    PoolManagerDep,
    ProcessorDep,
    UserIdDep,
)
from .config import config
from .core.event_builder import V1ProxyEventBuilder
from .core.exceptions import (
    ContainerStartError,
    FunctionNotFoundError,
    LambdaExecutionError,
    ResourceExhaustedError,
    global_exception_handler,
    http_exception_handler,
    validation_exception_handler,
)
from .core.logging_config import setup_logging
from .core.security import create_access_token
from .core.utils import parse_lambda_response
from .models import AuthenticationResult, AuthRequest, AuthResponse
from .models.function import FunctionEntity

# Services Imports
from .services.function_registry import FunctionRegistry
from .services.janitor import HeartbeatJanitor
from .services.lambda_invoker import LambdaInvoker
from .services.pool_manager import PoolManager
from .services.processor import GatewayRequestProcessor
from .services.route_matcher import RouteMatcher
from .services.scheduler import SchedulerService

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
    def config_loader(function_name: str) -> Optional[FunctionEntity]:
        """Load scaling config for a function"""
        return function_registry.get_function_config(function_name)

    logger.info(f"Initializing Gateway with Go Agent gRPC Backend: {config.AGENT_GRPC_ADDRESS}")

    # New ARCH: PoolManager -> GrpcProvisionClient -> Agent
    from .pb import agent_pb2_grpc
    from .services.agent_invoke import AgentInvokeClient
    from .services.grpc_channel import create_agent_channel
    from .services.grpc_provision import GrpcProvisionClient

    # shared channel for lifecycle and info
    channel = create_agent_channel(config.AGENT_GRPC_ADDRESS, config)
    agent_stub = agent_pb2_grpc.AgentServiceStub(channel)

    grpc_provision_client = GrpcProvisionClient(
        agent_stub,
        function_registry,
        skip_readiness_check=config.AGENT_INVOKE_PROXY,
        owner_id=config.GATEWAY_OWNER_ID,
    )

    pool_manager = PoolManager(
        provision_client=grpc_provision_client,
        config_loader=config_loader,
        pause_enabled=config.ENABLE_CONTAINER_PAUSE,
        pause_idle_seconds=config.PAUSE_IDLE_SECONDS,
    )
    if config.ENABLE_CONTAINER_PAUSE:
        logger.info("Container pause enabled (idle_delay=%ss)", config.PAUSE_IDLE_SECONDS)

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
    agent_invoker = None
    if config.AGENT_INVOKE_PROXY:
        agent_invoker = AgentInvokeClient(agent_stub, owner_id=config.GATEWAY_OWNER_ID)
        logger.info("Gateway invoke proxy enabled (L7 via Agent).")

    lambda_invoker = LambdaInvoker(
        client=client,
        registry=function_registry,
        config=config,
        backend=invocation_backend,  # ty: ignore[invalid-argument-type]  # PoolManager implements InvocationBackend protocol
        agent_invoker=agent_invoker,
    )

    # Initialize Scheduler
    scheduler = SchedulerService(lambda_invoker)
    await scheduler.start()

    # Load schedules from registry
    # Note: function_registry.load_functions_config() was already called above
    scheduler.load_schedules(function_registry._registry)

    # Store in app.state for DI
    app.state.http_client = client
    app.state.function_registry = function_registry
    app.state.route_matcher = route_matcher
    app.state.lambda_invoker = lambda_invoker
    app.state.event_builder = V1ProxyEventBuilder()
    app.state.processor = GatewayRequestProcessor(lambda_invoker, app.state.event_builder)
    app.state.pool_manager = pool_manager
    app.state.scheduler = scheduler

    logger.info("Gateway initialized with shared resources.")

    yield

    # Cleanup
    if janitor:
        await janitor.stop()

    if scheduler:
        await scheduler.stop()

    if pool_manager:
        await pool_manager.shutdown_all()

    if channel:
        try:
            close_result = channel.close()
            if asyncio.iscoroutine(close_result):
                await close_result
        except Exception as e:
            logger.warning("Failed to close gRPC channel: %s", e)

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

    from services.common.core.request_context import (
        clear_trace_id,
        generate_request_id,
        set_trace_id,
    )
    from services.common.core.trace import TraceId

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
                "query_params": str(request.query_params),
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
app.add_exception_handler(Exception, global_exception_handler)  # ty: ignore[invalid-argument-type]  # Starlette type stubs incomplete
app.add_exception_handler(StarletteHTTPException, http_exception_handler)  # ty: ignore[invalid-argument-type]  # Starlette type stubs incomplete
app.add_exception_handler(RequestValidationError, validation_exception_handler)  # ty: ignore[invalid-argument-type]  # Starlette type stubs incomplete


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


async def health_check():
    """Health check implementation."""
    return {"status": "healthy", "timestamp": datetime.now(timezone.utc).isoformat()}


@app.get("/health")
async def health_check_endpoint():
    """Health check endpoint."""
    return await health_check()


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
    not_implemented_errors = 0

    for container, result in zip(containers, results, strict=True):
        if isinstance(result, Exception):
            err_msg = str(result)
            logger.error(f"Failed to fetch metrics for container {container.id}: {err_msg}")

            # Check for "not implemented" error from Agent (Docker runtime)
            if "metrics not implemented" in err_msg.lower():
                not_implemented_errors += 1

            failures += 1
            metrics_list.append(
                {
                    "container_id": container.id,
                    "container_name": container.name,
                    "error": err_msg,
                }
            )
            continue
        metrics_list.append(asdict(result))

    if failures == len(containers):
        # If all failed and at least one was "not implemented", return 501
        # (Assuming consistent runtime across containers)
        if not_implemented_errors > 0:
            raise HTTPException(
                status_code=501,
                detail=(
                    "Container metrics are not implemented for the current runtime (e.g. Docker)"
                ),
            )

        raise HTTPException(
            status_code=503,
            detail="Container metrics are unavailable from Agent runtime",
        )

    return {"containers": metrics_list, "failures": failures}


@app.get("/metrics/pools")
async def list_pool_metrics(user_id: UserIdDep, pool_manager: PoolManagerDep):
    """Gateway のプール統計を返す (runtime 非依存)."""
    return {
        "pools": await pool_manager.get_pool_stats(),
        "collected_at": datetime.now(timezone.utc).isoformat(),
    }


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
            background_tasks.add_task(  # ty: ignore[invalid-argument-type]  # FastAPI BackgroundTasks type stubs
                invoker.invoke_function,
                function_name,
                body,
                timeout=config.LAMBDA_INVOKE_TIMEOUT,
            )
            return Response(status_code=202, content=b"", media_type="application/json")

        # Sync invoke: wait for the result.
        result = await invoker.invoke_function(
            function_name, body, timeout=config.LAMBDA_INVOKE_TIMEOUT
        )

        if not result.success:
            return JSONResponse(status_code=result.status_code, content={"message": result.error})

        # Pass through the RIE response to the client (boto3).
        return Response(
            content=result.payload,
            status_code=result.status_code,
            headers=result.headers,
            media_type="application/json",
        )
    except ContainerStartError as e:
        return JSONResponse(status_code=503, content={"message": str(e)})
    except LambdaExecutionError as e:
        return JSONResponse(status_code=502, content={"message": str(e)})


@app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "DELETE"])
async def gateway_handler(
    context: InputContextDep,
    processor: ProcessorDep,
):
    """
    Catch-all route: process request via GatewayRequestProcessor.
    """
    result = await processor.process_request(context)

    if not result.success:
        return JSONResponse(
            status_code=result.status_code,
            content={"message": result.error},
        )

    # Transform response.
    parsed_result = parse_lambda_response(result)

    if "raw_content" in parsed_result:
        response = Response(
            content=parsed_result["raw_content"],
            status_code=parsed_result["status_code"],
            # Initial headers (single values)
            headers=parsed_result["headers"],
        )
    else:
        response = JSONResponse(
            status_code=parsed_result["status_code"],
            content=parsed_result["content"],
            headers=parsed_result["headers"],
        )

    # Apply multi-value headers (e.g. Set-Cookie)
    # Note: parsed_result["multi_headers"] should contain lists of strings
    if "multi_headers" in parsed_result:
        for key, values in parsed_result["multi_headers"].items():
            # multi_headers supersedes single headers.
            # Remove any potentially partial value set by 'headers' init.
            if key in response.headers:
                del response.headers[key]
            for v in values:
                response.headers.append(key, v)

    return response


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=8000)
