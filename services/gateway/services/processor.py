"""
Gateway Request Processor - Service Layer

Standardizes the flow: InputContext -> Event -> InvocationResult.
"""

import json
import logging

from services.gateway.core.event_builder import EventBuilder
from services.gateway.models.context import InputContext
from services.gateway.models.result import InvocationResult
from services.gateway.services.lambda_invoker import LambdaInvoker

logger = logging.getLogger("gateway.processor")


class GatewayRequestProcessor:
    """
    Orchestrates the request processing lifecycle.

    Acts as the Service Layer (Application Service) in our Clean Architecture.
    """

    def __init__(self, invoker: LambdaInvoker, event_builder: EventBuilder):
        self.invoker = invoker
        self.event_builder = event_builder

    async def process_request(self, context: InputContext) -> InvocationResult:
        """
        Process a request from InputContext to InvocationResult.
        """
        logger.info(
            f"Processing request for {context.function_name} ({context.method} {context.path})"
        )

        try:
            # 1. Build Event from Context
            event = self.event_builder.build(context)

            # 2. Invoke Lambda
            payload = json.dumps(event).encode("utf-8")
            result = await self.invoker.invoke_function(
                context.function_name, payload, timeout=context.timeout
            )

            return result

        except Exception as e:
            logger.exception(f"Unexpected error in request processor: {e}")
            return InvocationResult(
                success=False, status_code=500, error=f"Internal Processing Error: {str(e)}"
            )
