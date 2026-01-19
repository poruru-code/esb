"""
Where: tools/tests/test_cert_gen.py
What: Tests for cert-gen helpers.
Why: Validate mkcert command and host resolution for client/server certs.
"""

from importlib import util
from pathlib import Path

MODULE_PATH = Path(__file__).resolve().parents[2] / "tools" / "cert-gen" / "generate.py"
SPEC = util.spec_from_file_location("cert_gen", MODULE_PATH)
if SPEC is None or SPEC.loader is None:
    raise RuntimeError("Failed to load cert-gen module for tests.")
cert_gen = util.module_from_spec(SPEC)
SPEC.loader.exec_module(cert_gen)


def test_collect_hosts_includes_local_ip_once():
    host_cfg = {
        "domains": ["gateway"],
        "ips": ["127.0.0.1"],
        "include_local_ip": True,
    }

    domains, ips = cert_gen.collect_hosts(host_cfg, local_ip="10.0.0.5")

    assert domains == ["gateway"]
    assert ips.count("10.0.0.5") == 1


def test_collect_hosts_skips_local_ip_when_disabled():
    host_cfg = {
        "domains": ["gateway"],
        "ips": ["127.0.0.1"],
        "include_local_ip": False,
    }

    domains, ips = cert_gen.collect_hosts(host_cfg, local_ip="10.0.0.5")

    assert domains == ["gateway"]
    assert ips == ["127.0.0.1"]


def test_build_mkcert_command_order():
    cmd = cert_gen.build_mkcert_command(
        "/bin/mkcert",
        "/tmp/server.crt",
        "/tmp/server.key",
        ["gateway", "localhost"],
        ["127.0.0.1"],
    )

    assert cmd == [
        "/bin/mkcert",
        "-cert-file",
        "/tmp/server.crt",
        "-key-file",
        "/tmp/server.key",
        "gateway",
        "localhost",
        "127.0.0.1",
    ]


def test_build_mkcert_command_includes_client_flag():
    cmd = cert_gen.build_mkcert_command(
        "/bin/mkcert",
        "/tmp/client.crt",
        "/tmp/client.key",
        ["gateway"],
        [],
        extra_args=["-client"],
    )

    assert cmd == [
        "/bin/mkcert",
        "-client",
        "-cert-file",
        "/tmp/client.crt",
        "-key-file",
        "/tmp/client.key",
        "gateway",
    ]
