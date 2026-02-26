# Where: e2e/runner/proxy_harness.py
# What: Optional local proxy harness for E2E runner execution.
# Why: Integrate proxy setup directly into e2e/run_tests.py without external wrappers.
from __future__ import annotations

import os
import shlex
import shutil
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Mapping
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path
from typing import Any, TextIO
from xml.sax.saxutils import escape as xml_escape

from e2e.runner.env import apply_proxy_defaults
from e2e.runner.utils import PROJECT_ROOT, env_key

PROXY_PY_VERSION = "2.4.10"
DEFAULT_PROXY_PORT = 18888
DEFAULT_PROXY_BIND_HOST = "0.0.0.0"
DEFAULT_PROXY_AUTH_USER = "proxy-user"
DEFAULT_PROXY_AUTH_PASSWORD = "proxy-pass"
FILTER_UPSTREAM_PLUGIN = "proxy.plugin.filter_by_upstream.FilterByUpstreamHostPlugin"
CHECK_URL = "https://repo.maven.apache.org/maven2/"
JAVA_PROXY_PROOF_IMAGE = (
    "public.ecr.aws/sam/build-java21"
    "@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7"
)
JAVA_PROXY_PROOF_PROJECT = Path("e2e/fixtures/functions/java/echo")
JAVA_PROXY_PROOF_SETTINGS_PATH = "/tmp/m2/settings.xml"
JAVA_PROXY_PROOF_COMMAND = (
    "set -euo pipefail; "
    "mkdir -p /tmp/work; "
    "cp -a /src/. /tmp/work; "
    "cd /tmp/work; "
    f"mvn -s {JAVA_PROXY_PROOF_SETTINGS_PATH} -q "
    "-Dmaven.artifact.threads=1 -DskipTests dependency:go-offline"
)

_PROXY_ENV_KEYS = (
    "HTTP_PROXY",
    "http_proxy",
    "HTTPS_PROXY",
    "https_proxy",
    "NO_PROXY",
    "no_proxy",
    env_key("NO_PROXY_EXTRA"),
)


class ProxyHarnessError(RuntimeError):
    """Raised when proxy harness setup or verification fails."""


@dataclass(slots=True)
class ProxyHarnessOptions:
    enabled: bool = False
    port: int = DEFAULT_PROXY_PORT
    bind_host: str = DEFAULT_PROXY_BIND_HOST


@dataclass(slots=True)
class ProxyProcess:
    process: subprocess.Popen[str]
    log_path: Path
    log_stream: TextIO
    port: int


def options_from_args(args: Any) -> ProxyHarnessOptions:
    enabled = bool(getattr(args, "with_proxy", False))
    return ProxyHarnessOptions(enabled=enabled)


def split_no_proxy(value: str) -> list[str]:
    if not value:
        return []
    normalized = value.replace(";", ",")
    return [item.strip() for item in normalized.split(",") if item.strip()]


def build_proxy_url(
    proxy_host: str,
    proxy_port: int,
    *,
    redact_password: bool = False,
) -> str:
    host = proxy_host.strip()
    if not host:
        raise ProxyHarnessError("proxy host must not be empty")
    if proxy_port < 1 or proxy_port > 65535:
        raise ProxyHarnessError(f"proxy port out of range: {proxy_port}")
    user = urllib.parse.quote(DEFAULT_PROXY_AUTH_USER, safe="")
    password = urllib.parse.quote(DEFAULT_PROXY_AUTH_PASSWORD, safe="")
    if redact_password:
        password = "***"
    return f"http://{user}:{password}@{host}:{proxy_port}"


def resolve_bridge_gateway() -> str:
    cmd = [
        "docker",
        "network",
        "inspect",
        "bridge",
        "--format",
        "{{(index .IPAM.Config 0).Gateway}}",
    ]
    try:
        output = subprocess.check_output(cmd, text=True, stderr=subprocess.DEVNULL).strip()
    except (OSError, subprocess.CalledProcessError):
        return "127.0.0.1"
    return output or "127.0.0.1"


