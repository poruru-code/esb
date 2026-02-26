"""CLI parser for pre-building bundle images."""

from __future__ import annotations

import argparse
from pathlib import Path

from tools.deployops.core.artifact_manifest import load_artifact_manifest_from_dir
from tools.deployops.core.bundle_ops import BundleOptions, execute_prepare_images
from tools.deployops.core.discovery import (
    derive_env_hint_from_env_file,
    resolve_artifact_dirs,
    resolve_env_file_path,
)
from tools.deployops.core.runner import CommandRunner


def register_parser(subparsers: argparse._SubParsersAction[argparse.ArgumentParser]) -> None:
    parser = subparsers.add_parser(
        "prepare-images",
        help="Prepare compose/function images required by artifact bundle inputs",
    )
    parser.add_argument(
        "-a",
        "--artifact-dir",
        action="append",
        help=("Artifact directory containing artifact.yml (repeatable). Omit to auto-discover."),
    )
    parser.add_argument(
        "-e",
        "--env-file",
        help="Path to env file. Omit to auto-resolve from artifact/env conventions.",
    )
    parser.add_argument("-c", "--compose-file", help="Path to compose file (optional)")
    parser.add_argument(
        "--build-dir",
        default="tools/deployops/.build/dind",
        help="Build context directory path",
    )
    parser.set_defaults(func=run)


def run(args: argparse.Namespace, runner: CommandRunner) -> int:
    project_root = Path.cwd().resolve()
    env_hint = derive_env_hint_from_env_file(args.env_file)
    artifact_dirs = resolve_artifact_dirs(
        project_root=project_root,
        artifact_dirs=list(args.artifact_dir or []),
        env_hint=env_hint,
    )

    manifest_env_hint = load_artifact_manifest_from_dir(artifact_dirs[0]).env
    env_file = resolve_env_file_path(
        project_root=project_root,
        env_file=args.env_file,
        env_hint=manifest_env_hint,
        required=True,
    )
    assert env_file is not None

    options = BundleOptions(
        artifact_dirs=[str(path) for path in artifact_dirs],
        env_file=str(env_file),
        compose_file=args.compose_file,
        prepare_images=True,
        output_tag=None,
        positional_tag=None,
        build_dir=args.build_dir,
    )
    return execute_prepare_images(options, runner)
