#!/usr/bin/env python3
# Where: tools/e2e_proxy/run_with_tinyproxy.py
# What: Runs E2E commands with a local tinyproxy container as HTTP(S) proxy.
# Why: Reproduce proxy-network behavior locally and validate proxy handling.
from __future__ import annotations

import argparse
import os
import shlex
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Mapping, Sequence
from pathlib import Path
from xml.sax.saxutils import escape as xml_escape

DEFAULT_IMAGE = "monokal/tinyproxy:latest"
DEFAULT_CONTAINER_NAME = "esb-e2e-tinyproxy"
DEFAULT_PORT = 18888
DEFAULT_ACL = "ANY"
DEFAULT_NO_PROXY_TARGETS = (
    "agent",
    "database",
    "gateway",
    "host.docker.internal",
    "local-proxy",
    "localhost",
    "registry",
    "runtime-node",
    "s3-storage",
    "victorialogs",
    "::1",
    "127.0.0.1",
)
DEFAULT_COMMAND = ["uv", "run", "e2e/run_tests.py", "--parallel", "--verbose"]
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


def split_no_proxy(value: str) -> list[str]:
    if not value:
        return []
    normalized = value.replace(";", ",")
    return [item.strip() for item in normalized.split(",") if item.strip()]


def merge_no_proxy(existing: str, defaults: Sequence[str], extra: str = "") -> str:
    merged: list[str] = []
    seen: set[str] = set()
    for item in split_no_proxy(existing) + list(defaults) + split_no_proxy(extra):
        if item in seen:
            continue
        seen.add(item)
        merged.append(item)
    return ",".join(merged)


def normalize_command(command: list[str]) -> list[str]:
    if command and command[0] == "--":
        return command[1:]
    return command


def _validate_tinyproxy_auth_value(field_name: str, value: str) -> str:
    stripped = value.strip()
    if not stripped:
        raise ValueError(f"{field_name} must not be empty")
    if any(character.isspace() for character in stripped):
        raise ValueError(f"{field_name} must not include whitespace for tinyproxy BasicAuth")
    if any(ord(character) < 0x20 or ord(character) == 0x7F for character in stripped):
        raise ValueError(
            f"{field_name} must not include control characters for tinyproxy BasicAuth"
        )
    return stripped


def resolve_proxy_auth(username: str, password: str) -> tuple[str, str] | None:
    user = username.strip()
    passwd = password.strip()
    if not user and not passwd:
        return None
    if not user or not passwd:
        raise ValueError("proxy auth requires both username and password")
    return (
        _validate_tinyproxy_auth_value("proxy username", user),
        _validate_tinyproxy_auth_value("proxy password", passwd),
    )


def build_proxy_url(
    proxy_host: str,
    proxy_port: int,
    *,
    auth: tuple[str, str] | None = None,
    redact_password: bool = False,
) -> str:
    host = proxy_host.strip()
    if not auth:
        return f"http://{host}:{proxy_port}"

    user, passwd = auth
    encoded_user = urllib.parse.quote(user, safe="")
    encoded_pass = urllib.parse.quote(passwd, safe="")
    if redact_password:
        encoded_pass = "***"
    return f"http://{encoded_user}:{encoded_pass}@{host}:{proxy_port}"


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
    no_proxy_extra: str = "",
) -> dict[str, str]:
    env = dict(base_env)
    for key in ("HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"):
        env[key] = proxy_url

    existing_no_proxy = env.get("NO_PROXY", "").strip() or env.get("no_proxy", "").strip()
    defaults = list(DEFAULT_NO_PROXY_TARGETS)
    if proxy_host:
        defaults.append(proxy_host)
    merged = merge_no_proxy(existing_no_proxy, defaults, no_proxy_extra)
    env["NO_PROXY"] = merged
    env["no_proxy"] = merged
    return env


def _print_command(prefix: str, cmd: Sequence[str]) -> None:
    rendered = " ".join(shlex.quote(part) for part in cmd)
    print(f"[proxy-e2e] {prefix}: {rendered}")


def _remove_container(container_name: str) -> None:
    subprocess.run(
        ["docker", "rm", "-f", container_name],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
        check=False,
    )


def _start_tinyproxy(
    *,
    image: str,
    container_name: str,
    host_port: int,
    acl: str,
    auth: tuple[str, str] | None = None,
) -> None:
    _remove_container(container_name)
    cmd = [
        "docker",
        "run",
        "-d",
        "--rm",
        "--name",
        container_name,
        "-p",
        f"{host_port}:8888",
    ]
    run_env = os.environ.copy()
    if auth:
        user, passwd = auth
        run_env["BASIC_AUTH_USER"] = user
        run_env["BASIC_AUTH_PASSWORD"] = passwd
        cmd.extend(["-e", "BASIC_AUTH_USER", "-e", "BASIC_AUTH_PASSWORD"])
    cmd.extend([image, *shlex.split(acl)])
    _print_command("start tinyproxy", cmd)
    subprocess.run(cmd, check=True, env=run_env)


