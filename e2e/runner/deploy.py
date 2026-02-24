# Where: e2e/runner/deploy.py
# What: Deployment execution for E2E environments.
# Why: Keep deploy logic separate from lifecycle and test orchestration.
from __future__ import annotations

import base64
import re
import threading
import urllib.parse
from pathlib import Path
from typing import Callable
from xml.sax.saxutils import escape as xml_escape

import yaml

from e2e.runner.logging import LogSink, run_and_stream
from e2e.runner.models import RunContext
from e2e.runner.utils import PROJECT_ROOT

LOCAL_IMAGE_FIXTURE_ROOT = PROJECT_ROOT / "e2e" / "fixtures" / "images" / "lambda"
LOCAL_IMAGE_FIXTURES: dict[str, Path] = {
    "esb-e2e-image-python": LOCAL_IMAGE_FIXTURE_ROOT / "python",
    "esb-e2e-image-java": LOCAL_IMAGE_FIXTURE_ROOT / "java",
}
_PROXY_ENV_ALIASES: tuple[tuple[str, str], ...] = (
    ("HTTP_PROXY", "http_proxy"),
    ("HTTPS_PROXY", "https_proxy"),
    ("NO_PROXY", "no_proxy"),
)
_MAVEN_SETTINGS_B64_BUILD_ARG = "ESB_MAVEN_SETTINGS_XML_B64"

_prepared_local_fixture_images: set[str] = set()
_prepared_local_fixture_lock = threading.Lock()
_DOCKERFILE_FROM_PATTERN = re.compile(
    r"^FROM(?:\s+--platform=[^\s]+)?\s+(?P<source>[^\s]+)",
    re.IGNORECASE,
)


def _artifactctl_bin(ctx: RunContext) -> str:
    # run_tests.py resolves ARTIFACTCTL_BIN(_RESOLVED) before runner starts.
    resolved = str(ctx.deploy_env.get("ARTIFACTCTL_BIN_RESOLVED", "")).strip()
    if resolved:
        return resolved
    configured = str(ctx.deploy_env.get("ARTIFACTCTL_BIN", "")).strip()
    if configured:
        return configured
    return "artifactctl"


