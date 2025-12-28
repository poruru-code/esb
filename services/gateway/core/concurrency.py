import asyncio
from collections import deque
from typing import Dict, Optional, TYPE_CHECKING
from services.gateway.core.exceptions import ResourceExhaustedError

if TYPE_CHECKING:
    from services.gateway.services.function_registry import FunctionRegistry


class FunctionThrottle:
    """
    Per-function rate control class.
    Uses asyncio.Condition and deque to guarantee FIFO.
    """

    def __init__(self, limit: int, default_timeout: float = 10.0):
        self.limit = limit
        self.default_timeout = default_timeout
        self.current = 0
        self.condition = asyncio.Condition()
        self.waiters: deque[asyncio.Future] = deque()

    async def acquire(self, timeout: Optional[float] = None) -> None:
        if timeout is None:
            timeout = self.default_timeout

        async with self.condition:
            if self.current < self.limit:
                self.current += 1
                return
            waiter = asyncio.get_running_loop().create_future()
            self.waiters.append(waiter)

        try:
            await asyncio.wait_for(waiter, timeout)
            return
        except asyncio.TimeoutError:
            async with self.condition:
                if waiter in self.waiters:
                    self.waiters.remove(waiter)
            raise ResourceExhaustedError("Request timed out in queue")
        except BaseException:
            async with self.condition:
                if waiter in self.waiters:
                    self.waiters.remove(waiter)
            raise

    async def release(self) -> None:
        async with self.condition:
            if self.waiters:
                next_waiter = self.waiters.popleft()
                if not next_waiter.done():
                    next_waiter.set_result(None)
            else:
                if self.current > 0:
                    self.current -= 1
            self.condition.notify()

    async def __aenter__(self):
        await self.acquire()
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        await self.release()


class ConcurrencyManager:
    def __init__(
        self,
        default_limit: int,
        default_timeout: int,
        function_registry: Optional["FunctionRegistry"] = None,
    ):
        self._default_limit = default_limit
        self._default_timeout = default_timeout
        self._function_registry = function_registry
        self._throttles: Dict[str, FunctionThrottle] = {}

    def get_throttle(self, function_name: str) -> FunctionThrottle:
        if function_name not in self._throttles:
            limit = self._default_limit
            if self._function_registry:
                func_config = self._function_registry.get_function_config(function_name)
                if func_config:
                    # Look for scaling.max_capacity first (Padma standard)
                    if "scaling" in func_config:
                        limit = func_config["scaling"].get("max_capacity", limit)
                    # Support ReservedConcurrentExecutions (SAM/Lambda standard)
                    elif "ReservedConcurrentExecutions" in func_config:
                        limit = func_config["ReservedConcurrentExecutions"]

            self._throttles[function_name] = FunctionThrottle(limit, self._default_timeout)
        return self._throttles[function_name]

    @property
    def default_timeout(self) -> int:
        return self._default_timeout
