#!/usr/bin/env python3
# Where: tools/ci/sbom_support.py
# What: Shared CycloneDX SBOM helpers for discovery, command execution, and validation.
# Why: Keep the main SBOM script small and maintainable while centralizing reusable behavior.

from __future__ import annotations

import json
import shlex
import subprocess
from dataclasses import dataclass
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
EXCLUDED_PREFIXES = (".esb", "e2e/fixtures")


@dataclass(frozen=True)
class GeneratedSBOM:
    ecosystem: str
    source: str
    output: str
    command: list[str]


def log(message: str) -> None:
    print(f"[sbom] {message}")


def repo_relative(path: Path) -> str:
    relative = path.relative_to(REPO_ROOT)
    if relative == Path("."):
        return "."
    return relative.as_posix()


def slug_for(path: Path) -> str:
    relative = path.relative_to(REPO_ROOT)
    if relative == Path("."):
        return "root"
    return "-".join(relative.parts)


def is_excluded(path: Path) -> bool:
    rel = repo_relative(path)
    for prefix in EXCLUDED_PREFIXES:
        if rel == prefix or rel.startswith(f"{prefix}/"):
            return True
    return False


def read_versions_file(path: Path) -> dict[str, str]:
    versions: dict[str, str] = {}
    if not path.exists():
        return versions
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        versions[key.strip()] = value.strip()
    return versions


def discover_python_projects() -> list[Path]:
    projects: list[Path] = []
    for lock_file in REPO_ROOT.rglob("uv.lock"):
        project_dir = lock_file.parent
        if is_excluded(project_dir):
            continue
        if (project_dir / "pyproject.toml").is_file():
            projects.append(project_dir)
    return sorted(projects, key=repo_relative)


def discover_go_modules() -> list[Path]:
    modules: list[Path] = []
    for go_mod in REPO_ROOT.rglob("go.mod"):
        module_dir = go_mod.parent
        if is_excluded(module_dir):
            continue
        modules.append(module_dir)
    return sorted(modules, key=repo_relative)


def discover_java_project() -> Path | None:
    pom_path = REPO_ROOT / "runtime/java/build/pom.xml"
    if pom_path.is_file():
        return pom_path
    return None


def run_command(command: list[str], *, cwd: Path | None = None) -> None:
    where = repo_relative(cwd) if cwd is not None else "."
    rendered = " ".join(shlex.quote(part) for part in command)
    log(f"run ({where}): {rendered}")
    try:
        completed = subprocess.run(command, cwd=cwd, check=False, capture_output=True, text=True)
    except FileNotFoundError as exc:
        raise RuntimeError(f"command not found: {command[0]}") from exc
    if completed.returncode != 0:
        if completed.stdout:
            log(f"stdout:\n{completed.stdout.strip()}")
        if completed.stderr:
            log(f"stderr:\n{completed.stderr.strip()}")
        raise RuntimeError(f"command failed with exit code {completed.returncode}: {rendered}")


def validate_sbom(path: Path, schema_version: str) -> list[str]:
    errors: list[str] = []
    if not path.is_file():
        return [f"{path} does not exist"]

    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return [f"{path} is not valid JSON: {exc}"]

    if not isinstance(payload, dict):
        return [f"{path} root element must be an object"]

    if payload.get("bomFormat") != "CycloneDX":
        errors.append(f"{path} has unexpected bomFormat: {payload.get('bomFormat')!r}")
    if str(payload.get("specVersion")) != schema_version:
        errors.append(f"{path} has unexpected specVersion: {payload.get('specVersion')!r}")
    return errors


def uv_export_supports_cyclonedx(help_text: str) -> bool:
    return "cyclonedx1.5" in help_text


def assert_uv_supports_cyclonedx_export() -> None:
    command = ["uv", "export", "--help"]
    rendered = " ".join(shlex.quote(part) for part in command)
    log(f"run (.): {rendered}")
    try:
        completed = subprocess.run(command, check=False, capture_output=True, text=True)
    except FileNotFoundError as exc:
        raise RuntimeError("command not found: uv") from exc

    if completed.returncode != 0:
        if completed.stdout:
            log(f"stdout:\n{completed.stdout.strip()}")
        if completed.stderr:
            log(f"stderr:\n{completed.stderr.strip()}")
        raise RuntimeError(f"failed to inspect uv export formats: {rendered}")

    if not uv_export_supports_cyclonedx(completed.stdout):
        raise RuntimeError(
            "installed uv does not support '--format cyclonedx1.5'. "
            "Upgrade uv or pin UV_VERSION in tools/ci/sbom-tool-versions.env "
            "to a release that includes CycloneDX export."
        )


def build_python_export_command(project_dir: Path, output_file: Path) -> list[str]:
    return [
        "uv",
        "export",
        "--project",
        str(project_dir),
        "--format",
        "cyclonedx1.5",
        "--frozen",
        "--no-dev",
        "--output-file",
        str(output_file),
    ]


def generate_python_sboms(output_dir: Path) -> list[GeneratedSBOM]:
    assert_uv_supports_cyclonedx_export()
    generated: list[GeneratedSBOM] = []
    for project_dir in discover_python_projects():
        output_file = output_dir / f"python-{slug_for(project_dir)}.cdx.json"
        command = build_python_export_command(project_dir, output_file)
        run_command(command)
        generated.append(
            GeneratedSBOM(
                ecosystem="python",
                source=repo_relative(project_dir),
                output=output_file.name,
                command=command,
            )
        )
    return generated


def generate_go_sboms(output_dir: Path) -> list[GeneratedSBOM]:
    generated: list[GeneratedSBOM] = []
    for module_dir in discover_go_modules():
        output_file = output_dir / f"go-{slug_for(module_dir)}.cdx.json"
        command = [
            "cyclonedx-gomod",
            "mod",
            "-licenses",
            "-json",
            "-output-version",
            "1.5",
            "-output",
            str(output_file),
        ]
        run_command(command, cwd=module_dir)
        generated.append(
            GeneratedSBOM(
                ecosystem="go",
                source=repo_relative(module_dir),
                output=output_file.name,
                command=command,
            )
        )
    return generated


def generate_java_sbom(output_dir: Path, maven_plugin_version: str) -> list[GeneratedSBOM]:
    pom_path = discover_java_project()
    if pom_path is None:
        return []

    output_name = "java-runtime.cdx"
    output_file = output_dir / f"{output_name}.json"
    maven_local_repo = REPO_ROOT / ".esb/cache/m2/repository"
    maven_local_repo.mkdir(parents=True, exist_ok=True)
    command = [
        "mvn",
        "-B",
        "-f",
        str(pom_path),
        f"org.cyclonedx:cyclonedx-maven-plugin:{maven_plugin_version}:makeAggregateBom",
        f"-Dmaven.repo.local={maven_local_repo}",
        "-DoutputFormat=json",
        "-DschemaVersion=1.5",
        f"-DoutputDirectory={output_dir}",
        f"-DoutputName={output_name}",
    ]
    run_command(command)
    return [
        GeneratedSBOM(
            ecosystem="java",
            source=repo_relative(pom_path),
            output=output_file.name,
            command=command,
        )
    ]
