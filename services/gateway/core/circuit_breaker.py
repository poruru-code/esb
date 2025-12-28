import time
import logging
from typing import Callable, Any

logger = logging.getLogger("gateway.circuit_breaker")


class CircuitBreakerOpenError(Exception):
    """Raised when the circuit is open (OPEN)."""

    pass


class CircuitBreaker:
    """
    Core circuit breaker logic.
    Monitors failures for specific external services (containers) and
    temporarily blocks requests when a threshold is exceeded.
    """

    def __init__(self, failure_threshold: int = 5, recovery_timeout: int = 30):
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self.failures = 0
        self.last_failure_time: float = 0
        self.state = "CLOSED"  # CLOSED, OPEN, HALF_OPEN

    async def call(self, func: Callable, *args, **kwargs) -> Any:
        """
        Execute the target function and open/close the circuit as needed.
        """
        if self.state == "OPEN":
            # Check if timeout has elapsed.
            if time.time() - self.last_failure_time > self.recovery_timeout:
                self.state = "HALF_OPEN"
                logger.info("Circuit Breaker transitions to HALF_OPEN")
            else:
                raise CircuitBreakerOpenError(f"Circuit is open (failures: {self.failures})")

        try:
            result = await func(*args, **kwargs)
            # On success.
            if self.state == "HALF_OPEN":
                self.reset()
                logger.info("Circuit Breaker recovered (back to CLOSED)")
            return result
        except Exception as e:
            self.failures += 1
            self.last_failure_time = time.time()

            # Failure in HALF_OPEN immediately returns to OPEN.
            if self.failures >= self.failure_threshold or self.state == "HALF_OPEN":
                self.state = "OPEN"
                logger.warning(f"Circuit Breaker opened due to error: {e}")

            raise e

    def reset(self):
        """Reset state to CLOSED."""
        self.failures = 0
        self.state = "CLOSED"
        self.last_failure_time = 0
