from fastapi import FastAPI, HTTPException, Request
import logging
from contextlib import asynccontextmanager
import asyncio

from .service import ContainerManager
import docker.errors
from services.common.core.request_context import (
    set_trace_id,
)
from .config import config

from .core.logging_config import setup_logging
from services.common.models.internal import ContainerEnsureRequest, ContainerInfoResponse

# Logger setup
setup_logging()
logger = logging.getLogger("manager.main")

manager = ContainerManager()
# レベル設定などはYAML側で行う


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup Logic: Initialize HTTP client and Sync with Docker
    await manager.startup()
    try:
        await manager.sync_with_docker()
    except Exception as e:
        logger.error(f"Failed to sync containers on startup: {e}", exc_info=True)

    # Start background scheduler for idle cleanup
    from apscheduler.schedulers.asyncio import AsyncIOScheduler

    scheduler = AsyncIOScheduler()
    scheduler.add_job(
        manager.stop_idle_containers,
        "interval",
        minutes=1,
        id="idle_cleanup",
        args=[config.IDLE_TIMEOUT_MINUTES * 60],
    )
    scheduler.start()
    logger.info(f"Idle cleanup scheduler started (timeout: {config.IDLE_TIMEOUT_MINUTES}m)")

    yield
    # Shutdown logic
    await manager.shutdown()
    scheduler.shutdown()


app = FastAPI(lifespan=lifespan)


# ミドルウェアの登録（デコレーター方式）
# ミドルウェアの登録（デコレーター方式）
@app.middleware("http")
async def trace_propagation_middleware(request: Request, call_next):
    """Trace ID ヘッダーをキャプチャし、ContextVar に設定するミドルウェア"""
    from services.common.core.trace import TraceId

    # 1. Trace ID の取得または生成
    trace_id_str = request.headers.get("X-Amzn-Trace-Id")
    if trace_id_str:
        try:
            set_trace_id(trace_id_str)
        except Exception as e:
            logger.warning(
                f"Failed to parse incoming X-Amzn-Trace-Id: '{trace_id_str}', error: {e}"
            )
            trace = TraceId.generate()
            trace_id_str = str(trace)
            set_trace_id(trace_id_str)
    else:
        # Trace ID がない場合は新規生成
        trace = TraceId.generate()
        trace_id_str = str(trace)
        set_trace_id(trace_id_str)

    # ログ出力 (trace_id を明示的に渡す)
    logger.info(f"Request: {request.method} {request.url.path}", extra={"trace_id": trace_id_str})

    try:
        response = await call_next(request)
        # レスポンスヘッダーに付与
        response.headers["X-Amzn-Trace-Id"] = trace_id_str

        logger.info(f"Response: {response.status_code}", extra={"trace_id": trace_id_str})
        return response
    finally:
        # クリーンアップ
        from services.common.core.request_context import clear_trace_id

        clear_trace_id()


@app.post("/containers/ensure", response_model=ContainerInfoResponse)
async def ensure_container(req: ContainerEnsureRequest, request: Request):
    """
    Ensures a container with the given function name is running.
    """

    try:
        host = await manager.ensure_container_running(req.function_name, req.image, req.env)
        return ContainerInfoResponse(host=host, port=config.LAMBDA_PORT)
    except docker.errors.ImageNotFound as e:
        logger.error(f"Image not found: {e.explanation}")
        raise HTTPException(status_code=404, detail=f"Lambda image not found: {e.explanation}")
    except asyncio.TimeoutError as e:
        logger.error(f"Container startup timeout: {e}")
        raise HTTPException(status_code=408, detail="Container startup timeout")
    except docker.errors.ContainerError as e:
        logger.error(f"Container error: {e}")
        raise HTTPException(status_code=409, detail=f"Container conflict: {e}")
    except docker.errors.APIError as e:
        logger.error(f"Docker API error: {e.explanation}")
        raise HTTPException(status_code=400, detail=f"Docker API error: {e.explanation}")
    except Exception as e:
        logger.error(f"Error ensuring container: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail="Internal server error managing containers")
