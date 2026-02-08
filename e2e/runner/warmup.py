# Where: e2e/runner/warmup.py
# What: Template scanning and Java fixture warmup helpers for E2E runs.
# Why: Keep warmup concerns separate from test orchestration flow.
from __future__ import annotations

import os
import subprocess
from pathlib import Path
from typing import Callable

import yaml

from e2e.runner.models import Scenario
from e2e.runner.utils import PROJECT_ROOT, default_e2e_deploy_templates


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
    if _uses_java_templates(scenarios):
        _emit_warmup(printer, "Java fixture warmup ... start")
        _build_java_fixtures(printer=printer, verbose=verbose)
        _emit_warmup(printer, "Java fixture warmup ... done")


def _uses_java_templates(scenarios: dict[str, Scenario]) -> bool:
    runtime_extensions = PROJECT_ROOT / "runtime" / "java" / "extensions"
    if not runtime_extensions.exists():
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
    fixtures_dir = PROJECT_ROOT / "e2e" / "fixtures" / "functions" / "java"
    if not fixtures_dir.exists():
        return

    for project_dir in sorted(p for p in fixtures_dir.iterdir() if p.is_dir()):
        pom = project_dir / "pom.xml"
        if not pom.exists():
            continue
        if printer and verbose:
            printer(f"Building Java fixture: {project_dir.name}")
        _build_java_project(project_dir, verbose=verbose)


def _build_java_project(project_dir: Path, *, verbose: bool = False) -> None:
    cmd = _docker_maven_command(project_dir, verbose=verbose)
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

    jar_path = project_dir / "app.jar"
    if not jar_path.exists():
        raise RuntimeError(f"Java fixture jar not found in {project_dir}")


def _docker_maven_command(project_dir: Path, *, verbose: bool = False) -> list[str]:
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
    home_dir = Path.home()
    m2_dir = home_dir / ".m2"
    if m2_dir.exists() and os.access(m2_dir, os.W_OK):
        cmd.extend(["-v", f"{m2_dir}:/tmp/m2"])
    cmd.extend(["-e", "MAVEN_CONFIG=/tmp/m2", "-e", "HOME=/tmp"])
    for key in (
        "HTTP_PROXY",
        "HTTPS_PROXY",
        "NO_PROXY",
        "http_proxy",
        "https_proxy",
        "no_proxy",
        "MAVEN_OPTS",
        "JAVA_TOOL_OPTIONS",
    ):
        value = os.environ.get(key)
        if value:
            cmd.extend(["-e", f"{key}={value}"])
    maven_cmd = "mvn -DskipTests package" if verbose else "mvn -q -DskipTests package"
    script = "\n".join(
        [
            "set -euo pipefail",
            "mkdir -p /tmp/work /tmp/m2 /out",
            "cp -a /src/. /tmp/work",
            "cd /tmp/work",
            maven_cmd,
            "jar=$(ls -S target/*.jar 2>/dev/null | "
            "grep -vE '(-sources|-javadoc)\\.jar$' | head -n 1 || true)",
            'if [ -z "$jar" ]; then echo "jar not found in target" >&2; exit 1; fi',
            'cp "$jar" /out/app.jar',
        ]
    )
    cmd.extend(
        [
            "maven:3.9.6-eclipse-temurin-21",
            "bash",
            "-lc",
            script,
        ]
    )
    return cmd


def _emit_warmup(printer: Callable[[str], None] | None, message: str) -> None:
    if printer:
        printer(message)
    else:
        print(message)
