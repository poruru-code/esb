"""CLI parser for artifact apply command."""

from __future__ import annotations

import argparse
from pathlib import Path

from tools.deployops.core.apply_ops import ApplyOptions, execute_apply
from tools.deployops.core.discovery import (
    derive_env_hint_from_env_file,
    resolve_artifact_manifest_path,
)
from tools.deployops.core.runner import CommandRunner


def register_parser(subparsers: argparse._SubParsersAction[argparse.ArgumentParser]) -> None:
    parser = subparsers.add_parser(
        "apply",
        help="Apply artifact manifest and run deploy/provision on current stack",
    )
    parser.add_argument(
        "--artifact",
        help=(
            "Path to artifact manifest (artifact.yml). "
            "Omit to auto-discover from project conventions."
        ),
    )
    parser.add_argument(
        "--compose-file",
        help="Compose file path (default: <project-root>/docker-compose.yml)",
    )
    parser.add_argument(
        "--env-file",
        help="Compose env file path (default: <project-root>/.env when present)",
    )
    parser.add_argument("--ctl-bin", help="ctl binary override (or use CTL_BIN env)")
    parser.add_argument(
        "--registry-wait-timeout",
        type=int,
        help="Registry readiness timeout in seconds (default: REGISTRY_WAIT_TIMEOUT or 60)",
    )
    parser.add_argument(
        "--registry-port",
        type=int,
        help=(
            "Registry host port override (default: PORT_REGISTRY or 5010; 0 resolves from compose)"
        ),
    )
    parser.add_argument(
        "--project-dir",
        default=".",
        help="Project root directory for compose/provision context",
    )
    parser.set_defaults(func=run)


def run(args: argparse.Namespace, runner: CommandRunner) -> int:
    project_root = Path(args.project_dir).expanduser().resolve()
    env_hint = derive_env_hint_from_env_file(args.env_file)
    artifact_path = resolve_artifact_manifest_path(
        project_root=project_root,
        artifact=args.artifact,
        env_hint=env_hint,
    )

    options = ApplyOptions(
        artifact=str(artifact_path),
        compose_file=args.compose_file,
        env_file=args.env_file,
        ctl_bin=args.ctl_bin,
        registry_wait_timeout=args.registry_wait_timeout,
        registry_port=args.registry_port,
        project_dir=args.project_dir,
    )
    return execute_apply(options, runner)