def _wait_for_port(host: str, port: int, timeout: float = 20.0) -> None:
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with socket.create_connection((host, port), timeout=1.0):
                return
        except OSError:
            time.sleep(0.25)
    raise RuntimeError(f"tinyproxy did not become ready at {host}:{port} within {timeout:.1f}s")


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
    raise RuntimeError(f"proxy probe failed via {proxy_url}: {last_error}")


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
                last_error = RuntimeError(f"proxy probe failed with status={status}")
        except (OSError, urllib.error.URLError) as exc:
            last_error = exc
        time.sleep(0.5)
    raise RuntimeError(f"proxy probe failed via {proxy_url}: {last_error}")


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
        raise ValueError(f"proxy URL must include scheme and host: {proxy_url}")
    scheme = parsed.scheme.lower()
    if scheme not in {"http", "https"}:
        raise ValueError(f"proxy URL must use http or https: {proxy_url}")
    if parsed.path not in {"", "/"} or parsed.query or parsed.fragment:
        raise ValueError(f"proxy URL must not include path/query/fragment: {proxy_url}")

    try:
        port = parsed.port
    except ValueError as exc:
        raise ValueError(f"proxy URL has invalid port: {proxy_url}") from exc
    if port is None:
        port = 80 if scheme == "http" else 443
    if port < 1 or port > 65535:
        raise ValueError(f"proxy URL has invalid port: {proxy_url}")

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
    lines.extend(
        [
            "  </proxies>",
            "</settings>",
        ]
    )
    return "\n".join(lines) + "\n"


def _run_java_maven_with_settings(
    *,
    repo_root: Path,
    settings_xml: str,
) -> subprocess.CompletedProcess[str]:
    project_dir = repo_root / JAVA_PROXY_PROOF_PROJECT
    if not project_dir.exists():
        raise RuntimeError(f"java proxy-proof fixture not found: {project_dir}")

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
        return subprocess.run(
            command,
            capture_output=True,
            text=True,
            check=False,
        )
    finally:
        settings_path.unlink(missing_ok=True)


def _start_java_proxy_proof_proxy(
    *,
    image: str,
    auth: tuple[str, str] | None,
) -> tuple[str, str]:
    container_name = f"{DEFAULT_CONTAINER_NAME}-proof-{os.getpid()}"
    host_port = _find_free_port()
    _start_tinyproxy(
        image=image,
        container_name=container_name,
        host_port=host_port,
        acl=DEFAULT_ACL,
        auth=auth,
    )
    _wait_for_port("127.0.0.1", host_port)
    proxy_host = resolve_bridge_gateway()
    local_probe_url = build_proxy_url("127.0.0.1", host_port, auth=auth)
    _probe_proxy(local_probe_url)
    return container_name, build_proxy_url(proxy_host, host_port, auth=auth)


def _run_java_proxy_proof(
    *,
    repo_root: Path,
    no_proxy: str,
    image: str,
    auth: tuple[str, str] | None,
) -> None:
    proof_container, proof_proxy_url = _start_java_proxy_proof_proxy(image=image, auth=auth)
    try:
        print("[proxy-e2e] java proxy-proof (A): expect success with configured proxy settings")
        settings_xml = _render_maven_proxy_settings(proof_proxy_url, no_proxy)
        positive = _run_java_maven_with_settings(repo_root=repo_root, settings_xml=settings_xml)
        if positive.returncode != 0:
            details = (positive.stdout + "\n" + positive.stderr).strip()
            raise RuntimeError(f"java proxy-proof (A) failed unexpectedly\n{details}")
        print("[proxy-e2e] java proxy-proof (A) OK")

        print("[proxy-e2e] java proxy-proof (B): expect failure with broken proxy settings")
        bad_settings_xml = _render_maven_proxy_settings("http://127.0.0.1:9", no_proxy)
        negative = _run_java_maven_with_settings(repo_root=repo_root, settings_xml=bad_settings_xml)
        if negative.returncode == 0:
            raise RuntimeError(
                "java proxy-proof (B) unexpectedly succeeded; "
                "Maven appears to bypass proxy settings"
            )
        print("[proxy-e2e] java proxy-proof (B) OK (expected failure detected)")
    finally:
        _remove_container(proof_container)