def build_proxy_env(
    base_env: Mapping[str, str],
    *,
    proxy_url: str,
    proxy_host: str,
) -> dict[str, str]:
    env = dict(base_env)
    for key in ("HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"):
        env[key] = proxy_url

    extra_key = env_key("NO_PROXY_EXTRA")
    extra_values = split_no_proxy(env.get(extra_key, ""))
    if proxy_host and proxy_host not in extra_values:
        extra_values.append(proxy_host)
    if extra_values:
        env[extra_key] = ",".join(extra_values)

    apply_proxy_defaults(env)
    return env


def _normalize_filtered_upstream_host(token: str) -> str:
    value = token.strip()
    if not value:
        return ""

    if value.startswith("[") and "]" in value:
        closing_index = value.find("]")
        ipv6_host = value[1:closing_index].strip()
        if ipv6_host:
            value = ipv6_host
    elif value.count(":") == 1:
        host, port = value.rsplit(":", 1)
        if port.isdigit():
            value = host.strip()

    # filter_by_upstream is exact host match; skip patterns/CIDRs that cannot match exactly.
    if "/" in value or "*" in value:
        return ""
    if value.startswith("."):
        value = value[1:]

    return value.strip().lower()


def _filtered_upstream_hosts_from_no_proxy(no_proxy_value: str) -> list[str]:
    hosts: list[str] = []
    seen: set[str] = set()
    for token in split_no_proxy(no_proxy_value):
        normalized = _normalize_filtered_upstream_host(token)
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        hosts.append(normalized)
    return hosts


def _effective_no_proxy(env: Mapping[str, str]) -> str:
    merged: list[str] = []
    seen: set[str] = set()
    for key in ("NO_PROXY", "no_proxy"):
        for token in split_no_proxy(env.get(key, "")):
            if token in seen:
                continue
            seen.add(token)
            merged.append(token)
    return ",".join(merged)


def _apply_proxy_env_to_process(proxy_env: Mapping[str, str]) -> dict[str, str | None]:
    previous: dict[str, str | None] = {}
    for key in _PROXY_ENV_KEYS:
        previous[key] = os.environ.get(key)
        value = proxy_env.get(key, "")
        if value:
            os.environ[key] = value
        else:
            os.environ.pop(key, None)
    return previous


def _restore_proxy_env(previous: Mapping[str, str | None]) -> None:
    for key, value in previous.items():
        if value is None:
            os.environ.pop(key, None)
        else:
            os.environ[key] = value


def _print_command(prefix: str, cmd: list[str]) -> None:
    rendered = " ".join(shlex.quote(part) for part in cmd)
    print(f"[proxy-e2e] {prefix}: {rendered}")


def _resolve_proxy_command() -> list[str]:
    proxy_bin = shutil.which("proxy")
    if proxy_bin:
        return [proxy_bin]

    uvx_bin = shutil.which("uvx")
    if uvx_bin:
        return [uvx_bin, "--from", f"proxy-py=={PROXY_PY_VERSION}", "proxy"]

    raise ProxyHarnessError(
        "proxy command not found. Install proxy-py or ensure uvx is available (mise run setup)."
    )


def _tail_log(path: Path, lines: int = 40) -> str:
    try:
        data = path.read_text(encoding="utf-8", errors="replace").splitlines()
    except OSError:
        return "<proxy log unavailable>"
    if not data:
        return "<proxy log is empty>"
    return "\n".join(data[-lines:])


def _resolve_proxy_log_path(log_label: str) -> Path:
    safe_label = "".join(ch if ch.isalnum() or ch in {"-", "_"} else "-" for ch in log_label)
    safe_label = safe_label.strip("-_") or "proxy"
    return PROJECT_ROOT / "e2e" / f".parallel-{safe_label}.log"


