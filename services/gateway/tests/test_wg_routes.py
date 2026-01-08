"""
Where: services/gateway/tests/test_wg_routes.py
What: Tests for WireGuard AllowedIPs parsing used by gateway route correction.
Why: Keep multi-node routing behavior stable as AllowedIPs formats evolve.
"""

import ipaddress

from services.gateway.core.wg_routes import (
    apply_include_filter,
    filter_allowed_ips,
    parse_allowed_ips,
)

RFC1918_SAMPLE = "10.88.1.0/24"
WG_HOST_SAMPLE = "10.99.0.2/32"
PUBLIC_HOST_SAMPLE = "203.0.113.10/32"
DEFAULT_ROUTE = "0.0.0.0/0"
LINK_LOCAL = "169.254.0.0/16"
MULTICAST = "224.0.0.0/4"
IPV6_DEFAULT = "::/0"


def test_parse_allowed_ips_collects_all_peers():
    showconf = """
[Interface]
PrivateKey = test
ListenPort = 51820

[Peer]
PublicKey = peer-a
AllowedIPs = 10.88.1.0/24, 10.99.0.2/32

[Peer]
PublicKey = peer-b
AllowedIPs=10.88.2.0/24,10.99.0.3/32
"""

    assert parse_allowed_ips(showconf) == [
        "10.88.1.0/24",
        "10.99.0.2/32",
        "10.88.2.0/24",
        "10.99.0.3/32",
    ]


def test_parse_allowed_ips_dedupes_and_ignores_noise():
    showconf = """
# comment
[Peer]
AllowedIPs = 10.88.1.0/24, 10.99.0.2/32
AllowedIPs = 10.88.1.0/24
; semicolon comment
"""

    assert parse_allowed_ips(showconf) == [
        "10.88.1.0/24",
        "10.99.0.2/32",
    ]


def test_filter_allowed_ips_accepts_rfc1918_and_host_routes():
    accepted, skipped = filter_allowed_ips(
        [RFC1918_SAMPLE, WG_HOST_SAMPLE, PUBLIC_HOST_SAMPLE]
    )

    assert accepted == [RFC1918_SAMPLE, WG_HOST_SAMPLE, PUBLIC_HOST_SAMPLE]
    assert skipped == []


def test_filter_allowed_ips_skips_blocked_ranges_and_ipv6():
    accepted, skipped = filter_allowed_ips(
        [DEFAULT_ROUTE, LINK_LOCAL, MULTICAST, IPV6_DEFAULT, RFC1918_SAMPLE]
    )

    assert accepted == [RFC1918_SAMPLE]

    reasons = {reason for _, reason in skipped}
    assert "default-route" in reasons
    assert "link-local" in reasons
    assert "multicast" in reasons
    assert "ipv6-not-supported" in reasons


def test_apply_include_filter_limits_subnets():
    allowed = [RFC1918_SAMPLE, WG_HOST_SAMPLE, PUBLIC_HOST_SAMPLE]
    include_nets = [ipaddress.ip_network("10.88.0.0/16")]

    filtered, skipped = apply_include_filter(allowed, include_nets)

    assert filtered == [RFC1918_SAMPLE]
    assert {reason for _, reason in skipped} == {"outside-include"}
