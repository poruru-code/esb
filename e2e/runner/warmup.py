# Where: e2e/runner/warmup.py
# What: Template scanning and Java fixture warmup helpers for E2E runs.
# Why: Keep warmup concerns separate from test orchestration flow.
from __future__ import annotations

import os
import subprocess
import tempfile
import urllib.parse
from dataclasses import dataclass
from pathlib import Path
from typing import Callable
from xml.sax.saxutils import escape as xml_escape

import yaml

from e2e.runner import constants
from e2e.runner.buildx import ensure_buildx_builder
from e2e.runner.env import apply_proxy_defaults, calculate_runtime_env
from e2e.runner.models import Scenario
from e2e.runner.utils import BRAND_HOME_DIR, BRAND_SLUG, PROJECT_ROOT, default_e2e_deploy_templates

M2_SETTINGS_PATH = "/tmp/m2/settings.xml"
M2_REPOSITORY_PATH = "/tmp/m2/repository"
JAVA_BUILD_IMAGE = "public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7"
JAVA_FIXTURE_ROOTS = (
    PROJECT_ROOT / "e2e" / "fixtures" / "functions" / "java",
    PROJECT_ROOT / "tools" / "e2e-lambda-fixtures" / "java",
)
_PROXY_ENV_KEYS = (
    "HTTP_PROXY",
    "http_proxy",
    "HTTPS_PROXY",
    "https_proxy",
    "NO_PROXY",
    "no_proxy",
)


@dataclass(frozen=True)
class _ProxyEndpoint:
    host: str
    port: int
    username: str
    password: str


def _first_configured_env(*keys: str) -> str | None:
    for key in keys:
        value = os.environ.get(key, "").strip()
        if value:
            return value
    return None


def _validate_proxy_url(env_label: str, raw_url: str) -> None:
    try:
        parsed = urllib.parse.urlsplit(raw_url.strip())
    except ValueError as exc:  # pragma: no cover - defensive for malformed URLs
        raise ValueError(f"{env_label} is invalid: {raw_url}") from exc

    if not parsed.scheme or not parsed.hostname:
        raise ValueError(f"{env_label} must include scheme and host: {raw_url}")
    if parsed.scheme.lower() not in {"http", "https"}:
        raise ValueError(f"{env_label} must use http or https scheme: {raw_url}")
    if parsed.query or parsed.fragment or parsed.path not in {"", "/"}:
        raise ValueError(f"{env_label} must not include path/query/fragment: {raw_url}")

    try:
        port = parsed.port
    except ValueError as exc:
        raise ValueError(f"{env_label} has invalid port: {raw_url}") from exc
    if port is None:
        port = 80 if parsed.scheme.lower() == "http" else 443
    if port < 1 or port > 65535:
        raise ValueError(f"{env_label} has invalid port: {raw_url}")

    _ = urllib.parse.unquote(parsed.username or "")
    _ = urllib.parse.unquote(parsed.password or "")


def _parse_proxy_endpoint(env_label: str, raw_url: str) -> _ProxyEndpoint:
    _validate_proxy_url(env_label, raw_url)
    parsed = urllib.parse.urlsplit(raw_url.strip())
    scheme = parsed.scheme.lower()
    port = parsed.port if parsed.port is not None else (80 if scheme == "http" else 443)
    return _ProxyEndpoint(
        host=parsed.hostname or "",
        port=port,
        username=urllib.parse.unquote(parsed.username or ""),
        password=urllib.parse.unquote(parsed.password or ""),
    )


def _normalize_non_proxy_token(token: str) -> str:
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


def _maven_non_proxy_hosts() -> str:
    raw_value = _first_configured_env("NO_PROXY", "no_proxy")
    if not raw_value:
        return ""

    seen: set[str] = set()
    normalized_tokens: list[str] = []
    for chunk in raw_value.replace(";", ",").split(","):
        normalized = _normalize_non_proxy_token(chunk)
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        normalized_tokens.append(normalized)
    return "|".join(normalized_tokens)


