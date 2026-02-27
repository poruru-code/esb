<!--
Where: e2e/docs/proxy-harness.md
What: Usage guide for running E2E under the integrated proxy harness.
Why: Keep proxy workflow simple and stable with a single public switch.
-->
# E2E Proxy Harness

`uv run e2e/run_tests.py --with-proxy` starts a local `proxy.py` process,
injects proxy environment variables, runs proxy checks, and then executes E2E.

## Single Switch

Use only this switch:

```bash
uv run e2e/run_tests.py --with-proxy --parallel --verbose
```

You can combine it with normal E2E options such as `--profile`, `--parallel`, and `--verbose`.

## Fixed Defaults

When `--with-proxy` is enabled, these defaults are fixed:

- proxy runtime: `proxy-py==2.4.10` (or local `proxy` binary if present)
- proxy bind: `0.0.0.0:18888`
- proxy auth: BasicAuth enabled (`proxy-user` / `proxy-pass`)
- proxy host for `HTTP(S)_PROXY`: Docker bridge gateway (fallback `127.0.0.1`)
- proxy deny rule: destinations in resolved `NO_PROXY`/`no_proxy` are blocked at proxy side
- outbound probe: enabled (`https://repo.maven.apache.org/maven2/`)
- Java proxy-proof A/B checks: enabled
- Buildx builder: `BUILDX_BUILDER` is scoped to `*-proxy` during run
- process cleanup: proxy is stopped automatically after run
- proxy logs: `e2e/.parallel-proxy-e2e.log` and `e2e/.parallel-proxy-e2e-java-proof.log` (overwritten per run)

## Notes

- `NO_PROXY`/`no_proxy` are normalized via the existing E2E proxy default logic.
- If a request is forced through proxy and destination host matches resolved no-proxy hosts, proxy returns reject response.
- If proxy startup/probe/proof fails, `run_tests.py` exits non-zero before scenario execution.
