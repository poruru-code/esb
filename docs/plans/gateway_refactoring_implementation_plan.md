# Gateway Refactoring Implementation Plan (v2)

This document serves as the **executable specification** for refactoring `services/gateway`.
It is designed to be followed by an independent implementer.

**Guiding Principle**: "Testing First, Strangler Fig Second."
We will build the new structure in parallel with the old one, verifying with new tests before switching traffic.

---

## Phase 0: Test Infrastructure & Critical Fixes

**Rationale**: Fixing the known concurrency bug and establishing a safety net is the prerequisite for any refactoring.

### 0.1 Fix `ContainerPool.acquire` Race Condition
*   **Problem**: Logic releases lock to check `_idle_workers` but implementation might allow other coroutines to steal the worker before the waiter wakes up.
*   **Action**: Create `services/gateway/tests/stress/test_pool_concurrency.py`.
    *   **Test Case**: Spawn 100 concurrent `acquire` requests for a pool with `max_capacity=5`. Verify no `TimeoutError` occurs if tasks complete quickly.
*   **Fix Implementation**: Modify `services/gateway/services/container_pool.py`:
    *   Ensure `_idle_workers` check happens *inside* the `while True` loop immediately after waking up.
    *   Validate strict FIFO ordering if possible (optional but recommended).

### 0.2 Unit Test Harness for `LambdaInvoker`
*   **Action**: Create `services/gateway/tests/unit/test_lambda_invoker_isolation.py`.
*   **Scope**:
    *   Mock `InvocationBackend` (strategy).
    *   Mock `httpx.AsyncClient`.
    *   **Crucial**: Test `invoke_function` behavior when `backend.acquire_worker` raises `Exception`.
    *   **Crucial**: Test Circuit Breaker state transitions (Closed -> Open -> Half-Open).

---

## Phase 1: Domain & Error Handling Foundation

**Rationale**: Defining "What is a Function" and "What is a Result" allows us to break the monolithic logic.

### 1.1 `FunctionEntity` (Pydantic)
*   **File**: `services/gateway/models/function.py`
*   **Spec**:
    ```python
    class FunctionEntity(BaseModel):
        name: str
        image: str
        environment: Dict[str, str] = {}
        scaling: ScalingConfig
        timeout_seconds: int = 300
    ```
*   **Migration**: Update `FunctionRegistry.get_function_config()` to return this model. Update consumers (`LambdaInvoker`, `PoolManager`) to read attributes (`.image`) instead of dict keys (`['image']`).

### 1.2 `InvocationResult` Pattern
*   **File**: `services/gateway/models/result.py`
*   **Spec**:
    ```python
    @dataclass(frozen=True)
    class InvocationResult:
        success: bool
        status_code: int
        payload: Optional[bytes]
        headers: Dict[str, str]
        error: Optional[Exception] = None
        
        @property
        def is_retryable(self) -> bool:
            # Logic for 502/503/ConnectionError
            pass
    ```

### 1.3 `LambdaInvoker` Composed Method Refactor
*   **Target**: `services/gateway/services/lambda_invoker.py`
*   **Refactoring Steps**:
    1.  Keep the public API `invoke_function` stable for now.
    2.  Extract private methods (Green-Refactor loop):
        *   `async def _acquire_worker(self, name: str) -> WorkerInfo`
        *   `async def _execute_request(self, worker: WorkerInfo, payload: bytes) -> httpx.Response`
        *   `def _validate_response(self, response: httpx.Response) -> InvocationResult`
    3.  Update `invoke_function` to choreograph these private methods.

---

## Phase 2: Service Layer & DTO (The "New World")

**Rationale**: Build a parallel universe where `FastAPI` does not exist.

### 2.1 `InputContext` DTO
*   **File**: `services/gateway/models/context.py`
*   **Spec**:
    ```python
    @dataclass
    class InputContext:
        path: str
        method: str
        headers: Dict[str, str]
        query_params: Dict[str, str]
        body: bytes
        user_id: Optional[str]
        request_id: str
    ```

### 2.2 `GatewayRequestProcessor`
*   **File**: `services/gateway/services/processor.py`
*   **Spec**:
    ```python
    class GatewayRequestProcessor:
        def __init__(self, invoker: LambdaInvoker, event_builder: EventBuilder):
            ...
        
        async def process(self, context: InputContext) -> InvocationResult:
            # 1. Build Event (using context)
            # 2. Invoke (using invoker)
            # 3. Return Result
    ```

---

## Phase 3: Strangler Fig Migration (Presentation Layer)

**Rationale**: Move endpoints one by one without a "Big Bang" rewrite.

### 3.1 Create Router Structure
*   `services/gateway/api/routers/lambda_api.py` (Move `invoke_lambda_api`)
*   `services/gateway/api/routers/system.py` (Move `health_check`, `metrics`)

### 3.2 Switch `gateway_handler`
*   **Action**: In `main.py`, keep the handler definition but replace its *body*.
*   **Code Change**:
    ```python
    # main.py
    async def gateway_handler(request: Request, ...):
        # 1. Convert Request -> InputContext
        context = await request_to_context(request)
        # 2. Call Processor
        result = await processor.process(context)
        # 3. specific response mapping
        return map_result_to_response(result)
    ```
*   **Benefit**: If this works for the catch-all route, `main.py` is effectively just a shell (Controller).

---

## Phase 4: Dependency Injection & Cleanup

### 4.1 `setup_dependencies`
*   **File**: `services/gateway/core/factory.py`
*   **Action**: Move the entire `lifespan` initialization logic (creating `PoolManager`, `Janitor`, `Scheduler`) into a function `create_container() -> ServiceContainer`.

### 4.2 Finalize `main.py`
*   Import `create_container`.
*   Connect `lifespan` to use the factory.
*   Verify all old imports are unused and remove them.

---

## Verification Checklist

implementer must ensure:
1.  [ ] `pytest services/gateway/tests/stress/` passes 100 consecutive runs (Flakiness check).
2.  [ ] `LambdaInvoker` has no methods longer than 30 lines.
3.  [ ] `main.py` contains NO business logic (no `event_builder` calls, no `json.dumps`).
