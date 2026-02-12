<!--
Where: tools/e2e_proxy/README.md
What: Usage guide for running E2E under a local tinyproxy.
Why: Provide a reproducible proxy validation workflow for regressions.
-->
# E2E Proxy Harness (tinyproxy)

`tools/e2e_proxy/run_with_tinyproxy.py` starts a local `tinyproxy` container,
exports proxy env vars for both host and Docker workloads, and runs an E2E command.

It is useful when validating:
- deploy-time image resolution under proxy
- Java warmup/build behavior under proxy
- `NO_PROXY` bypass for local services (`gateway`, `registry`, etc.)

## Quick Start

Connectivity check only:

```bash
uv run python tools/e2e_proxy/run_with_tinyproxy.py --check-only
```

Run E2E through tinyproxy:

```bash
uv run python tools/e2e_proxy/run_with_tinyproxy.py -- \
  uv run e2e/run_tests.py --profile e2e-docker --verbose
```

By default, this command also runs a strict Java proxy-proof before E2E.
The proof starts an additional temporary tinyproxy instance
(BasicAuth enabled when credentials are provided), then checks:
- (A) Maven resolution succeeds with generated proxy `settings.xml`
- (B) Maven resolution fails with intentionally broken proxy `settings.xml`

If you need to skip this strict proof, use:

```bash
uv run python tools/e2e_proxy/run_with_tinyproxy.py --skip-java-proxy-proof -- \
  uv run e2e/run_tests.py --profile e2e-docker --verbose
```

Run with credentialed proxy URL:

```bash
uv run python tools/e2e_proxy/run_with_tinyproxy.py \
  --proxy-user proxyuser \
  --proxy-password proxypass \
  --check-only
```

If no command is provided, the default command is:

```bash
uv run e2e/run_tests.py --parallel --verbose
```

## Useful Options

- `--keep-proxy`: keep tinyproxy running after command ends
- `--skip-probe`: skip outbound proxy probe to Maven Central
- `--skip-java-proxy-proof`: skip strict Java proxy-proof (A/B) checks
- `--proxy-host`: override proxy host used in `HTTP(S)_PROXY`
- `--proxy-user` / `--proxy-password`: embed credentials into `HTTP(S)_PROXY` URLs
- `--port`: change tinyproxy exposed host port (default: `18888`)
- `--no-proxy-extra`: append additional `NO_PROXY` targets

By default, proxy host is Docker bridge gateway (`docker network inspect bridge ...`),
so both host processes and Docker containers can reach the same proxy endpoint.

Canonical environment variable names (legacy `ESB_*` is still accepted as fallback):
- `TINYPROXY_IMAGE`
- `TINYPROXY_CONTAINER`
- `TINYPROXY_PORT`
- `TINYPROXY_ACL`
- `TINYPROXY_HOST`
- `TINYPROXY_USER`
- `TINYPROXY_PASSWORD`
- `TINYPROXY_NO_PROXY_EXTRA`

## Auth Notes

- You can also set credentials with env vars:
  - `TINYPROXY_USER`（互換: `ESB_TINYPROXY_USER`）
  - `TINYPROXY_PASSWORD`（互換: `ESB_TINYPROXY_PASSWORD`）
- `tinyproxy` BasicAuth values must not contain whitespace/control characters.
- Java proxy-proof uses `-Dmaven.artifact.threads=1` for reproducible authenticated proxy runs.
