#!/usr/bin/env python3
# Where: tools/ci/generate_cyclonedx_sbom.py
# What: Generate CycloneDX 1.5 SBOM files for production project manifests.
# Why: Provide reproducible multi-ecosystem SBOM artifacts for CI and release flows.

from __future__ import annotations

import argparse
import json
import os
import shlex
import subprocess
import sys
from dataclasses import asdict, dataclass
from datetime import datetime, timezone
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_TOOL_VERSIONS_FILE = REPO_ROOT / "tools/ci/sbom-tool-versions.env"
EXCLUDED_PREFIXES = (".esb", "e2e/fixtures")


@dataclass(frozen=True)
class GeneratedSBOM:
    ecosystem: str
    source: str
    output: str
    command: list[str]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Generate CycloneDX 1.5 SBOM files for Python/Go/Java production manifests."
    )
    parser.add_argument(
        "--output-dir",
        required=True,
        type=Path,
        help="Directory for generated SBOMs",
    )
    parser.add_argument(
        "--schema-version",
        default="1.5",
        help="CycloneDX schema version. This script currently supports 1.5 only.",
    )
    parser.add_argument(
        "--strict",
        action="store_true",
        help="Stop immediately when any generation or validation step fails.",
    )
    parser.add_argument(
        "--tool-versions-file",
        type=Path,
        default=DEFAULT_TOOL_VERSIONS_FILE,
        help="Path to KEY=VALUE file containing SBOM tool versions.",
    )
    parser.add_argument(
        "--maven-plugin-version",
        default="",
        help="Override CycloneDX Maven plugin version (default: env or tool versions file).",
    )
    return parser.parse_args()


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


def clean_output_dir(output_dir: Path) -> None:
    for old_sbom in output_dir.glob("*.cdx.json"):
        old_sbom.unlink()
    index_path = output_dir / "index.json"
    if index_path.exists():
        index_path.unlink()


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


def resolve_maven_plugin_version(args: argparse.Namespace) -> str:
    if args.maven_plugin_version:
        return args.maven_plugin_version

    plugin_from_env = os.environ.get("CYCLONEDX_MAVEN_PLUGIN_VERSION", "").strip()
    if plugin_from_env:
        return plugin_from_env

    versions = read_versions_file(args.tool_versions_file)
    plugin_from_file = versions.get("CYCLONEDX_MAVEN_PLUGIN_VERSION", "").strip()
    if plugin_from_file:
        return plugin_from_file

    raise RuntimeError(
        "CycloneDX Maven plugin version is missing. "
        "Set CYCLONEDX_MAVEN_PLUGIN_VERSION or update tools/ci/sbom-tool-versions.env."
    )


def generate_python_sboms(output_dir: Path) -> list[GeneratedSBOM]:
    generated: list[GeneratedSBOM] = []
    for project_dir in discover_python_projects():
        output_file = output_dir / f"python-{slug_for(project_dir)}.cdx.json"
        command = [
            "uv",
            "export",
            "--project",
            str(project_dir),
            "--format",
            "cyclonedx1.5",
            "--preview-features",
            "sbom-export",
            "--frozen",
            "--no-dev",
            "--output-file",
            str(output_file),
        ]
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


def write_index(output_dir: Path, schema_version: str, items: list[GeneratedSBOM]) -> Path:
    index_path = output_dir / "index.json"
    payload = {
        "generatedAt": datetime.now(tz=timezone.utc).isoformat(),
        "schemaVersion": schema_version,
        "repositoryRoot": str(REPO_ROOT),
        "artifacts": [asdict(item) for item in items],
    }
    index_path.write_text(json.dumps(payload, ensure_ascii=True, indent=2) + "\n", encoding="utf-8")
    return index_path


def main() -> int:
    args = parse_args()
    if args.schema_version != "1.5":
        log(f"unsupported schema version: {args.schema_version}. only 1.5 is supported")
        return 2

    output_dir = args.output_dir.resolve()
    output_dir.mkdir(parents=True, exist_ok=True)
    clean_output_dir(output_dir)
    maven_plugin_version = resolve_maven_plugin_version(args)

    generated: list[GeneratedSBOM] = []
    generation_errors: list[str] = []
    generators = (
        ("python", lambda: generate_python_sboms(output_dir)),
        ("go", lambda: generate_go_sboms(output_dir)),
        ("java", lambda: generate_java_sbom(output_dir, maven_plugin_version)),
    )
    for ecosystem, generator in generators:
        try:
            generated.extend(generator())
        except RuntimeError as exc:
            message = f"{ecosystem} generation failed: {exc}"
            if args.strict:
                log(message)
                return 1
            generation_errors.append(message)

    for error in generation_errors:
        log(error)

    if not generated:
        log("no SBOM artifacts were generated")
        return 1 if args.strict else 0

    validation_errors: list[str] = []
    for item in generated:
        errors = validate_sbom(output_dir / item.output, args.schema_version)
        validation_errors.extend(errors)

    if validation_errors:
        for error in validation_errors:
            log(f"validation error: {error}")
        return 1 if args.strict else 0

    index_path = write_index(output_dir, args.schema_version, generated)
    log(f"generated {len(generated)} SBOM files and index: {index_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
