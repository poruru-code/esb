from __future__ import annotations

import argparse
import json
import sys
from dataclasses import dataclass
from typing import NoReturn

from e2e.runner.branding_constants_gen import DEFAULT_CTL_BIN
from tools.cli.deploy_ops import (
    DeployInput,
    ProvisionInput,
    execute_deploy,
    execute_provision,
)
from tools.cli.fixture_image import (
    FixtureImageEnsureInput,
    execute_fixture_image_ensure,
)
from tools.cli.maven_shim import EnsureInput as MavenShimEnsureInput
from tools.cli.maven_shim import ensure_image as ensure_maven_shim_image

_CTL_CAPABILITIES_SCHEMA_VERSION = 1
_MAVEN_SHIM_ENSURE_SCHEMA_VERSION = 1
_FIXTURE_IMAGE_ENSURE_SCHEMA_VERSION = 1


@dataclass(frozen=True)
class ArtifactctlContractVersions:
    maven_shim_ensure_schema_version: int
    fixture_image_ensure_schema_version: int


@dataclass(frozen=True)
class ArtifactctlCapabilities:
    schema_version: int
    contracts: ArtifactctlContractVersions

    def to_json(self) -> str:
        return json.dumps(
            {
                "schema_version": self.schema_version,
                "contracts": {
                    "maven_shim_ensure_schema_version": (
                        self.contracts.maven_shim_ensure_schema_version
                    ),
                    "fixture_image_ensure_schema_version": (
                        self.contracts.fixture_image_ensure_schema_version
                    ),
                },
            }
        )


def current_capabilities() -> ArtifactctlCapabilities:
    return ArtifactctlCapabilities(
        schema_version=_CTL_CAPABILITIES_SCHEMA_VERSION,
        contracts=ArtifactctlContractVersions(
            maven_shim_ensure_schema_version=_MAVEN_SHIM_ENSURE_SCHEMA_VERSION,
            fixture_image_ensure_schema_version=_FIXTURE_IMAGE_ENSURE_SCHEMA_VERSION,
        ),
    )


def command_text(*parts: str) -> str:
    return " ".join((DEFAULT_CTL_BIN, *parts))


def hint_run(*parts: str) -> str:
    return f"Hint: run `{command_text(*parts)}`."


def parse_error_hint() -> str:
    return (
        f"Hint: run `{command_text('--help')}`, `{command_text('deploy', '--help')}`, "
        f"or `{command_text('provision', '--help')}`."
    )


def _exit_parser_error(message: str) -> NoReturn:
    print(f"Error: {message}", file=sys.stderr)
    print(parse_error_hint(), file=sys.stderr)
    raise SystemExit(1)


class _CtlArgumentParser(argparse.ArgumentParser):
    def error(self, message: str) -> NoReturn:
        _exit_parser_error(message)


def build_parser() -> argparse.ArgumentParser:
    parser = _CtlArgumentParser(
        prog=DEFAULT_CTL_BIN,
        description="Prepare images and apply generated artifact manifests.",
    )
    subparsers = parser.add_subparsers(dest="command")

    deploy = subparsers.add_parser(
        "deploy",
        help="Prepare images and apply artifact manifest",
    )
    deploy.add_argument(
        "--artifact",
        required=True,
        help="Path to artifact manifest (artifact.yml)",
    )
    deploy.add_argument(
        "--no-cache",
        action="store_true",
        help="Do not use cache when building images",
    )

    provision = subparsers.add_parser(
        "provision",
        help="Run deploy provisioner via docker compose",
    )
    provision.add_argument("--project", required=True, help="Compose project name")
    provision.add_argument(
        "--compose-file",
        required=True,
        action="append",
        help="Compose file(s) to use (repeatable or comma-separated)",
    )
    provision.add_argument("--env-file", default="", help="Path to compose env file")
    provision.add_argument(
        "--project-dir",
        default="",
        help="Working directory for docker compose (default: current directory)",
    )
    provision.add_argument(
        "--with-deps",
        action="store_true",
        help="Start dependent services when running provisioner",
    )
    provision.add_argument("-v", "--verbose", action="store_true", help="Verbose output")

    internal = subparsers.add_parser("internal", help="Internal commands for orchestrators")
    internal_subparsers = internal.add_subparsers(dest="internal_command")

    maven_shim = internal_subparsers.add_parser(
        "maven-shim",
        help="Maven shim helper operations",
    )
    maven_shim_subparsers = maven_shim.add_subparsers(dest="maven_shim_command")
    maven_shim_ensure = maven_shim_subparsers.add_parser(
        "ensure",
        help="Ensure a Maven shim image and print JSON payload",
    )
    maven_shim_ensure.add_argument(
        "--base-image",
        required=True,
        help="Base Maven image reference used for shim derivation",
    )
    maven_shim_ensure.add_argument(
        "--host-registry",
        default="",
        help="Host registry prefix (for pushable shim reference)",
    )
    maven_shim_ensure.add_argument(
        "--no-cache",
        action="store_true",
        help="Do not use local cache",
    )
    maven_shim_ensure.add_argument(
        "--output",
        choices=("json",),
        default="json",
        help="Output format",
    )

    fixture_image = internal_subparsers.add_parser(
        "fixture-image",
        help="Local fixture image helper operations",
    )
    fixture_image_subparsers = fixture_image.add_subparsers(dest="fixture_image_command")
    fixture_image_ensure = fixture_image_subparsers.add_parser(
        "ensure",
        help="Ensure local fixture images and print JSON payload",
    )
    fixture_image_ensure.add_argument(
        "--artifact",
        required=True,
        help="Path to artifact manifest (artifact.yml)",
    )
    fixture_image_ensure.add_argument(
        "--no-cache",
        action="store_true",
        help="Do not use local cache",
    )
    fixture_image_ensure.add_argument(
        "--output",
        choices=("json",),
        default="json",
        help="Output format",
    )

    capabilities = internal_subparsers.add_parser(
        "capabilities",
        help="Print internal contract versions for orchestrators",
    )
    capabilities.add_argument(
        "--output",
        choices=("json",),
        default="json",
        help="Output format",
    )

    return parser


