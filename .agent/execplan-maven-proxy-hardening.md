# Harden Maven Proxy Paths Without Dockerfile RUN Rewrites

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

After this change, every Maven dependency download that can occur during deploy-time image builds will follow one stable proxy contract, including authenticated proxies and `NO_PROXY` handling, without relying on brittle text rewriting of `RUN mvn ...` lines. The user-visible result is that Java fixture/function image builds succeed in proxy-only environments through both the E2E deploy path and `artifactctl deploy` path, while command logs no longer expose proxy credentials.

`docker compose up` is explicitly considered in scope analysis: it currently builds only control-plane service images and does not execute Maven in this repository, so it is not a direct Maven failure path today. We still add guardrails so future compose-time Maven introduction is detected and cannot silently bypass the contract.

## Progress

- [x] (2026-02-24 09:28Z) Reviewed current proxy changes and identified architectural risks (credential leakage, brittle Dockerfile rewrite, duplicated parser logic).
- [x] (2026-02-24 09:28Z) Mapped Maven execution paths (`e2e/runner/deploy.py`, `pkg/deployops/prepare_images.go`, Java fixture Dockerfile, runtime docs/tools).
- [x] (2026-02-24 09:28Z) Verified `docker compose up` build targets and confirmed no current Maven execution in compose-managed service Dockerfiles.
- [x] (2026-02-24 10:20Z) Introduced `pkg/proxy/maven` as proxy parsing/normalization/rendering source with unit tests.
- [x] (2026-02-24 10:20Z) Replaced deployops Maven `RUN` rewrite path with Maven builder-shim image strategy.
- [x] (2026-02-24 10:20Z) Migrated E2E Java fixture build path to Maven shim strategy (`MAVEN_IMAGE` build arg override).
- [x] (2026-02-24 10:20Z) Added command-log redaction for proxy credentials in `e2e/runner/logging.py`.
- [x] (2026-02-24 10:20Z) Added CI/static guard (`tools/ci/check_maven_proxy_contract.sh`) and wired it into quality-gates workflow.
- [ ] Execute full proxy validation matrix (remaining: tinyproxy-backed deploy scenario end-to-end for both docker/containerd modes).

## Surprises & Discoveries

- Observation: `docker compose up` can trigger image builds (`--build` or missing local images), but currently those compose build targets do not contain `mvn` usage.
  Evidence: `docker-compose.docker.yml` and `docker-compose.containerd.yml` contain multiple `build:` services, while repository search over `services/**/Dockerfile*` found no Maven commands.

- Observation: Current deployops behavior injects proxy-aware Maven settings by rewriting `RUN` lines and embedding base64 settings into generated Dockerfiles.
  Evidence: `pkg/deployops/prepare_images.go` (`rewriteDockerfileForMavenProxy`, `ARG ESB_MAVEN_SETTINGS_XML_B64="..."`).

- Observation: Current E2E runner logs full command lines, so `--build-arg HTTP_PROXY=http://user:pass@...` can leak credentials.
  Evidence: `e2e/runner/logging.py` logs `"$ {' '.join(cmd)}"` before execution.

- Observation: Maven shim image can be built and executed locally without changing caller Dockerfile `RUN mvn ...`.
  Evidence: `docker buildx build --tag esb-maven-shim:local-test tools/maven-shim` succeeded; `docker run ... esb-maven-shim:local-test mvn -v` succeeded.

## Decision Log

- Decision: Treat `docker compose up` Maven handling as a monitored non-goal for this implementation, not as an immediate code path to modify.
  Rationale: Compose-managed service builds currently do not execute Maven. Forcing the Maven-specific mechanism into compose path now would add complexity without immediate benefit. Instead we enforce detection guardrails so future Maven addition cannot drift.
  Date/Author: 2026-02-24 / Codex

- Decision: Remove Dockerfile `RUN mvn` string rewriting as the primary mechanism.
  Rationale: Line-oriented rewrite is fragile across multiline/json-form/run-wrapper variants and is difficult to prove safe.
  Date/Author: 2026-02-24 / Codex

- Decision: Standardize on a Maven builder-shim image approach for deploy-time function/fixture builds.
  Rationale: It centralizes Maven invocation behavior in one place, avoids per-Dockerfile command mutation, and can be applied consistently by both deployops and E2E runner.
  Date/Author: 2026-02-24 / Codex