def _render_proxy_block(
    *,
    proxy_id: str,
    protocol: str,
    endpoint: _ProxyEndpoint,
    non_proxy_hosts: str,
) -> list[str]:
    lines = [
        "    <proxy>",
        f"      <id>{xml_escape(proxy_id)}</id>",
        "      <active>true</active>",
        f"      <protocol>{xml_escape(protocol)}</protocol>",
        f"      <host>{xml_escape(endpoint.host)}</host>",
        f"      <port>{endpoint.port}</port>",
    ]
    if endpoint.username:
        lines.append(f"      <username>{xml_escape(endpoint.username)}</username>")
    if endpoint.password:
        lines.append(f"      <password>{xml_escape(endpoint.password)}</password>")
    if non_proxy_hosts:
        lines.append(f"      <nonProxyHosts>{xml_escape(non_proxy_hosts)}</nonProxyHosts>")
    lines.append("    </proxy>")
    return lines


def _render_maven_settings_xml() -> str:
    http_raw = _first_configured_env("HTTP_PROXY", "http_proxy")
    https_raw = _first_configured_env("HTTPS_PROXY", "https_proxy")

    http_endpoint = _parse_proxy_endpoint("HTTP_PROXY/http_proxy", http_raw) if http_raw else None
    https_endpoint = (
        _parse_proxy_endpoint("HTTPS_PROXY/https_proxy", https_raw) if https_raw else None
    )
    if https_endpoint is None and http_endpoint is not None:
        https_endpoint = http_endpoint

    non_proxy_hosts = _maven_non_proxy_hosts()
    lines = [
        "<settings>",
        "  <proxies>",
    ]
    if http_endpoint is not None:
        lines.extend(
            _render_proxy_block(
                proxy_id="http-proxy",
                protocol="http",
                endpoint=http_endpoint,
                non_proxy_hosts=non_proxy_hosts,
            )
        )
    if https_endpoint is not None:
        lines.extend(
            _render_proxy_block(
                proxy_id="https-proxy",
                protocol="https",
                endpoint=https_endpoint,
                non_proxy_hosts=non_proxy_hosts,
            )
        )
    lines.extend(
        [
            "  </proxies>",
            "</settings>",
        ]
    )
    return "\n".join(lines) + "\n"


def _write_temp_maven_settings() -> Path:
    settings_xml = _render_maven_settings_xml()
    with tempfile.NamedTemporaryFile(
        mode="w",
        encoding="utf-8",
        suffix="-esb-m2-settings.xml",
        delete=False,
    ) as temp_file:
        temp_file.write(settings_xml)
        return Path(temp_file.name)


def _warmup(
    scenarios: dict[str, Scenario],
    *,
    printer: Callable[[str], None] | None = None,
    verbose: bool = False,
) -> None:
    templates = _collect_templates(scenarios)
    missing_templates = [template for template in templates if not template.exists()]
    if missing_templates:
        missing = ", ".join(str(template) for template in missing_templates)
        raise FileNotFoundError(f"Missing E2E template(s): {missing}")
    _ensure_buildx_builders(scenarios)
    if _uses_java_templates(scenarios):
        _emit_warmup(printer, "Java fixture warmup ... start")
        _build_java_fixtures(printer=printer, verbose=verbose)
        _emit_warmup(printer, "Java fixture warmup ... done")


def _uses_java_templates(scenarios: dict[str, Scenario]) -> bool:
    java_hooks_dir = PROJECT_ROOT / "runtime-hooks" / "java"
    if not java_hooks_dir.exists():
        return False
    for template in _collect_templates(scenarios):
        if _template_has_java_runtime(template):
            return True
    return False


def _collect_templates(scenarios: dict[str, Scenario]) -> list[Path]:
    templates: set[Path] = set()
    for scenario in scenarios.values():
        templates.update(_resolve_templates(scenario))
    return sorted(templates)


def _resolve_templates(scenario: Scenario) -> list[Path]:
    if scenario.deploy_templates:
        return [_resolve_template_path(Path(template)) for template in scenario.deploy_templates]
    return default_e2e_deploy_templates()


def _resolve_env_file(env_file: str | None) -> str | None:
    if not env_file:
        return None
    path = Path(env_file)
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    return str(path.absolute())