def _start_proxy_process(
    *,
    bind_host: str,
    host_port: int,
    filtered_upstream_hosts: list[str],
    log_label: str = "proxy-e2e",
) -> ProxyProcess:
    blocked_hosts_csv = ",".join(filtered_upstream_hosts) or "localhost,127.0.0.1,::1"
    cmd = [
        *_resolve_proxy_command(),
        "--hostname",
        bind_host,
        "--port",
        str(host_port),
        "--num-workers",
        "1",
        "--num-acceptors",
        "1",
        "--log-level",
        "WARNING",
        "--basic-auth",
        f"{DEFAULT_PROXY_AUTH_USER}:{DEFAULT_PROXY_AUTH_PASSWORD}",
        "--plugins",
        FILTER_UPSTREAM_PLUGIN,
        "--filtered-upstream-hosts",
        blocked_hosts_csv,
    ]
    _print_command("start proxy", cmd)

    log_path = _resolve_proxy_log_path(log_label)
    log_path.parent.mkdir(parents=True, exist_ok=True)
    log_stream = log_path.open("w", encoding="utf-8")
    process = subprocess.Popen(
        cmd,
        stdout=log_stream,
        stderr=subprocess.STDOUT,
        text=True,
    )

    proxy = ProxyProcess(process=process, log_path=log_path, log_stream=log_stream, port=host_port)

    try:
        _wait_for_port("127.0.0.1", host_port)
    except Exception as exc:  # pragma: no cover - startup failure path
        _stop_proxy_process(proxy, keep_logs=True)
        tail = _tail_log(log_path)
        raise ProxyHarnessError(
            f"proxy did not become ready at 127.0.0.1:{host_port} within timeout\n{tail}"
        ) from exc

    return proxy


def _stop_proxy_process(proxy: ProxyProcess, *, keep_logs: bool = False) -> None:
    process = proxy.process
    if process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)

    try:
        proxy.log_stream.close()
    except OSError:
        pass

    if not keep_logs:
        proxy.log_path.unlink(missing_ok=True)