- Decision: Keep proxy parsing/normalization logic in one Go module (`pkg/proxy/maven`) and use shared test vectors.
  Rationale: Avoid drift between Python and Go implementations and preserve strict, deterministic behavior.
  Date/Author: 2026-02-24 / Codex

- Decision: Keep compose path as a static-guard boundary, and do not auto-inject Maven shim into compose service builds now.
  Rationale: Compose services currently do not run Maven. Guardrails are sufficient until Maven appears there.
  Date/Author: 2026-02-24 / Codex

## Outcomes & Retrospective

Implemented outcomes:

- `pkg/deployops` no longer injects base64 settings or rewrites `RUN mvn` lines. It rewrites Maven base stages to shim images and fails fast on unsupported Maven-command patterns.
- E2E Java fixture build now uses Maven shim override (`MAVEN_IMAGE`) instead of generated settings build args.
- Proxy credential redaction is now applied to command logging.
- Maven proxy contract static guard was added to CI.

Remaining gap:

- Full tinyproxy-backed deploy scenario validation for both docker/containerd modes has not been executed in this implementation pass.

## Context and Orientation

This repository has two deploy-time paths that can build function images. First, the E2E runner path (`e2e/runner/deploy.py`) may pre-build local fixture images such as `e2e/fixtures/images/lambda/java/Dockerfile`. Second, `artifactctl deploy` path (`pkg/deployops/prepare_images.go`) builds function Dockerfiles from artifact outputs. Both currently pass proxy build args; Maven-specific handling is currently separate and partly duplicated.

A "Maven builder-shim image" in this plan means a Docker image derived from a Maven base image where the `mvn` entrypoint behavior is controlled by a small wrapper. The wrapper always materializes proxy settings and executes Maven with deterministic settings, so caller Dockerfiles can remain normal `RUN mvn ...` statements.

`docker compose up` controls control-plane services (`gateway`, `agent`, `provisioner`, `runtime-node`, and bases). Those Dockerfiles currently do not run Maven. We therefore do not couple compose lifecycle code to Maven proxy logic now, but we add static detection to fail fast if Maven appears later in compose-managed builds.

## Plan of Work

Milestone 1 establishes a single proxy contract implementation in Go. Add `pkg/proxy/maven` with functions for parsing `HTTP_PROXY`/`HTTPS_PROXY`, normalizing `NO_PROXY`, and rendering Maven `settings.xml`. This module must support separate HTTP and HTTPS proxy endpoints (with fallback from HTTPS to HTTP only when HTTPS is absent), preserve credentials, and validate URL shape. Add test vectors in `pkg/proxy/maven/testdata/proxy_cases.json` and table-driven tests that include invalid URLs, trailing slash, host:port tokens, IPv6 tokens, and dedupe behavior.

Milestone 2 adds the Maven builder-shim image definition under `tools/maven-shim`. The shim Dockerfile accepts a base Maven image reference and installs a wrapper that routes all Maven calls through generated settings. The wrapper must avoid writing secrets to stdout/stderr and should create temporary settings files with restrictive permissions. The wrapper may call a tiny helper binary included in the shim to avoid fragile shell URL parsing.

Milestone 3 migrates deployops path. In `pkg/deployops/prepare_images.go`, remove `rewriteDockerfileForMavenProxy` and replace it with a safer `FROM` rewrite that swaps Maven builder stage references to the local shim image when Maven base images are detected. Keep existing registry rewrite behavior intact. If `RUN mvn` is detected but no Maven base stage can be safely rewritten, fail with an actionable error message rather than applying partial mutation.

Milestone 4 migrates E2E runner fixture path. In `e2e/runner/deploy.py`, remove `ESB_MAVEN_SETTINGS_XML_B64` generation and pass-through. Ensure the Java fixture build uses the same shim image strategy (same contract, same proxy normalization behavior). Update `e2e/fixtures/images/lambda/java/Dockerfile` to plain Maven invocation without embedded conditional settings logic.

Milestone 5 adds secret-safe logging and drift guardrails. In `e2e/runner/logging.py` and deployops command logging surfaces, redact userinfo in proxy URLs before writing command lines. Add a CI script (for example `tools/ci/check_maven_proxy_contract.sh`) that scans compose-managed Dockerfile build contexts and fails if Maven is introduced without shim-contract integration.

## Concrete Steps

Run all commands from `/home/akira/esb3`.

1. Create and test proxy contract module.

    go test ./pkg/proxy/maven -count=1

