#!/usr/bin/env python3
# Where: tools/cli/main.py
# What: CLI entrypoint and argument dispatch.
# Why: Provide a single command surface for ESB operations.
import argparse
import sys
from pathlib import Path

# Add the project root to sys.path.
project_root = Path(__file__).parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.append(str(project_root))

# Commands will be imported inside main() to ensure env vars are set first.


def main():
    from tools.cli import config as cli_config
    from tools.cli.commands import build, down, init, logs, node, reset, stop, up, watch
    from tools.cli.core import context, help_text

    parser = argparse.ArgumentParser(
        description="Edge Serverless Box CLI", formatter_class=argparse.RawDescriptionHelpFormatter
    )
    parser.add_argument(
        "--template", "-t", type=str, help="Path to SAM template.yaml (default: auto-detect)"
    )
    subparsers = parser.add_subparsers(dest="command", required=True, help="Command to execute")

    # --- init command ---
    init_parser = subparsers.add_parser(
        "init", help="Initialize generator configuration interactively"
    )
    init_parser.add_argument("--env", type=str, default=None, help=help_text.INIT_ENV)
    # Note: --template is handled by main parser, not subparser.
    # Environment is prompted in the wizard (unless --env is given).

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
    build_parser.add_argument(
        "-f", "--file", action="append", help="Specify an additional compose file"
    )
    build_parser.add_argument("--env", type=str, help="Environment name")

    # --- up command ---
    up_parser = subparsers.add_parser("up", help="Start the environment")
    up_parser.add_argument("--build", action="store_true", help="Rebuild before starting")
    up_parser.add_argument(
        "--detach", "-d", action="store_true", default=True, help="Run in background"
    )
    up_parser.add_argument("--wait", action="store_true", help="Wait for services to be ready")
    up_parser.add_argument(
        "-f", "--file", action="append", help="Specify an additional compose file"
    )
    up_parser.add_argument("--env", type=str, help="Environment name")

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
    down_parser.add_argument(
        "-f", "--file", action="append", help="Specify an additional compose file"
    )
    down_parser.add_argument("--env", type=str, help="Environment name")

    # --- stop command ---
    stop_parser = subparsers.add_parser("stop", help="Stop the environment (preserve state)")
    stop_parser.add_argument(
        "-f", "--file", action="append", help="Specify an additional compose file"
    )
    stop_parser.add_argument("--env", type=str, help="Environment name")

    # --- reset command ---
    reset_parser = subparsers.add_parser(
        "reset", help="Completely reset the environment (deletes data!)"
    )
    reset_parser.add_argument("--yes", "-y", action="store_true", help="Skip confirmation prompt")
    reset_parser.add_argument("--rmi", action="store_true", help="Remove images as well")
    reset_parser.add_argument("--env", type=str, help="Environment name")

    # --- logs command ---
    logs_parser = subparsers.add_parser("logs", help="View service logs")
    logs_parser.add_argument("service", nargs="?", help="Service name (default: all services)")
    logs_parser.add_argument("--follow", "-f", action="store_true", help="Follow log output")
    logs_parser.add_argument("--tail", type=int, default=None, help="Number of lines to show")
    logs_parser.add_argument("--timestamps", "-t", action="store_true", help="Show timestamps")
    logs_parser.add_argument("--env", type=str, help="Environment name")

    # --- node command ---
    node_parser = subparsers.add_parser("node", help="Manage compute nodes")
    node_subparsers = node_parser.add_subparsers(
        dest="node_command", required=True, help="Node subcommand"
    )
    node_add_parser = node_subparsers.add_parser("add", help="Register a compute node")
    node_add_parser.add_argument(
        "--payload",
        help="Node payload JSON/YAML (if omitted, paste interactively)",
    )
    node_add_parser.add_argument(
        "--host",
        help="Fetch payload over SSH (user@host) or override host from payload",
    )
    node_add_parser.add_argument("--user", help="Override SSH user from payload")
    node_add_parser.add_argument("--port", type=int, help="Override SSH port from payload")
    node_add_parser.add_argument("--name", help="Override node name from payload")
    node_add_parser.add_argument(
        "--identity-file",
        help="SSH identity file (optional)",
    )
    node_add_parser.add_argument(
        "--password",
        help="SSH password (used to install the generated key)",
    )
    node_add_parser.add_argument(
        "--skip-key-setup",
        action="store_true",
        help="Skip generating/installing an SSH key",
    )
    node_add_parser.add_argument(
        "--ssh-option",
        action="append",
        default=[],
        help="Additional ssh -o options (repeatable)",
    )
    node_doctor_parser = node_subparsers.add_parser("doctor", help="Check node readiness")
    node_doctor_parser.add_argument(
        "--name",
        help="Check a specific node by name",
    )
    node_doctor_parser.add_argument(
        "--host",
        help="Check a specific host (user@host supported)",
    )
    node_doctor_parser.add_argument("--user", help="Override SSH user")
    node_doctor_parser.add_argument("--port", type=int, help="Override SSH port")
    node_doctor_parser.add_argument(
        "--identity-file",
        help="SSH identity file (optional)",
    )
    node_doctor_parser.add_argument(
        "--ssh-option",
        action="append",
        default=[],
        help="Additional ssh -o options (repeatable)",
    )
    node_doctor_parser.add_argument(
        "--strict",
        action="store_true",
        help="Exit with non-zero if any node fails",
    )
    node_doctor_parser.add_argument(
        "--require-up",
        action="store_true",
        help="Fail if compute services are not running",
    )
    node_up_parser = node_subparsers.add_parser("up", help="Start compute node services")
    node_up_parser.add_argument(
        "--name",
        help="Start a specific node by name",
    )
    node_up_parser.add_argument(
        "--host",
        help="Start a specific host (user@host supported)",
    )
    node_up_parser.add_argument("--user", help="Override SSH user")
    node_up_parser.add_argument("--port", type=int, help="Override SSH port")
    node_up_parser.add_argument(
        "--identity-file",
        help="SSH identity file (optional)",
    )
    node_up_parser.add_argument(
        "--ssh-option",
        action="append",
        default=[],
        help="Additional ssh -o options (repeatable)",
    )
    node_provision_parser = node_subparsers.add_parser(
        "provision", help="Provision compute node prerequisites"
    )
    node_provision_parser.add_argument(
        "--name",
        help="Provision a specific node by name",
    )
    node_provision_parser.add_argument(
        "--host",
        help="Provision a specific host (user@host supported)",
    )
    node_provision_parser.add_argument("--user", help="Override SSH user")
    node_provision_parser.add_argument("--port", type=int, help="Override SSH port")
    node_provision_parser.add_argument(
        "--identity-file",
        help="SSH identity file (optional)",
    )
    node_provision_parser.add_argument(
        "--password",
        help="SSH password (optional, will prompt if not provided)",
    )
    node_provision_parser.add_argument(
        "--sudo-password",
        help="Sudo password (defaults to SSH password)",
    )
    node_provision_parser.add_argument(
        "--sudo-nopasswd",
        action="store_true",
        help="Configure passwordless sudo for the SSH user",
    )
    node_provision_parser.add_argument(
        "--firecracker-version",
        help="Firecracker version to install (default: 1.14.0)",
    )
    node_provision_parser.add_argument(
        "--firecracker-containerd-ref",
        help="firecracker-containerd git ref to build (default: d6ffdaa)",
    )
    node_provision_parser.add_argument(
        "--firecracker-install-dir",
        help="Install directory for Firecracker binaries (default: /usr/local/bin)",
    )
    node_provision_parser.add_argument(
        "--firecracker-runtime-dir",
        help="Runtime asset directory (default: /var/lib/firecracker-containerd/runtime)",
    )
    node_provision_parser.add_argument(
        "--firecracker-kernel-url",
        help="Kernel URL for Firecracker runtime (optional)",
    )
    node_provision_parser.add_argument(
        "--firecracker-rootfs-url",
        help="RootFS URL for Firecracker runtime (optional)",
    )
    node_provision_parser.add_argument(
        "--firecracker-kernel-path",
        help="Destination path for the kernel image (optional)",
    )
    node_provision_parser.add_argument(
        "--firecracker-rootfs-path",
        help="Destination path for the rootfs image (optional)",
    )
    node_provision_parser.add_argument(
        "--devmapper-pool",
        help="devmapper pool name (default: fc-dev-pool2)",
    )
    node_provision_parser.add_argument(
        "--devmapper-dir",
        help="devmapper root directory (default: /var/lib/containerd/devmapper2)",
    )
    node_provision_parser.add_argument(
        "--devmapper-data-size",
        help="devmapper data file size (default: 10G)",
    )
    node_provision_parser.add_argument(
        "--devmapper-meta-size",
        help="devmapper metadata file size (default: 2G)",
    )
    node_provision_parser.add_argument(
        "--devmapper-base-image-size",
        help="devmapper base image size (default: 10GB)",
    )
    node_provision_parser.add_argument(
        "--wg-conf",
        help="WireGuard wg0.conf for compute node (default: ~/.esb/wireguard/compute/wg0.conf)",
    )
    node_provision_parser.add_argument(
        "--wg-subnet",
        help="WireGuard subnet for this node (default: 10.88.<index>.0/24)",
    )
    node_provision_parser.add_argument(
        "--wg-runtime-ip",
        help="runtime-node external IP used for route (default: 172.20.0.10)",
    )
    node_provision_parser.add_argument(
        "--wg-endpoint-port",
        type=int,
        help="WireGuard listen port on compute node (default: 51820)",
    )
    node_provision_parser.add_argument(
        "--devmapper-udev",
        dest="devmapper_udev",
        action="store_true",
        help="Enable udev synchronization for devmapper",
    )
    node_provision_parser.add_argument(
        "--no-devmapper-udev",
        dest="devmapper_udev",
        action="store_false",
        help="Disable udev synchronization for devmapper",
    )
    node_provision_parser.set_defaults(devmapper_udev=None)
    node_provision_parser.add_argument(
        "-v",
        "--verbose",
        action="count",
        default=0,
        help="Increase pyinfra verbosity (repeatable)",
    )

    args = parser.parse_args()

    # Template override handled FIRST so that E2E_DIR is correctly set for generator.yml lookup
    if args.template:
        from tools.cli.config import set_template_yaml

        set_template_yaml(args.template)

    # Ensure template is available (required for most commands)
    if cli_config.TEMPLATE_YAML is None:
        print("❌ No template specified.")
        print(help_text.NO_TEMPLATE_ERROR)
        sys.exit(1)

    # Enforce environment argument if present (main entrypoint is lenient on existence)
    # Skip interactive selection for init command - it prompts for environment name in its wizard
    skip_interactive = args.command == "init"
    context.enforce_env_arg(args, require_built=False, skip_interactive=skip_interactive)

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
        elif args.command == "stop":
            stop.run(args)
        elif args.command == "reset":
            reset.run(args)
        elif args.command == "logs":
            logs.run(args)
        elif args.command == "node":
            node.run(args)
    except KeyboardInterrupt:
        print("\nAborted.")
        sys.exit(0)
    except Exception as e:
        print(f"\n❌ Error: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
