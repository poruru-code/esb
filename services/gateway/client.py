from typing import Optional, Dict
import httpx
import os
import logging

logger = logging.getLogger("gateway.client")

MANAGER_URL = os.getenv("MANAGER_URL", "http://manager:8081")


async def get_lambda_host(
    function_name: str, image: Optional[str] = None, env: Optional[Dict[str, str]] = None
) -> str:
    """
    Calls Manager Service to ensure container is running and get its host/IP.
    """
    url = f"{MANAGER_URL}/containers/ensure"
    payload = {"function_name": function_name, "image": image, "env": env or {}}
    try:
        async with httpx.AsyncClient() as client:
            resp = await client.post(
                url,
                json=payload,
                timeout=30.0,  # Cold start might take time
            )
            resp.raise_for_status()
            data = resp.json()
            return data["host"]
    except httpx.RequestError as e:
        logger.error(f"Failed to connect to Manager: {e}")
        raise
    except httpx.HTTPStatusError as e:
        logger.error(f"Manager returned error: {e.response.text}")
        raise
