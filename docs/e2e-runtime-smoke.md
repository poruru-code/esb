<!--
Where: docs/e2e-runtime-smoke.md
What: System-level smoke test contract and matrix policy.
Why: Define stable E2E verification semantics across runtimes.
-->
# E2E Runtime Smoke Design

## Summary
Runtime smoke tests are unified across languages and executed via one REST contract.
This document defines **what is verified**. Runner internals are documented in
`e2e/runner/README.md`.

- Endpoint: `POST /api/connectivity/{runtime}`
- Common actions: `echo`, `dynamodb_put`, `s3_put`, `chain_invoke`
- Goal: identical pass/fail semantics across runtime targets

## Test Layers
| Layer | Purpose | Primary location |
| --- | --- | --- |
| Smoke | Cross-runtime connectivity contract | `e2e/scenarios/smoke/` |
| Standard | Feature-level end-to-end coverage | `e2e/scenarios/standard/`, `e2e/scenarios/autoscaling/` |
| Runtime | Language/runtime-specific non-smoke checks | `e2e/scenarios/runtime/` |

## REST Contract
Endpoint: `/api/connectivity/{runtime}`

Actions:
- `echo`: `{ "action": "echo", "message": "..." }`
- `dynamodb_put`: `{ "action": "dynamodb_put", "key": "...", "value": "..." }`
- `s3_put`: `{ "action": "s3_put", "bucket": "...", "key": "...", "content": "..." }`
- `chain_invoke`: `{ "action": "chain_invoke", "target": "lambda-echo" }`

Common response:
- `{"success": true, "action": "..."}`
- `chain_invoke` additionally returns `child` payload

## Matrix Policy (Current)
`e2e/environments/test_matrix.yaml` currently defines these runtime matrices:

- `e2e-docker`: `smoke`, `standard`, `runtime`
- `e2e-containerd`: `smoke`, `standard`
- firecracker profile exists as commented plan and is not enabled by default matrix

This keeps default CI/runtime coverage stable while preserving future runtime expansion.

## Execution Path and Contract
Runner behavior is intentionally centralized:

```text
e2e/run_tests.py -> e2e.runner.*
```

Contract visible to CLI users:
- non-zero exit when any environment fails
- failed environment tail logs from `e2e/.parallel-<env>.log`
- `--profile` for targeted execution
- `--build-only` / `--test-only` mutual exclusion

## Directory Layout
```text
e2e/
  run_tests.py
  environments/test_matrix.yaml
  runner/
    README.md
  scenarios/
    smoke/
    standard/
    autoscaling/
    runtime/
```

## Current Runtime Scope
Current smoke runtime targets are `python` and `java` only.
Runtime-specific non-smoke cases are also maintained for these two runtimes.

---

## Implementation references
- `e2e/run_tests.py`
- `e2e/environments/test_matrix.yaml`
- `e2e/scenarios/smoke/test_connectivity.py`
- `e2e/scenarios/smoke/test_smoke.py`
- `e2e/runner/README.md`