def _scenario_runtime_env_for_buildx(scenario: Scenario) -> dict[str, str]:
    templates = _resolve_templates(scenario)
    template_path = str(templates[0]) if templates else None
    runtime_env = calculate_runtime_env(
        scenario.project_name or BRAND_SLUG,
        scenario.env_name,
        scenario.mode,
        _resolve_env_file(scenario.env_file),
        template_path=template_path,
    )
    runtime_env.update(scenario.env_vars)
    apply_proxy_defaults(runtime_env)
    return runtime_env


def _ensure_buildx_builders(scenarios: dict[str, Scenario]) -> None:
    seen: set[tuple[str, str, tuple[tuple[str, str], ...]]] = set()
    for scenario in scenarios.values():
        runtime_env = _scenario_runtime_env_for_buildx(scenario)
        builder_name = runtime_env.get("BUILDX_BUILDER", "").strip()
        if not builder_name:
            continue
        config_path = runtime_env.get(constants.ENV_BUILDKITD_CONFIG, "").strip()
        proxy_signature = tuple((key, runtime_env.get(key, "").strip()) for key in _PROXY_ENV_KEYS)
        signature = (builder_name, config_path, proxy_signature)
        if signature in seen:
            continue
        seen.add(signature)
        ensure_buildx_builder(
            builder_name,
            config_path=config_path,
            proxy_source=runtime_env,
        )


def _resolve_template_path(path: Path) -> Path:
    if path.is_absolute():
        return path
    return (PROJECT_ROOT / path).resolve()


def _template_has_java_runtime(path: Path) -> bool:
    if not path.exists():
        return False
    try:
        data = yaml.load(path.read_text(encoding="utf-8"), Loader=_YamlIgnoreTagsLoader)
    except (OSError, yaml.YAMLError):
        return False
    if not isinstance(data, dict):
        return False
    if _globals_runtime_is_java(data):
        return True
    resources = data.get("Resources")
    if not isinstance(resources, dict):
        return False
    for resource in resources.values():
        if not isinstance(resource, dict):
            continue
        props = resource.get("Properties")
        if not isinstance(props, dict):
            continue
        runtime = str(props.get("Runtime", "")).lower().strip()
        if runtime.startswith("java"):
            return True
        code_uri = props.get("CodeUri", "")
        if isinstance(code_uri, str):
            if "functions/java/" in code_uri or code_uri.lower().endswith(".jar"):
                return True
    return False


def _globals_runtime_is_java(payload: dict) -> bool:
    globals_section = payload.get("Globals")
    if not isinstance(globals_section, dict):
        return False
    function_globals = globals_section.get("Function")
    if not isinstance(function_globals, dict):
        return False
    runtime = str(function_globals.get("Runtime", "")).lower().strip()
    return runtime.startswith("java")


class _YamlIgnoreTagsLoader(yaml.SafeLoader):
    pass


def _yaml_ignore_unknown_tags(loader, node):
    if isinstance(node, yaml.ScalarNode):
        return loader.construct_scalar(node)
    if isinstance(node, yaml.SequenceNode):
        return loader.construct_sequence(node)
    if isinstance(node, yaml.MappingNode):
        return loader.construct_mapping(node)
    return None


_YamlIgnoreTagsLoader.add_constructor(None, _yaml_ignore_unknown_tags)


def _build_java_fixtures(
    *,
    printer: Callable[[str], None] | None = None,
    verbose: bool = False,
) -> None:
    for project_dir in _discover_java_fixture_projects():
        if printer and verbose:
            printer(f"Building Java fixture: {project_dir.name}")
        _build_java_project(project_dir, verbose=verbose)


def _discover_java_fixture_projects() -> list[Path]:
    projects: list[Path] = []
    seen: set[Path] = set()
    for root in JAVA_FIXTURE_ROOTS:
        if not root.exists():
            continue
        if (root / "pom.xml").exists():
            resolved = root.resolve()
            if resolved not in seen:
                seen.add(resolved)
                projects.append(resolved)
            continue
        for project_dir in sorted(p for p in root.iterdir() if p.is_dir()):
            if not (project_dir / "pom.xml").exists():
                continue
            resolved = project_dir.resolve()
            if resolved in seen:
                continue
            seen.add(resolved)
            projects.append(resolved)
    return projects