def deploy_artifacts(
    ctx: RunContext,
    *,
    no_cache: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    _prepare_local_fixture_images(ctx, log=log, printer=printer)
    _deploy_via_artifact_driver(
        ctx,
        no_cache=no_cache,
        log=log,
        printer=printer,
    )


def _deploy_via_artifact_driver(
    ctx: RunContext,
    *,
    no_cache: bool,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    manifest_path = _resolve_artifact_manifest_path(ctx)
    if not manifest_path.exists():
        raise FileNotFoundError(f"artifact manifest not found: {manifest_path}")

    config_dir = str(ctx.runtime_env.get("CONFIG_DIR", "")).strip()
    if config_dir == "":
        raise RuntimeError("CONFIG_DIR is required for artifact apply")

    message = f"Deploying artifact manifest for {ctx.scenario.env_name}..."
    log.write_line(message)
    if printer:
        printer(message)
    artifactctl_bin = _artifactctl_bin(ctx)
    deploy_cmd = [
        artifactctl_bin,
        "deploy",
        "--artifact",
        str(manifest_path),
        "--out",
        config_dir,
    ]
    if no_cache:
        deploy_cmd.append("--no-cache")
    secret_env = str(ctx.scenario.extra.get("secret_env_file", "")).strip()
    if secret_env:
        deploy_cmd.extend(["--secret-env", secret_env])
    rc = run_and_stream(
        deploy_cmd,
        cwd=PROJECT_ROOT,
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )
    if rc != 0:
        raise RuntimeError(f"artifact deploy failed with exit code {rc}")

    provision_cmd = [
        artifactctl_bin,
        "provision",
        "--project",
        ctx.compose_project,
        "--compose-file",
        str(ctx.compose_file),
    ]
    if ctx.env_file:
        provision_cmd.extend(["--env-file", ctx.env_file])
    rc = run_and_stream(
        provision_cmd,
        cwd=PROJECT_ROOT,
        env=ctx.deploy_env,
        log=log,
        printer=printer,
    )
    if rc != 0:
        raise RuntimeError(f"provisioner failed with exit code {rc}")

    log.write_line("Done")
    if printer:
        printer("Done")


def _resolve_artifact_manifest_path(ctx: RunContext) -> Path:
    manifest_value = ctx.scenario.extra.get("artifact_manifest")
    if manifest_value is None:
        raise ValueError(f"artifact_manifest is required for scenario '{ctx.scenario.env_name}'")
    raw = str(manifest_value).strip()
    if raw == "":
        raise ValueError(f"artifact_manifest is required for scenario '{ctx.scenario.env_name}'")
    path = Path(raw)
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    return path.resolve()


def _prepare_local_fixture_images(
    ctx: RunContext,
    *,
    log: LogSink,
    printer: Callable[[str], None] | None = None,
) -> None:
    manifest_path = _resolve_artifact_manifest_path(ctx)
    if not manifest_path.exists():
        return
    sources = _collect_local_fixture_image_sources(manifest_path)
    if not sources:
        return

    for source in sources:
        with _prepared_local_fixture_lock:
            if source in _prepared_local_fixture_images:
                continue
            fixture_name = _fixture_repo_name(source)
            fixture_dir = LOCAL_IMAGE_FIXTURES.get(fixture_name)
            if fixture_dir is None:
                raise RuntimeError(f"Unknown local fixture image source: {source}")
            if not fixture_dir.exists():
                raise FileNotFoundError(f"Local fixture image source not found: {fixture_dir}")
            message = f"Preparing local image fixture: {source}"
            log.write_line(message)
            if printer:
                printer(message)

            build_cmd = [
                "docker",
                "buildx",
                "build",
                "--platform",
                "linux/amd64",
                "--load",
            ]
            build_cmd = _append_proxy_build_args(build_cmd, ctx.deploy_env)
            maven_settings_b64 = _maven_settings_b64_for_fixture(fixture_name, ctx.deploy_env)
            if maven_settings_b64:
                build_cmd.extend(
                    [
                        "--build-arg",
                        f"{_MAVEN_SETTINGS_B64_BUILD_ARG}={maven_settings_b64}",
                    ]
                )
            build_cmd.extend(
                [
                    "--tag",
                    source,
                    str(fixture_dir),
                ]
            )
            rc = run_and_stream(
                build_cmd,
                cwd=PROJECT_ROOT,
                env=ctx.deploy_env,
                log=log,
                printer=printer,
            )
            if rc != 0:
                raise RuntimeError(f"failed to build local fixture image {source} (exit code {rc})")

            push_cmd = ["docker", "push", source]
            rc = run_and_stream(
                push_cmd,
                cwd=PROJECT_ROOT,
                env=ctx.deploy_env,
                log=log,
                printer=printer,
            )
            if rc != 0:
                raise RuntimeError(f"failed to push local fixture image {source} (exit code {rc})")

            _prepared_local_fixture_images.add(source)


def _append_proxy_build_args(cmd: list[str], env: dict[str, str]) -> list[str]:
    for upper, lower in _PROXY_ENV_ALIASES:
        value = env.get(upper, "").strip() or env.get(lower, "").strip()
        if value == "":
            continue
        cmd.extend(["--build-arg", f"{upper}={value}"])
        cmd.extend(["--build-arg", f"{lower}={value}"])
    return cmd


def _maven_settings_b64_for_fixture(fixture_name: str, env: dict[str, str]) -> str:
    if fixture_name != "esb-e2e-image-java":
        return ""

    https_proxy = env.get("HTTPS_PROXY", "").strip() or env.get("https_proxy", "").strip()
    http_proxy = env.get("HTTP_PROXY", "").strip() or env.get("http_proxy", "").strip()
    proxy_url = https_proxy or http_proxy
    if not proxy_url:
        return ""

    no_proxy = env.get("NO_PROXY", "").strip() or env.get("no_proxy", "").strip()
    settings_xml = _render_maven_proxy_settings(proxy_url, no_proxy)
    return base64.b64encode(settings_xml.encode("utf-8")).decode("ascii")


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
    for token in no_proxy_value.replace(";", ",").split(","):
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


def _collect_local_fixture_image_sources(manifest_path: Path) -> list[str]:
    with manifest_path.open("r", encoding="utf-8") as handle:
        payload = yaml.safe_load(handle) or {}
    if not isinstance(payload, dict):
        raise ValueError(f"artifact manifest must be a map: {manifest_path}")

    artifacts = payload.get("artifacts")
    if artifacts is None:
        return []
    if not isinstance(artifacts, list):
        raise ValueError(f"artifact manifest field 'artifacts' must be a list: {manifest_path}")

    sources: set[str] = set()
    for artifact in artifacts:
        if not isinstance(artifact, dict):
            raise ValueError(f"artifact entries must be maps: {manifest_path}")
        artifact_root_raw = str(artifact.get("artifact_root", "")).strip()
        if artifact_root_raw == "":
            continue
        artifact_root = Path(artifact_root_raw)
        if not artifact_root.is_absolute():
            artifact_root = manifest_path.parent / artifact_root
        artifact_root = artifact_root.resolve()
        if not artifact_root.exists():
            raise FileNotFoundError(f"artifact_root not found: {artifact_root}")
        sources.update(_collect_local_fixture_sources_from_artifact_root(artifact_root))
    return sorted(sources)


def _collect_local_fixture_sources_from_artifact_root(artifact_root: Path) -> set[str]:
    sources: set[str] = set()
    for dockerfile in sorted(artifact_root.rglob("Dockerfile")):
        sources.update(_collect_local_fixture_sources_from_dockerfile(dockerfile))
    return sources


def _collect_local_fixture_sources_from_dockerfile(dockerfile: Path) -> set[str]:
    sources: set[str] = set()
    content = dockerfile.read_text(encoding="utf-8")
    for line in content.splitlines():
        match = _DOCKERFILE_FROM_PATTERN.match(line.strip())
        if match is None:
            continue
        source = match.group("source").strip()
        if _is_local_fixture_image_source(source):
            sources.add(source)
    return sources


def _is_local_fixture_image_source(source: str) -> bool:
    if not source:
        return False
    return _fixture_repo_name(source) in LOCAL_IMAGE_FIXTURES


def _fixture_repo_name(source: str) -> str:
    without_digest = source.split("@", 1)[0]
    last_segment = without_digest.rsplit("/", 1)[-1]
    return last_segment.split(":", 1)[0]
