#!/usr/bin/env python3
import argparse
import sys
from pathlib import Path

# Add the project root to sys.path.
project_root = Path(__file__).parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.append(str(project_root))

from tools.cli.commands import build, up, watch, down, reset, init, logs  # noqa: E402


def main():
    parser = argparse.ArgumentParser(
        description="Edge Serverless Box CLI", formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument(
        "--template", "-t", type=str, help="Path to SAM template.yaml (default: auto-detect)"
    )
    subparsers = parser.add_subparsers(dest="command", required=True, help="Command to execute")

    # --- init command ---
    subparsers.add_parser("init", help="Initialize generator configuration interactively")
    # Note: --template is handled by main parser, not subparser

    # --- build command ---
    build_parser = subparsers.add_parser("build", help="Generate config and build function images")
    build_parser.add_argument(
        "--no-cache", action="store_true", help="Do not use cache when building images"
    )
    build_parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Show what would be generated without writing files or building",
    )
    build_parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")

    # --- up command ---
    up_parser = subparsers.add_parser("up", help="Start the environment")
    up_parser.add_argument("--build", action="store_true", help="Rebuild before starting")
    up_parser.add_argument(
        "--detach", "-d", action="store_true", default=True, help="Run in background"
    )
    up_parser.add_argument("--wait", action="store_true", help="Wait for services to be ready")

    # --- watch command ---
    subparsers.add_parser("watch", help="Watch for changes and hot-reload")

    # --- down command ---
    down_parser = subparsers.add_parser("down", help="Stop the environment")
    down_parser.add_argument(
        "--volumes",
        "-v",
        action="store_true",
        help="Remove named volumes declared in the volumes section",
    )

    # --- reset command ---
    reset_parser = subparsers.add_parser(
        "reset", help="Completely reset the environment (deletes data!)"
    )
    reset_parser.add_argument("--yes", "-y", action="store_true", help="Skip confirmation prompt")
    reset_parser.add_argument("--rmi", action="store_true", help="Remove images as well")

    # --- logs command ---
    logs_parser = subparsers.add_parser("logs", help="View service logs")
    logs_parser.add_argument("service", nargs="?", help="Service name (default: all services)")
    logs_parser.add_argument("--follow", "-f", action="store_true", help="Follow log output")
    logs_parser.add_argument("--tail", type=int, default=None, help="Number of lines to show")
    logs_parser.add_argument("--timestamps", "-t", action="store_true", help="Show timestamps")

    args = parser.parse_args()

    # Override config when --template is specified.
    if args.template:
        from tools.cli.config import set_template_yaml

        set_template_yaml(args.template)

    try:
        if args.command == "init":
            init.run(args)
        elif args.command == "build":
            build.run(args)
        elif args.command == "up":
            up.run(args)
        elif args.command == "watch":
            watch.run(args)
        elif args.command == "down":
            down.run(args)
        elif args.command == "reset":
            reset.run(args)
        elif args.command == "logs":
            logs.run(args)
    except KeyboardInterrupt:
        print("\nAborted.")
        sys.exit(0)
    except Exception as e:
        print(f"\n❌ Error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