def _wait_for_port(host: str, port: int, timeout: float = 20.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with socket.create_connection((host, port), timeout=1.0):
                return
        except OSError:
            time.sleep(0.25)
    raise ProxyHarnessError(f"proxy did not become ready at {host}:{port} within {timeout:.1f}s")


def _find_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return int(sock.getsockname()[1])


def _probe_proxy(proxy_url: str, timeout: float = 15.0) -> None:
    curl_path = shutil.which("curl")
    if curl_path:
        _probe_proxy_with_curl(curl_path, proxy_url, timeout)
        return
    _probe_proxy_with_urllib(proxy_url, timeout)


def _probe_proxy_with_curl(curl_path: str, proxy_url: str, timeout: float) -> None:
    deadline = time.monotonic() + timeout
    last_error = ""
    while time.monotonic() < deadline:
        result = subprocess.run(
            [
                curl_path,
                "--silent",
                "--show-error",
                "--fail",
                "--proxy",
                proxy_url,
                "--max-time",
                "3",
                "--output",
                "/dev/null",
                CHECK_URL,
            ],
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode == 0:
            return
        last_error = (result.stderr or result.stdout).strip() or f"curl exit={result.returncode}"
        time.sleep(0.5)
    raise ProxyHarnessError(f"proxy probe failed via {proxy_url}: {last_error}")


def _probe_proxy_with_urllib(proxy_url: str, timeout: float) -> None:
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({"http": proxy_url, "https": proxy_url})
    )
    deadline = time.monotonic() + timeout
    last_error: Exception | None = None
    while time.monotonic() < deadline:
        try:
            with opener.open(CHECK_URL, timeout=3.0) as response:
                status = getattr(response, "status", 200)
                if status < 500:
                    return
                last_error = ProxyHarnessError(f"proxy probe failed with status={status}")
        except (OSError, urllib.error.URLError) as exc:
            last_error = exc
        time.sleep(0.5)
    raise ProxyHarnessError(f"proxy probe failed via {proxy_url}: {last_error}")


def _normalize_maven_non_proxy_token(token: str) -> str:
    normalized = token.strip()
    if not normalized:
        return ""

    if normalized.startswith("[") and "]" in normalized:
        closing_index = normalized.find("]")
        ipv6_host = normalized[1:closing_index].strip()
        if ipv6_host:
            normalized = ipv6_host
    elif normalized.count(":") == 1:
        host, port = normalized.rsplit(":", 1)
        if port.isdigit():
            normalized = host.strip()

    if normalized.startswith(".") and not normalized.startswith("*."):
        normalized = f"*{normalized}"

    return normalized


def _maven_non_proxy_hosts(no_proxy_value: str) -> str:
    seen: set[str] = set()
    values: list[str] = []
    for token in split_no_proxy(no_proxy_value):
        normalized = _normalize_maven_non_proxy_token(token)
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        values.append(normalized)
    return "|".join(values)


def _parse_proxy_endpoint(proxy_url: str) -> tuple[str, int, str, str]:
    parsed = urllib.parse.urlsplit(proxy_url.strip())
    if not parsed.scheme or not parsed.hostname:
        raise ProxyHarnessError(f"proxy URL must include scheme and host: {proxy_url}")
    scheme = parsed.scheme.lower()
    if scheme not in {"http", "https"}:
        raise ProxyHarnessError(f"proxy URL must use http or https: {proxy_url}")
    if parsed.path not in {"", "/"} or parsed.query or parsed.fragment:
        raise ProxyHarnessError(f"proxy URL must not include path/query/fragment: {proxy_url}")

    try:
        port = parsed.port
    except ValueError as exc:
        raise ProxyHarnessError(f"proxy URL has invalid port: {proxy_url}") from exc
    if port is None:
        port = 80 if scheme == "http" else 443
    if port < 1 or port > 65535:
        raise ProxyHarnessError(f"proxy URL has invalid port: {proxy_url}")

    return (
        parsed.hostname,
        port,
        urllib.parse.unquote(parsed.username or ""),
        urllib.parse.unquote(parsed.password or ""),
    )


def _render_maven_proxy_settings(proxy_url: str, no_proxy: str) -> str:
    host, port, username, password = _parse_proxy_endpoint(proxy_url)
    non_proxy_hosts = _maven_non_proxy_hosts(no_proxy)

    lines = [
        "<settings>",
        "  <proxies>",
    ]
    for proxy_id, protocol in (("http-proxy", "http"), ("https-proxy", "https")):
        lines.extend(
            [
                "    <proxy>",
                f"      <id>{xml_escape(proxy_id)}</id>",
                "      <active>true</active>",
                f"      <protocol>{xml_escape(protocol)}</protocol>",
                f"      <host>{xml_escape(host)}</host>",
                f"      <port>{port}</port>",
            ]
        )
        if username:
            lines.append(f"      <username>{xml_escape(username)}</username>")
        if password:
            lines.append(f"      <password>{xml_escape(password)}</password>")
        if non_proxy_hosts:
            lines.append(f"      <nonProxyHosts>{xml_escape(non_proxy_hosts)}</nonProxyHosts>")
        lines.append("    </proxy>")
    lines.extend(["  </proxies>", "</settings>"])
    return "\n".join(lines) + "\n"


def _run_java_maven_with_settings(
    *,
    settings_xml: str,
) -> subprocess.CompletedProcess[str]:
    project_dir = PROJECT_ROOT / JAVA_PROXY_PROOF_PROJECT
    if not project_dir.exists():
        raise ProxyHarnessError(f"java proxy-proof fixture not found: {project_dir}")

    with tempfile.NamedTemporaryFile(
        mode="w",
        encoding="utf-8",
        suffix="-java-proxy-proof-settings.xml",
        delete=False,
    ) as temp_file:
        temp_file.write(settings_xml)
        settings_path = Path(temp_file.name)

    command = [
        "docker",
        "run",
        "--rm",
        "-v",
        f"{project_dir}:/src:ro",
        "-v",
        f"{settings_path}:{JAVA_PROXY_PROOF_SETTINGS_PATH}:ro",
        "-e",
        "MAVEN_CONFIG=/tmp/m2",
        "-e",
        "HOME=/tmp",
        "-e",
        "HTTP_PROXY=",
        "-e",
        "http_proxy=",
        "-e",
        "HTTPS_PROXY=",
        "-e",
        "https_proxy=",
        "-e",
        "NO_PROXY=",
        "-e",
        "no_proxy=",
        JAVA_PROXY_PROOF_IMAGE,
        "bash",
        "-lc",
        JAVA_PROXY_PROOF_COMMAND,
    ]

    try:
        return subprocess.run(command, capture_output=True, text=True, check=False)
    finally:
        settings_path.unlink(missing_ok=True)


def _start_java_proxy_proof_proxy(
    *,
    bind_host: str,
    no_proxy: str,
) -> tuple[ProxyProcess, str]:
    host_port = _find_free_port()
    filtered_upstream_hosts = _filtered_upstream_hosts_from_no_proxy(no_proxy)
    proxy = _start_proxy_process(
        bind_host=bind_host,
        host_port=host_port,
        filtered_upstream_hosts=filtered_upstream_hosts,
        log_label="proxy-e2e-java-proof",
    )
    local_probe_url = build_proxy_url("127.0.0.1", host_port)
    try:
        _probe_proxy(local_probe_url)
    except Exception:
        _stop_proxy_process(proxy, keep_logs=True)
        raise
    proxy_host = resolve_bridge_gateway()
    return proxy, build_proxy_url(proxy_host, host_port)


def _run_java_proxy_proof(
    *,
    bind_host: str,
    no_proxy: str,
) -> None:
    proof_proxy, proof_proxy_url = _start_java_proxy_proof_proxy(
        bind_host=bind_host,
        no_proxy=no_proxy,
    )
    try:
        print(f"[proxy-e2e] java proxy-proof log: {proof_proxy.log_path}")
        print("[proxy-e2e] java proxy-proof (A): expect success with configured proxy settings")
        settings_xml = _render_maven_proxy_settings(proof_proxy_url, no_proxy)
        positive = _run_java_maven_with_settings(settings_xml=settings_xml)
        if positive.returncode != 0:
            details = (positive.stdout + "\n" + positive.stderr).strip()
            raise ProxyHarnessError(f"java proxy-proof (A) failed unexpectedly\n{details}")
        print("[proxy-e2e] java proxy-proof (A) OK")

        print("[proxy-e2e] java proxy-proof (B): expect failure with broken proxy settings")
        bad_settings_xml = _render_maven_proxy_settings("http://127.0.0.1:9", no_proxy)
        negative = _run_java_maven_with_settings(settings_xml=bad_settings_xml)
        if negative.returncode == 0:
            raise ProxyHarnessError(
                "java proxy-proof (B) unexpectedly succeeded; "
                "Maven appears to bypass proxy settings"
            )
        print("[proxy-e2e] java proxy-proof (B) OK (expected failure detected)")
    finally:
        _stop_proxy_process(proof_proxy, keep_logs=True)


@contextmanager
def proxy_harness(options: ProxyHarnessOptions):
    if not options.enabled:
        yield None
        return

    proxy_host = resolve_bridge_gateway()
    proxy_url = build_proxy_url(proxy_host, options.port)
    display_proxy_url = build_proxy_url(proxy_host, options.port, redact_password=True)

    proxy_env = build_proxy_env(
        os.environ,
        proxy_url=proxy_url,
        proxy_host=proxy_host,
    )
    effective_no_proxy = _effective_no_proxy(proxy_env)
    filtered_upstream_hosts = _filtered_upstream_hosts_from_no_proxy(effective_no_proxy)

    proxy_process = _start_proxy_process(
        bind_host=options.bind_host,
        host_port=options.port,
        filtered_upstream_hosts=filtered_upstream_hosts,
        log_label="proxy-e2e",
    )
    previous = _apply_proxy_env_to_process(proxy_env)

    try:
        print(f"[proxy-e2e] proxy URL: {display_proxy_url}")
        print(f"[proxy-e2e] proxy log: {proxy_process.log_path}")
        print(f"[proxy-e2e] NO_PROXY: {effective_no_proxy}")

        _probe_proxy(build_proxy_url("127.0.0.1", options.port))
        print(f"[proxy-e2e] outbound probe OK: {CHECK_URL}")

        _run_java_proxy_proof(
            bind_host=options.bind_host,
            no_proxy=effective_no_proxy,
        )

        yield proxy_process
    finally:
        _restore_proxy_env(previous)
        _stop_proxy_process(proxy_process, keep_logs=True)