def parse_args(argv: Sequence[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run E2E command through a local tinyproxy container."
    )
    parser.add_argument(
        "--image",
        default=os.environ.get("ESB_TINYPROXY_IMAGE", DEFAULT_IMAGE),
        help=f"Tinyproxy image (default: {DEFAULT_IMAGE})",
    )
    parser.add_argument(
        "--container-name",
        default=os.environ.get("ESB_TINYPROXY_CONTAINER", DEFAULT_CONTAINER_NAME),
        help=f"Tinyproxy container name (default: {DEFAULT_CONTAINER_NAME})",
    )
    parser.add_argument(
        "--port",
        type=int,
        default=int(os.environ.get("ESB_TINYPROXY_PORT", str(DEFAULT_PORT))),
        help=f"Host port to expose tinyproxy (default: {DEFAULT_PORT})",
    )
    parser.add_argument(
        "--acl",
        default=os.environ.get("ESB_TINYPROXY_ACL", DEFAULT_ACL),
        help=f"tinyproxy ACL argument(s), shell-split (default: {DEFAULT_ACL})",
    )
    parser.add_argument(
        "--proxy-host",
        default=os.environ.get("ESB_TINYPROXY_HOST", ""),
        help="Proxy host used in HTTP(S)_PROXY. Default is Docker bridge gateway.",
    )
    parser.add_argument(
        "--proxy-user",
        default=os.environ.get("ESB_TINYPROXY_USER", ""),
        help="Proxy auth username (enables tinyproxy BasicAuth when paired with password).",
    )
    parser.add_argument(
        "--proxy-password",
        default=os.environ.get("ESB_TINYPROXY_PASSWORD", ""),
        help="Proxy auth password (enables tinyproxy BasicAuth when paired with username).",
    )
    parser.add_argument(
        "--no-proxy-extra",
        default=os.environ.get("ESB_TINYPROXY_NO_PROXY_EXTRA", ""),
        help="Additional NO_PROXY entries (comma/semicolon separated).",
    )
    parser.add_argument(
        "--skip-probe",
        action="store_true",
        help="Skip outbound proxy probe to Maven Central.",
    )
    parser.add_argument(
        "--check-only",
        action="store_true",
        help="Start proxy and run connectivity probe only.",
    )
    parser.add_argument(
        "--keep-proxy",
        action="store_true",
        help="Keep tinyproxy container running after command exits.",
    )
    parser.add_argument(
        "--skip-java-proxy-proof",
        action="store_true",
        help="Skip strict Java proxy-proof checks before running the target command.",
    )
    parser.add_argument(
        "command",
        nargs=argparse.REMAINDER,
        help="Command to run. Use `--` before command args.",
    )
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(argv if argv is not None else sys.argv[1:])
    command = normalize_command(list(args.command))
    if not command and not args.check_only:
        command = list(DEFAULT_COMMAND)

    repo_root = Path(__file__).resolve().parents[2]
    try:
        auth = resolve_proxy_auth(args.proxy_user, args.proxy_password)
    except ValueError as exc:
        print(f"[proxy-e2e] ERROR: {exc}", file=sys.stderr)
        return 2
    proxy_host = args.proxy_host.strip() or resolve_bridge_gateway()
    proxy_url = build_proxy_url(proxy_host, args.port, auth=auth)
    display_proxy_url = build_proxy_url(
        proxy_host,
        args.port,
        auth=auth,
        redact_password=True,
    )

    try:
        _start_tinyproxy(
            image=args.image,
            container_name=args.container_name,
            host_port=args.port,
            acl=args.acl,
            auth=auth,
        )
        _wait_for_port("127.0.0.1", args.port)
        proxy_env = build_proxy_env(
            os.environ,
            proxy_url=proxy_url,
            proxy_host=proxy_host,
            no_proxy_extra=args.no_proxy_extra,
        )
        print(f"[proxy-e2e] proxy URL: {display_proxy_url}")
        print(f"[proxy-e2e] NO_PROXY: {proxy_env['NO_PROXY']}")

        if not args.skip_probe:
            _probe_proxy(build_proxy_url("127.0.0.1", args.port, auth=auth))
            print(f"[proxy-e2e] outbound probe OK: {CHECK_URL}")

        if args.check_only:
            return 0

        if not args.skip_java_proxy_proof:
            _run_java_proxy_proof(
                repo_root=repo_root,
                no_proxy=proxy_env.get("NO_PROXY", ""),
                image=args.image,
                auth=auth,
            )

        _print_command("run", command)
        result = subprocess.run(command, cwd=repo_root, env=proxy_env, check=False)
        return result.returncode
    finally:
        if not args.keep_proxy:
            _remove_container(args.container_name)


if __name__ == "__main__":
    raise SystemExit(main())
