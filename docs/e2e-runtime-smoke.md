# E2E Runtime Smoke Design

## Summary
Runtime smoke tests are unified across languages and exercised via a single REST contract:

- `POST /api/connectivity/{runtime}`
- Common actions: `echo`, `dynamodb_put`, `s3_put`, `chain_invoke`

This keeps smoke verification identical across runtimes while allowing language-specific
observability tests to live elsewhere. Compatibility routes are not used.

Smoke tests live under `e2e/scenarios/smoke/` and use shared helpers there to guarantee
identical behavior across runtimes.

## REST Contract
**Endpoint**: `/api/connectivity/{runtime}`

**Actions**:
- `echo`: `{ "action": "echo", "message": "..." }`
- `dynamodb_put`: `{ "action": "dynamodb_put", "key": "...", "value": "..." }`
- `s3_put`: `{ "action": "s3_put", "bucket": "...", "key": "...", "content": "..." }`
- `chain_invoke`: `{ "action": "chain_invoke", "target": "lambda-echo" }`

**Response** (common):
- `{"success": true, "action": "..."}`
- `chain_invoke` includes `child` payload

## Directory Layout
```
e2e/scenarios/
  smoke/
    runtime_matrix.py
    runtime_helpers.py
    test_connectivity.py
    test_smoke.py
  runtime/
    python/
      test_echo.py
      test_observability.py
    java/
      test_echo.py
      test_observability.py
```

## test_matrix.yaml Policy
- `smoke`: runs on all envs (docker + containerd)
- `runtime`: docker only (language-specific non-smoke tests)

## Node Runtime Example (Design Only)
### Fixtures
```
e2e/fixtures/functions/node/
  connectivity/
    index.js
    package.json
  echo/
    index.js
    package.json
```

### Template example
```
NodeConnectivityFunction:
  Type: AWS::Serverless::Function
  Properties:
    FunctionName: lambda-connectivity-node
    Runtime: nodejs20.x
    Handler: index.handler
    CodeUri: functions/node/connectivity/
    Events:
      NodeConnectivityApi:
        Type: Api
        Properties:
          Path: /api/connectivity/node
          Method: post
```

### Runtime tests
```
e2e/scenarios/runtime/node/
  test_echo.py
  test_observability.py
```

Node should implement the same action contract as Python/Java in its connectivity handler.
Add `{"id": "node", "path": "/api/connectivity/node"}` to
`e2e/scenarios/smoke/runtime_matrix.py` and ensure `e2e/scenarios/smoke/test_smoke.py`
covers the node runtime.
