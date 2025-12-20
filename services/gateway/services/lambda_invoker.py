import logging
from typing import Optional
import httpx
import os

from .function_registry import get_function_config
from ..client import get_lambda_host
from ..core.exceptions import FunctionNotFoundError, ContainerStartError, LambdaExecutionError

logger = logging.getLogger("gateway.lambda_invoker")


async def invoke_function(function_name: str, payload: bytes, timeout: int = 300) -> httpx.Response:
    """
    Invokes Lambda function (Async).
    """
    # config check
    func_config = get_function_config(function_name)
    if func_config is None:
        raise FunctionNotFoundError(function_name)

    # Prepare env
    env = func_config.get("environment", {}).copy()

    # Resolve Gateway URL (Simple approach for now)
    gateway_internal_url = os.getenv("GATEWAY_INTERNAL_URL", "http://gateway:8080")
    env["GATEWAY_INTERNAL_URL"] = gateway_internal_url

    # Ensure container (via Manager)
    try:
        host = await get_lambda_host(
            function_name=function_name,
            image=func_config.get("image"),
            env=env,
        )
    except Exception as e:
        raise ContainerStartError(function_name, e) from e

    # POST to Lambda RIE
    rie_url = f"http://{host}:8080/2015-03-31/functions/function/invocations"
    logger.info(f"Invoking {function_name} at {rie_url}")

    try:
        async with httpx.AsyncClient() as client:
            response = await client.post(
                rie_url,
                content=payload,
                headers={"Content-Type": "application/json"},
                timeout=timeout,
            )
        return response
    except httpx.RequestError as e:
        raise LambdaExecutionError(function_name, e) from e


def get_function_config_or_none(function_name: str) -> Optional[dict]:
    return get_function_config(function_name)