def _build_java_project(project_dir: Path, *, verbose: bool = False) -> None:
    try:
        settings_path = _write_temp_maven_settings()
    except ValueError as exc:
        raise RuntimeError(f"Invalid proxy configuration for Java fixture build: {exc}") from exc
    try:
        cmd = _docker_maven_command(project_dir, settings_path, verbose=verbose)
        result = subprocess.run(
            cmd,
            capture_output=not verbose,
            text=True,
            check=False,
        )
        if result.returncode != 0:
            details = ""
            if not verbose:
                details = f"\n{result.stdout}\n{result.stderr}".rstrip()
            raise RuntimeError(f"Java fixture build failed: {project_dir}{details}")
    finally:
        settings_path.unlink(missing_ok=True)

    jar_path = project_dir / "app.jar"
    if not jar_path.exists():
        raise RuntimeError(f"Java fixture jar not found in {project_dir}")


def _java_proxy_env_overrides() -> list[tuple[str, str]]:
    return [
        ("HTTP_PROXY", ""),
        ("http_proxy", ""),
        ("HTTPS_PROXY", ""),
        ("https_proxy", ""),
        ("NO_PROXY", ""),
        ("no_proxy", ""),
        ("MAVEN_OPTS", ""),
        ("JAVA_TOOL_OPTIONS", ""),
    ]


def _project_maven_repo_cache_dir() -> Path:
    cache_dir = PROJECT_ROOT / BRAND_HOME_DIR / "cache" / "m2" / "repository"
    cache_dir.mkdir(parents=True, exist_ok=True)
    return cache_dir


def _docker_maven_command(
    project_dir: Path,
    settings_path: Path,
    *,
    verbose: bool = False,
) -> list[str]:
    m2_repo_cache_dir = _project_maven_repo_cache_dir()
    cmd = [
        "docker",
        "run",
        "--rm",
    ]
    getuid = getattr(os, "getuid", None)
    getgid = getattr(os, "getgid", None)
    if callable(getuid) and callable(getgid):
        cmd.extend(["--user", f"{getuid()}:{getgid()}"])
    cmd.extend(["-v", f"{project_dir}:/src:ro", "-v", f"{project_dir}:/out"])
    cmd.extend(["-v", f"{settings_path}:{M2_SETTINGS_PATH}:ro"])
    cmd.extend(["-v", f"{m2_repo_cache_dir}:{M2_REPOSITORY_PATH}"])
    cmd.extend(["-e", "MAVEN_CONFIG=/tmp/m2", "-e", "HOME=/tmp"])
    for key, value in _java_proxy_env_overrides():
        cmd.extend(["-e", f"{key}={value}"])
    maven_cmd_with_settings = (
        f"mvn -s {M2_SETTINGS_PATH} -Dmaven.repo.local={M2_REPOSITORY_PATH} "
        "-Dmaven.artifact.threads=1 -DskipTests package"
        if verbose
        else f"mvn -s {M2_SETTINGS_PATH} -q -Dmaven.repo.local={M2_REPOSITORY_PATH} "
        "-Dmaven.artifact.threads=1 -DskipTests package"
    )
    script = "\n".join(
        [
            "set -euo pipefail",
            "mkdir -p /tmp/work /out",
            "cp -a /src/. /tmp/work",
            "cd /tmp/work",
            maven_cmd_with_settings,
            "jar=$(ls -S target/*.jar 2>/dev/null | "
            "grep -vE '(-sources|-javadoc)\\.jar$' | head -n 1 || true)",
            'if [ -z "$jar" ]; then echo "jar not found in target" >&2; exit 1; fi',
            'cp "$jar" /out/app.jar',
        ]
    )
    cmd.extend(
        [
            JAVA_BUILD_IMAGE,
            "bash",
            "-c",
            script,
        ]
    )
    return cmd


def _emit_warmup(printer: Callable[[str], None] | None, message: str) -> None:
    if printer:
        printer(message)
    else:
        print(message)
