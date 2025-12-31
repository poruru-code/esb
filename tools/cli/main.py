#!/usr/bin/env python3
import argparse
import sys
from pathlib import Path

# Add the project root to sys.path.
project_root = Path(__file__).parent.parent.parent
if str(project_root) not in sys.path:
    sys.path.append(str(project_root))

from tools.cli.commands import build, up, watch, down, reset, init, logs, node  # noqa: E402


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