Expected: tests pass and include both split-proxy and invalid-url cases.

2. Build and smoke-test shim image locally with proxy and without proxy.

    docker buildx build --platform linux/amd64 --load \
      --build-arg BASE_MAVEN_IMAGE=maven:3.9.11-eclipse-temurin-21 \
      --tag esb-maven-shim:test tools/maven-shim

    docker run --rm \
      -e HTTP_PROXY=http://user:pass@proxy.example:8080/ \
      -e NO_PROXY=localhost,127.0.0.1,.example.local \
      esb-maven-shim:test mvn -v

Expected: command succeeds; no proxy credentials printed in wrapper logs.

3. Validate deployops unit tests after migration.

    go test ./pkg/deployops -count=1

Expected: no tests depend on `RUN` line rewriting behavior; new tests cover safe failure when unsupported Maven pattern appears.

4. Validate E2E runner tests after migration.

    X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy \
      uv run pytest -q e2e/runner/tests/test_deploy_command.py

Expected: tests assert shim-related build behavior and no longer assert `ESB_MAVEN_SETTINGS_XML_B64` build arg.

5. Run proxy harness proof with Java enabled.

    uv run python tools/e2e_proxy/run_with_tinyproxy.py -- \
      uv run pytest -q e2e/scenarios/deploy

Expected: Java fixture/function image build path resolves Maven dependencies through proxy.

## Validation and Acceptance

Acceptance is behavioral and path-based.

For E2E deploy path, the Java fixture image build must complete in a proxy-only environment, and logs must not contain raw `user:password@` values from proxy URLs. For artifactctl deploy path, function image builds that invoke Maven must either succeed through the shim path or fail fast with explicit unsupported-pattern guidance; silent fallback to direct `mvn` without contract is not allowed. For compose path, no functional behavior changes are required today, but CI must fail if a compose-managed Dockerfile introduces Maven without contract integration.

The final acceptance run is: `go test ./...` for touched Go packages, targeted `pytest` for E2E runner tests, and one tinyproxy-backed deploy scenario proving actual network path correctness.

## Idempotence and Recovery

All file changes are additive or local refactors and can be repeated safely. Temporary shim images should use deterministic tags (content hash) so re-runs reuse cache when unchanged. If a build fails mid-way, rerunning the same command should be safe after fixing inputs. If unsupported Dockerfile patterns are detected, the system must fail before image push and provide exact remediation text.

If rollback is needed, restoring prior behavior requires reverting deployops and E2E runner commits; no persistent data migration is involved.

## Artifacts and Notes

The most important artifact is the path matrix to keep scope explicit:

- E2E local fixture image build (`e2e/runner/deploy.py`) -> in scope, must use shim contract.
- `artifactctl deploy` function image build (`pkg/deployops/prepare_images.go`) -> in scope, must use shim contract.
- `docker compose up` control-plane service build (`docker-compose.*.yml`) -> currently no Maven; monitored by CI guard, not direct migration target.

Any implementation PR must include before/after snippets showing that `RUN mvn` text rewrite logic was removed and replaced by deterministic builder-stage handling.

## Interfaces and Dependencies

Define the following interfaces and entry points by end of implementation.

In `pkg/proxy/maven`:

- `func ResolveEndpointsFromEnv(env map[string]string) (Endpoints, error)`
- `func RenderSettingsXML(endpoints Endpoints, noProxyRaw string) (string, error)`
- `func RedactProxyURL(raw string) string`

Where `Endpoints` contains optional HTTP and HTTPS endpoint structs with host, port, username, and password.

In `pkg/deployops/prepare_images.go`:

- replace `rewriteDockerfileForMavenProxy(...)` call site with Maven-base-stage rewrite orchestration.
- add a resolver that ensures shim image availability per Maven base ref (build once, reuse by tag).

In `e2e/runner/deploy.py`:

- remove `_maven_settings_b64_for_fixture` path.
- use shared shim preparation contract for Java fixture image build.

In CI tooling:

- add one static check script under `tools/ci/` that scans compose build contexts for Maven usage and enforces contract integration.

Revision note (2026-02-24): Initial zero-based architecture draft created after explicit scope review, with `docker compose up` path classified as monitored non-goal because no current compose-managed Maven execution exists.
Revision note (2026-02-24): Updated after implementation milestones landed (proxy module, shim integration, logging redaction, CI guard); remaining work narrowed to full tinyproxy scenario matrix.
