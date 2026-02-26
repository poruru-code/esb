#!/usr/bin/env python3
"""Unified deploy operations CLI."""

from __future__ import annotations

import argparse
import sys

from tools.deployops.commands import apply, bundle_dind, prepare_images
from tools.deployops.core.runner import CommandRunner, RunnerError


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Deploy operations toolkit (artifact apply + DinD bundling)",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Print commands without executing side effects",
    )

    subparsers = parser.add_subparsers(dest="command", required=True)
    apply.register_parser(subparsers)
    bundle_dind.register_parser(subparsers)
    prepare_images.register_parser(subparsers)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)
    runner = CommandRunner(dry_run=bool(args.dry_run))

    if not hasattr(args, "func"):
        parser.print_help()
        return 2

    try:
        return int(args.func(args, runner))
    except RunnerError as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1
    except (ValueError, FileNotFoundError) as exc:
        print(f"Error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
