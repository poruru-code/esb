#!/usr/bin/env python3
# Where: tools/ci/generate_cyclonedx_sbom.py
# What: Entrypoint for CycloneDX 1.5 SBOM generation across Python/Go/Java.
# Why: Keep CI generation deterministic while delegating reusable logic to helper modules.

from __future__ import annotations

import argparse
import json
import os
import sys
from dataclasses import asdict
from datetime import datetime, timezone
from pathlib import Path

from sbom_support import (
    REPO_ROOT,
    GeneratedSBOM,
    generate_go_sboms,
    generate_java_sbom,
    generate_python_sboms,
    log,
    read_versions_file,
    validate_sbom,
)

DEFAULT_TOOL_VERSIONS_FILE = REPO_ROOT / "tools/ci/sbom-tool-versions.env"


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


def clean_output_dir(output_dir: Path) -> None:
    for old_sbom in output_dir.glob("*.cdx.json"):
        old_sbom.unlink()
    index_path = output_dir / "index.json"
    if index_path.exists():
        index_path.unlink()


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
