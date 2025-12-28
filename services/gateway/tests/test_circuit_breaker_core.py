import asyncio
import pytest
from services.gateway.core.circuit_breaker import CircuitBreaker, CircuitBreakerOpenError


class TestCircuitBreaker:
    @pytest.mark.asyncio
    async def test_closed_state_success(self):
        """Return success in normal (CLOSED) state."""
        breaker = CircuitBreaker(failure_threshold=2)

        async def mock_func():
            return "success"

        result = await breaker.call(mock_func)
        assert result == "success"
        assert breaker.state == "CLOSED"
        assert breaker.failures == 0

    @pytest.mark.asyncio
    async def test_to_open_state_on_failures(self):
        """Transition to OPEN when failures exceed threshold."""
        breaker = CircuitBreaker(failure_threshold=2, recovery_timeout=0.1)

        async def failing_func():
            raise ValueError("boom")

        # First failure.
        with pytest.raises(ValueError):
            await breaker.call(failing_func)
        assert breaker.state == "CLOSED"
        assert breaker.failures == 1

        # Second failure -> OPEN.
        with pytest.raises(ValueError):
            await breaker.call(failing_func)
        assert breaker.state == "OPEN"
        assert breaker.failures == 2

        # In OPEN, function is not called and CircuitBreakerOpenError is raised immediately.
        with pytest.raises(CircuitBreakerOpenError):
            await breaker.call(failing_func)

    @pytest.mark.asyncio
    async def test_recovery_to_half_open_and_closed(self):
        """Return to CLOSED after recovery timeout and success."""
        breaker = CircuitBreaker(failure_threshold=1, recovery_timeout=0.1)

        async def failing_func():
            raise ValueError("boom")

        async def success_func():
            return "ok"

        # Open the circuit.
        with pytest.raises(ValueError):
            await breaker.call(failing_func)
        assert breaker.state == "OPEN"

        # Wait for timeout.
        await asyncio.sleep(0.15)

        # Next call transitions to HALF_OPEN and success returns to CLOSED.
        result = await breaker.call(success_func)
        assert result == "ok"
        assert breaker.state == "CLOSED"
        assert breaker.failures == 0

    @pytest.mark.asyncio
    async def test_half_open_to_open_on_failure(self):
        """Return to OPEN when failing in HALF_OPEN."""
        breaker = CircuitBreaker(failure_threshold=1, recovery_timeout=0.1)

        async def failing_func():
            raise ValueError("boom")

        # Open the circuit.
        with pytest.raises(ValueError):
            await breaker.call(failing_func)
        assert breaker.state == "OPEN"

        # Wait for timeout.
        await asyncio.sleep(0.15)

        # Retry in HALF_OPEN fails.
        with pytest.raises(ValueError):
            await breaker.call(failing_func)

        assert breaker.state == "OPEN"
        # Failure in HALF_OPEN should return to OPEN immediately.
