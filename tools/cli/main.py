#!/usr/bin/env python3
import argparse
import sys
from pathlib import Path

# プロジェクトルートをパスに追加
project_root = Path(__file__).parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.append(str(project_root))

from tools.cli.commands import build, up, watch, down  # noqa: E402


def main():
    parser = argparse.ArgumentParser(
        description="Edge Serverless Box CLI", formatter_class=argparse.RawDescriptionHelpFormatter
    )
    subparsers = parser.add_subparsers(dest="command", required=True, help="Command to execute")

    # --- build command ---
    build_parser = subparsers.add_parser("build", help="Generate config and build function images")
    build_parser.add_argument(
        "--no-cache", action="store_true", help="Do not use cache when building images"
    )

    # --- up command ---
    up_parser = subparsers.add_parser("up", help="Start the environment")
    up_parser.add_argument("--build", action="store_true", help="Rebuild before starting")
    up_parser.add_argument(
        "--detach", "-d", action="store_true", default=True, help="Run in background"
    )

    # --- watch command ---
    subparsers.add_parser("watch", help="Watch for changes and hot-reload")

    # --- down command ---
    subparsers.add_parser("down", help="Stop the environment")

    args = parser.parse_args()

    try:
        if args.command == "build":
            build.run(args)
        elif args.command == "up":
            up.run(args)
        elif args.command == "watch":
            watch.run(args)
        elif args.command == "down":
            down.run(args)
    except KeyboardInterrupt:
        print("\nAborted.")
        sys.exit(0)
    except Exception as e:
        print(f"\n❌ Error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
