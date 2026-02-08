"""
Where: services/gateway/lifecycle.py
What: Gateway startup/shutdown orchestration for shared resources.
Why: Keep main.py focused on app assembly while preserving lifecycle behavior.
"""

import asyncio
import logging
from contextlib import asynccontextmanager
from typing import AsyncIterator, Optional

from fastapi import FastAPI

from services.common.core.http_client import HttpClientFactory

from .config import GatewayConfig
from .core.event_builder import V1ProxyEventBuilder
from .models.function import FunctionEntity
from .services.config_reloader import init_reloader, start_reloader, stop_reloader
from .services.function_registry import FunctionRegistry
from .services.janitor import HeartbeatJanitor
from .services.lambda_invoker import LambdaInvoker
from .services.pool_manager import PoolManager
from .services.processor import GatewayRequestProcessor
from .services.route_matcher import RouteMatcher
from .services.scheduler import SchedulerService

logger = logging.getLogger("gateway.main")


@asynccontextmanager
async def manage_lifespan(app: FastAPI, gateway_config: GatewayConfig) -> AsyncIterator[None]:
    """Manage application lifecycle."""
    factory = HttpClientFactory(gateway_config)
    factory.configure_global_settings()
    client = factory.create_async_client(timeout=gateway_config.LAMBDA_INVOKE_TIMEOUT)

    channel = None
    janitor: Optional[HeartbeatJanitor] = None
    scheduler: Optional[SchedulerService] = None
    pool_manager: Optional[PoolManager] = None
    reloader = None

    try:
        function_registry = FunctionRegistry()
        route_matcher = RouteMatcher(function_registry)

        function_registry.load_functions_config()
        route_matcher.load_routing_config()

        def config_loader(function_name: str) -> Optional[FunctionEntity]:
            """Load scaling config for a function."""
            return function_registry.get_function_config(function_name)

        logger.info(
            "Initializing Gateway with Agent gRPC Backend: %s",
            gateway_config.AGENT_GRPC_ADDRESS,
        )

        from .pb import agent_pb2_grpc
        from .services.agent_invoke import AgentInvokeClient
        from .services.grpc_channel import create_agent_channel
        from .services.grpc_provision import GrpcProvisionClient

        channel = create_agent_channel(gateway_config.AGENT_GRPC_ADDRESS, gateway_config)
        agent_stub = agent_pb2_grpc.AgentServiceStub(channel)

        grpc_provision_client = GrpcProvisionClient(
            agent_stub,
            function_registry,
            skip_readiness_check=gateway_config.AGENT_INVOKE_PROXY,
            owner_id=gateway_config.GATEWAY_OWNER_ID,
        )

        pool_manager = PoolManager(
            provision_client=grpc_provision_client,
            config_loader=config_loader,
            pause_enabled=gateway_config.ENABLE_CONTAINER_PAUSE,
            pause_idle_seconds=gateway_config.PAUSE_IDLE_SECONDS,
        )
        if gateway_config.ENABLE_CONTAINER_PAUSE:
            logger.info(
                "Container pause enabled (idle_delay=%ss)",
                gateway_config.PAUSE_IDLE_SECONDS,
            )

        await pool_manager.cleanup_all_containers()

        agent_invoker = None
        if gateway_config.AGENT_INVOKE_PROXY:
            agent_invoker = AgentInvokeClient(agent_stub, owner_id=gateway_config.GATEWAY_OWNER_ID)
            logger.info("Gateway invoke proxy enabled (L7 via Agent).")

        janitor = HeartbeatJanitor(
            pool_manager,
            manager_client=None,
            interval=gateway_config.HEARTBEAT_INTERVAL,
            idle_timeout=gateway_config.GATEWAY_IDLE_TIMEOUT_SECONDS,
        )
        await janitor.start()

        lambda_invoker = LambdaInvoker(
            client=client,
            registry=function_registry,
            config=gateway_config,
            backend=pool_manager,  # ty: ignore[invalid-argument-type]  # PoolManager satisfies InvocationBackend protocol
            agent_invoker=agent_invoker,
        )

        scheduler = SchedulerService(lambda_invoker)
        await scheduler.start()
        scheduler.load_schedules(function_registry._registry)

        def reload_functions_and_schedules() -> None:
            function_registry.reload()
            scheduler.load_schedules(function_registry._registry)

        reloader = init_reloader(
            functions_callback=reload_functions_and_schedules,
            routing_callback=route_matcher.reload,
        )
        start_reloader()

        app.state.config_reloader = reloader
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
    finally:
        if reloader:
            stop_reloader()

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
            except Exception as exc:
                logger.warning("Failed to close gRPC channel: %s", exc)

        logger.info("Gateway shutting down, closing http client.")
        await client.aclose()