def _split_compose_files(values: list[str] | None) -> list[str]:
    if not values:
        return []
    files: list[str] = []
    for value in values:
        for item in value.split(","):
            normalized = item.strip()
            if normalized != "":
                files.append(normalized)
    return files


def run(argv: list[str]) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    command = (args.command or "").strip()
    if command == "":
        print(f"Error: {DEFAULT_CTL_BIN} requires a subcommand.", file=sys.stderr)
        print(parse_error_hint(), file=sys.stderr)
        return 1

    try:
        if command == "deploy":
            warnings = execute_deploy(
                DeployInput(
                    artifact_path=args.artifact,
                    no_cache=args.no_cache,
                )
            )
            for warning in warnings:
                print(f"Warning: {warning}", file=sys.stderr)
            return 0

        if command == "provision":
            compose_files = _split_compose_files(args.compose_file)
            execute_provision(
                ProvisionInput(
                    compose_project=args.project,
                    compose_files=compose_files,
                    env_file=args.env_file,
                    project_dir=args.project_dir,
                    no_deps=not args.with_deps,
                    verbose=args.verbose,
                )
            )
            return 0

        if command == "internal":
            internal_command = (args.internal_command or "").strip()
            is_maven_ensure = (
                internal_command == "maven-shim"
                and (args.maven_shim_command or "").strip() == "ensure"
            )
            if is_maven_ensure:
                result = ensure_maven_shim_image(
                    MavenShimEnsureInput(
                        base_image=args.base_image,
                        host_registry=args.host_registry,
                        no_cache=args.no_cache,
                    )
                )
                print(
                    json.dumps(
                        {
                            "schema_version": _MAVEN_SHIM_ENSURE_SCHEMA_VERSION,
                            "shim_image": result.shim_image,
                        }
                    )
                )
                return 0

            is_fixture_ensure = (
                internal_command == "fixture-image"
                and (args.fixture_image_command or "").strip() == "ensure"
            )
            if is_fixture_ensure:
                result = execute_fixture_image_ensure(
                    FixtureImageEnsureInput(
                        artifact_path=args.artifact,
                        no_cache=args.no_cache,
                    )
                )
                print(
                    json.dumps(
                        {
                            "schema_version": result.schema_version,
                            "prepared_images": result.prepared_images,
                        }
                    )
                )
                return 0

            if internal_command == "capabilities":
                print(current_capabilities().to_json())
                return 0

            print("Error: unsupported internal command", file=sys.stderr)
            print(hint_run("internal", "capabilities", "--help"), file=sys.stderr)
            return 1

        print(f"Error: unsupported command: {command}", file=sys.stderr)
        print(hint_run("--help"), file=sys.stderr)
        return 1
    except Exception as exc:  # noqa: BLE001
        if command == "deploy":
            print(f"Error: deploy failed: {exc}", file=sys.stderr)
            print(
                f"Hint: run `{command_text('deploy', '--help')}` for required arguments.",
                file=sys.stderr,
            )
            return 1
        if command == "provision":
            print(f"Error: {exc}", file=sys.stderr)
            print(
                f"Hint: run `{command_text('provision', '--help')}` for required arguments.",
                file=sys.stderr,
            )
            return 1
        if command == "internal":
            if (args.internal_command or "").strip() == "maven-shim":
                print(f"Error: maven shim ensure failed: {exc}", file=sys.stderr)
                print(hint_run("internal", "maven-shim", "ensure", "--help"), file=sys.stderr)
                return 1
            if (args.internal_command or "").strip() == "fixture-image":
                print(f"Error: fixture image ensure failed: {exc}", file=sys.stderr)
                print(hint_run("internal", "fixture-image", "ensure", "--help"), file=sys.stderr)
                return 1
            if (args.internal_command or "").strip() == "capabilities":
                print(f"Error: {exc}", file=sys.stderr)
                print(hint_run("internal", "capabilities", "--help"), file=sys.stderr)
                return 1
        print(f"Error: {exc}", file=sys.stderr)
        print(hint_run("--help"), file=sys.stderr)
        return 1


def main() -> int:
    return run(sys.argv[1:])


if __name__ == "__main__":
    raise SystemExit(main())
