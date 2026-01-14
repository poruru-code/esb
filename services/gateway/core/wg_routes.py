"""
Where: services/gateway/core/wg_routes.py
What: Parse WireGuard AllowedIPs and reconcile gateway routes on startup.
Why: Keep multi-node routing stable when wg-quick misses or conflicts.
"""

from __future__ import annotations

import argparse
import ipaddress
import os
import shutil
import subprocess
import sys
from pathlib import Path
from typing import Iterable


def _dedupe_preserve_order(items: Iterable[str]) -> list[str]:
    seen: set[str] = set()
    result: list[str] = []
    for item in items:
        if item in seen:
            continue
        seen.add(item)
        result.append(item)
    return result


RFC1918_NETS = (
    ipaddress.ip_network("10.0.0.0/8"),
    ipaddress.ip_network("172.16.0.0/12"),
    ipaddress.ip_network("192.168.0.0/16"),
)
LINK_LOCAL_NET = ipaddress.ip_network("169.254.0.0/16")
MULTICAST_NET = ipaddress.ip_network("224.0.0.0/4")
DEFAULT_ROUTE_V4 = ipaddress.ip_network("0.0.0.0/0")


def parse_allowed_ips(config_text: str) -> list[str]:
    if not config_text:
        return []

    allowed_ips: list[str] = []
    for raw_line in config_text.splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or line.startswith(";"):
            continue
        if line.lower().startswith("allowedips"):
            _, _, value = line.partition("=")
            for cidr in value.split(","):
                cidr = cidr.strip()
                if cidr:
                    allowed_ips.append(cidr)
    return _dedupe_preserve_order(allowed_ips)


def filter_allowed_ips(allowed_ips: Iterable[str]) -> tuple[list[str], list[tuple[str, str]]]:
    accepted: list[str] = []
    skipped: list[tuple[str, str]] = []

    for cidr in allowed_ips:
        try:
            network = ipaddress.ip_network(cidr, strict=False)
        except ValueError:
            skipped.append((cidr, "invalid-cidr"))
            continue

        if network.version != 4:
            skipped.append((cidr, "ipv6-not-supported"))
            continue
        if network == DEFAULT_ROUTE_V4:
            skipped.append((cidr, "default-route"))
            continue
        if network.subnet_of(LINK_LOCAL_NET):
            skipped.append((cidr, "link-local"))
            continue
        if network.subnet_of(MULTICAST_NET):
            skipped.append((cidr, "multicast"))
            continue
        if network.prefixlen == 32 or any(network.subnet_of(net) for net in RFC1918_NETS):
            accepted.append(str(network))
            continue

        skipped.append((cidr, "non-rfc1918"))

    return _dedupe_preserve_order(accepted), skipped


def apply_include_filter(
    allowed_ips: Iterable[str],
    include_nets: Iterable[ipaddress._BaseNetwork],
) -> tuple[list[str], list[tuple[str, str]]]:
    filtered: list[str] = []
    skipped: list[tuple[str, str]] = []
    include_nets = tuple(include_nets)

    if not include_nets:
        return _dedupe_preserve_order(list(allowed_ips)), skipped

    for cidr in allowed_ips:
        try:
            network = ipaddress.ip_network(cidr, strict=False)
        except ValueError:
            skipped.append((cidr, "invalid-cidr"))
            continue

        if any(network.subnet_of(include_net) for include_net in include_nets):
            filtered.append(str(network))
        else:
            skipped.append((cidr, "outside-include"))

    return _dedupe_preserve_order(filtered), skipped


def _read_showconf(interface: str) -> str:
    if not interface:
        return ""
    try:
        return subprocess.check_output(
            ["wg", "showconf", interface],
            text=True,
            stderr=subprocess.DEVNULL,
        )
    except (FileNotFoundError, subprocess.CalledProcessError):
        return ""


def _read_conf_file(conf_path: str) -> str:
    if not conf_path:
        return ""
    try:
        return Path(conf_path).read_text()
    except OSError:
        return ""


def _interface_exists(interface: str) -> bool:
    if not interface:
        return False
    try:
        subprocess.check_output(
            ["ip", "link", "show", interface],
            stderr=subprocess.DEVNULL,
        )
        return True
    except (FileNotFoundError, subprocess.CalledProcessError):
        return False


def _route_replace(cidrs: Iterable[str], interface: str, via: str | None) -> int:
    if not cidrs:
        return 0
    if shutil.which("ip") is None:
        print("WARN: ip command not found; skipping route correction", file=sys.stderr)
        return 0

    if not via and not _interface_exists(interface):
        print(
            f"WARN: WireGuard interface '{interface}' not found; skipping route correction",
            file=sys.stderr,
        )
        return 0

    replaced = 0
    for cidr in cidrs:
        # Use replace to avoid 'File exists' when routes linger.
        if via:
            command = ["ip", "route", "replace", cidr, "via", via]
        else:
            command = ["ip", "route", "replace", cidr, "dev", interface]
        result = subprocess.run(
            command,
            check=False,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        if result.returncode == 0:
            target = f"via {via}" if via else f"dev {interface}"
            print(f"INFO: route replace {cidr} {target} ok", file=sys.stderr)
            replaced += 1
        else:
            reason = result.stderr.strip() or "ip-route-failed"
            target = f"via {via}" if via else f"dev {interface}"
            print(
                f"WARN: route replace {cidr} {target} failed reason={reason}",
                file=sys.stderr,
            )
    return replaced


def _resolve_allowed_ips(interface: str, conf_path: str) -> list[str]:
    text = _read_showconf(interface)
    if not text:
        text = _read_conf_file(conf_path)
    return parse_allowed_ips(text)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Ensure WireGuard routes exist for AllowedIPs.")
    parser.add_argument(
        "--interface",
        default=os.environ.get("WG_INTERFACE", ""),
        help="WireGuard interface name (default: env WG_INTERFACE)",
    )
    parser.add_argument(
        "--conf",
        default=os.environ.get("WG_CONF_PATH", ""),
        help="WireGuard config path (default: env WG_CONF_PATH)",
    )
    parser.add_argument(
        "--via",
        default=os.environ.get("WG_ROUTE_VIA", ""),
        help="Route next-hop IP for overrides (default: env WG_ROUTE_VIA)",
    )
    parser.add_argument(
        "--include",
        action="append",
        default=[],
        help="CIDR to include when applying routes (repeatable)",
    )
    args = parser.parse_args(argv)

    allowed_ips = _resolve_allowed_ips(args.interface, args.conf)
    filtered_ips, skipped = filter_allowed_ips(allowed_ips)
    include_nets: list[ipaddress._BaseNetwork] = []
    for raw_include in args.include:
        if not raw_include:
            continue
        try:
            include_nets.append(ipaddress.ip_network(raw_include, strict=False))
        except ValueError:
            print(f"WARN: include filter invalid {raw_include}", file=sys.stderr)

    if include_nets:
        filtered_ips, include_skipped = apply_include_filter(filtered_ips, include_nets)
        skipped.extend(include_skipped)

    for cidr, reason in skipped:
        print(f"WARN: route skip {cidr} reason={reason}", file=sys.stderr)

    if not filtered_ips:
        print("INFO: No eligible AllowedIPs found; skipping route correction", file=sys.stderr)
        return 0

    print(
        f"INFO: Applying WG routes from AllowedIPs: {len(filtered_ips)}",
        file=sys.stderr,
    )
    via = args.via.strip() or None
    _route_replace(filtered_ips, args.interface, via)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
