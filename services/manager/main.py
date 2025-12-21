from fastapi import FastAPI, HTTPException, Request
from pydantic import BaseModel
import logging
from contextlib import asynccontextmanager
from typing import Optional, Dict
import asyncio

from .service import ContainerManager
import docker.errors
from services.common.core.request_context import set_request_id, clear_request_id, get_request_id
from .config import config

from .core.logging_config import setup_logging

# Logger setup
setup_logging()
logger = logging.getLogger("manager.main")
# レベル設定などはYAML側で行う


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup Logic: Sync with Docker instead of killing all containers
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
    scheduler.shutdown()


app = FastAPI(lifespan=lifespan)


# ミドルウェアの登録（デコレーター方式）
@app.middleware("http")
async def request_id_middleware(request: Request, call_next):
    """X-Request-Id ヘッダーを取得または生成し、ContextVar に設定するミドルウェア"""
    # X-Request-Id ヘッダーから取得、なければ生成
    request_id = request.headers.get("X-Request-Id")
    request_id = set_request_id(request_id)

    # ログ出力（request_id は CustomJsonFormatter で自動付与される）
    logger.info(f"Request: {request.method} {request.url.path}")

    try:
        response = await call_next(request)
        # レスポンスヘッダーにも付与
        response.headers["X-Request-Id"] = request_id
        logger.info(f"Response: {response.status_code}")
        return response
    finally:
        # クリーンアップ
        clear_request_id()


manager = ContainerManager()


class EnsureRequest(BaseModel):
    function_name: str
    image: Optional[str] = None
    env: Optional[Dict[str, str]] = {}


@app.post("/containers/ensure")
async def ensure_container(req: EnsureRequest, request: Request):
    """
    Ensures a container with the given function name is running.
    """
    # request_id は ContextVar から取得されるため、明示的に渡す必要はないが念のため
    # request_id は ContextVar から取得されるため、明示的に渡す必要はないが念のため
    get_request_id()

    try:
        host = await manager.ensure_container_running(req.function_name, req.image, req.env)
        return {"host": host, "port": config.LAMBDA_PORT}
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
