<!--
Where: docs/java-maven-proxy-contract.md
What: Canonical contract for Java Maven proxy behavior in warmup/deploy.
Why: Prevent plan drift by defining hard invariants and forbidden paths.
-->
# Java Maven Proxy Contract

## Scope
This contract applies to both paths:
- E2E Java warmup (`e2e/runner/warmup.py`)
- Deploy-time Java runtime jar build (`cli/internal/infra/templategen/stage_java_runtime.go`)
- Maven settings/proxy rendering (`cli/internal/infra/templategen/stage_java_maven.go`)

## Invariants
1. A temporary Maven `settings.xml` is generated for every run.
2. Maven always runs with `mvn -s /tmp/m2/settings.xml ...`.
3. If proxy env is configured (`HTTP_PROXY`/`HTTPS_PROXY`, including lowercase aliases),
   generated `settings.xml` must include `<proxy>` definitions.
4. `NO_PROXY`/`no_proxy` is converted into Maven `nonProxyHosts` deterministically.
5. Java build container must not rely on `HTTP_PROXY`/`HTTPS_PROXY` env values at runtime;
   proxy source of truth is `settings.xml`.
6. Java build image is fixed to this digest only:
   `public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7`
7. Maven dependency resolution runs with `-Dmaven.artifact.threads=1` for reproducibility.
8. Maven local repository uses project-scope cache: `.esb/cache/m2/repository`.

## Forbidden
- Host `~/.m2/settings.xml` dependency.
- Conditional fallback such as `if [ -f /tmp/m2/settings.xml ] ... else mvn ...`.
- `build-java21:latest`.
- Passing runtime proxy env (`HTTP_PROXY`/`HTTPS_PROXY`) into Java build container as behavior source.

## Proxy Parsing Rules
1. Proxy URL must include scheme and host.
2. Allowed schemes: `http`, `https`.
3. Path/query/fragment are not allowed.
4. Port must be valid (`1-65535`) when specified.
5. HTTPS proxy falls back to HTTP proxy if HTTPS proxy is not specified.

## NO_PROXY to nonProxyHosts Rules
1. Split by `,` and `;`.
2. Trim and deduplicate while preserving order.
3. Convert `.example.com` to `*.example.com`.
4. Convert `host:port` to `host`.
5. Convert `[::1]:5010` to `::1`.
6. Join using `|`.

## Drift Guardrails
- Shared test vectors: `runtime-hooks/java/testdata/maven_proxy_cases.json`
- Static contract check: `tools/ci/check_java_proxy_contract.sh`
- Proxy proof runner: `tools/e2e_proxy/run_with_tinyproxy.py`

## Verification Commands
```bash
bash tools/ci/check_java_proxy_contract.sh
cd cli && go test ./internal/infra/templategen -count=1
uv run pytest -q e2e/runner/tests
```

---

## Implementation references
- `cli/internal/infra/templategen/stage_java_runtime.go`
- `cli/internal/infra/templategen/stage_java_maven.go`
- `cli/internal/infra/templategen/stage_java_env_test.go`
- `e2e/runner/warmup.py`
- `runtime-hooks/java/README.md`
