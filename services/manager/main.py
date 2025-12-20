from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from fastapi.concurrency import run_in_threadpool
import logging
import os
from contextlib import asynccontextmanager
from typing import Optional, Dict
from apscheduler.schedulers.background import BackgroundScheduler

from .service import ContainerManager

# Check logger setup
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("manager.main")

IDLE_TIMEOUT_MINUTES = int(os.environ.get("IDLE_TIMEOUT_MINUTES", 5))


@asynccontextmanager
async def lifespan(app: FastAPI):
    # Startup Logic: External reconciliation
    try:
        await run_in_threadpool(manager.prune_managed_containers)
    except Exception as e:
        logger.error(f"Failed to prune containers on startup: {e}")

    # Start background scheduler for idle cleanup
    scheduler = BackgroundScheduler()
    scheduler.add_job(
        lambda: manager.stop_idle_containers(IDLE_TIMEOUT_MINUTES * 60),
        "interval",
        minutes=1,
        id="idle_cleanup",
    )
    scheduler.start()
    logger.info(f"Idle cleanup scheduler started (timeout: {IDLE_TIMEOUT_MINUTES}m)")

    yield
    # Shutdown logic
    scheduler.shutdown()


app = FastAPI(lifespan=lifespan)
manager = ContainerManager()


class EnsureRequest(BaseModel):
    function_name: str
    image: Optional[str] = None
    env: Optional[Dict[str, str]] = {}


@app.post("/containers/ensure")
async def ensure_container(req: EnsureRequest):
    """
    Ensures a container with the given function name is running.
    """
    try:
        # Run blocking docker call in threadpool
        host = await run_in_threadpool(
            manager.ensure_container_running, req.function_name, req.image, req.env
        )
        return {"host": host, "port": 8080}
    except Exception as e:
        logger.error(f"Error ensuring container: {e}", exc_info=True)
        raise HTTPException(status_code=500, detail=str(e))
