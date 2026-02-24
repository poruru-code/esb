<!--
Where: docs/maven-proxy-contract-v2.md
What: Canonical contract for deploy-time Maven proxy behavior.
Why: Avoid Dockerfile-specific workarounds and keep proxy handling deterministic.
-->
# Maven Proxy Contract v2

## Scope

This contract applies to deploy-time function/fixture image builds where Maven may run:

- `artifactctl deploy` image build path (`pkg/deployops`)
- E2E local fixture image build path (`e2e/runner/deploy.py`)

It does not currently apply to `docker compose up` control-plane service builds, because
compose-managed Dockerfiles in `services/**` do not currently run Maven.
A CI static guard enforces this assumption.

## Invariants

1. Dockerfile `RUN mvn ...` lines are not rewritten.
2. Maven proxy behavior is provided by a Maven shim image (`tools/maven-shim`).
3. The shim image reference used in function/fixture Dockerfiles must be pullable by
   `buildx` (host registry tag + push when a host registry is configured).
4. Function/fixture Dockerfiles must consume a Maven base image that can be replaced by the shim.
5. Proxy URL parsing is strict (`http`/`https`, host required, no path/query/fragment, valid port).
6. `HTTPS_PROXY` is independent from `HTTP_PROXY`; if missing, HTTPS falls back to HTTP.
7. `NO_PROXY`/`no_proxy` is normalized to Maven `nonProxyHosts` deterministically.
8. Command logs must not expose proxy credentials.

## Maven Shim Model

`tools/maven-shim/mvn-wrapper.sh` is placed at `/usr/local/bin/mvn` inside a shim image.
When a build executes `mvn ...`, the wrapper:

1. Resolves proxy environment variables.
2. Generates a temporary `settings.xml`.
3. Executes the real Maven binary with `-s <generated settings.xml>`.

If no proxy is configured, it executes Maven directly without generated settings.

## Compose Guard

`tools/ci/check_maven_proxy_contract.sh` scans compose-managed service Dockerfiles and fails if
Maven usage appears there without explicit contract update.
