"""CLI parser for DinD bundling command."""

from __future__ import annotations

import argparse
from pathlib import Path

from tools.deployops.core.artifact_manifest import load_artifact_manifest_from_dir
from tools.deployops.core.bundle_ops import BundleOptions, execute_bundle_dind
from tools.deployops.core.discovery import (
    derive_env_hint_from_env_file,
    resolve_artifact_dirs,
    resolve_env_file_path,
)
from tools.deployops.core.runner import CommandRunner


def register_parser(subparsers: argparse._SubParsersAction[argparse.ArgumentParser]) -> None:
    parser = subparsers.add_parser(
        "bundle-dind",
        help="Build a self-contained DinD bundle image from artifact directories",
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
        "--prepare-images",
        action="store_true",
        help="Build/pull missing images before bundling",
    )
    parser.add_argument(
        "--output-tag",
        help="Output image tag (overrides positional tag if both are provided)",
    )
    parser.add_argument("tag", nargs="?", help="Optional positional output image tag")
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
        prepare_images=bool(args.prepare_images),
        output_tag=args.output_tag,
        positional_tag=args.tag,
        build_dir=args.build_dir,
    )
    return execute_bundle_dind(options, runner)
