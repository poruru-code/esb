# Where: tools/python_cli/tests/test_wireguard_config.py
# What: Tests for WireGuard config rendering helpers.
# Why: Keep wg-quick startup resilient when routes are unavailable.
from pathlib import Path

from tools.python_cli.commands import node as node_cmd


def test_write_compute_conf_routes_best_effort(tmp_path: Path) -> None:
    conf_path = tmp_path / "wg0.conf"

    node_cmd._write_compute_conf(
        conf_path,
        compute_priv="priv",
        compute_addr="10.99.0.2/32",
        listen_port=51820,
        gateway_pub="pub",
        gateway_allowed="10.99.0.1/32",
        subnet="10.88.1.0/24",
        runtime_ip="172.20.0.10",
    )

    content = conf_path.read_text(encoding="utf-8")
    assert "ip route replace 10.88.1.0/24 via 172.20.0.10 || true" in content
    assert "ip route del 10.88.1.0/24 via 172.20.0.10 || true" in content
