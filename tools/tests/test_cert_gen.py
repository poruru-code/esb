"""
Where: tools/tests/test_cert_gen.py
What: Tests for cert-gen helpers.
Why: Validate step-cli command and host resolution for client/server certs.
"""

from importlib import util
from pathlib import Path

import pytest

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


def test_build_step_root_ca_command_order():
    cmd = cert_gen.build_step_root_ca_command(
        "/bin/step",
        "ESB Local CA",
        "/tmp/rootCA.crt",
        "/tmp/rootCA.key",
        not_after="87600h",
    )

    assert cmd == [
        "/bin/step",
        "certificate",
        "create",
        "ESB Local CA",
        "/tmp/rootCA.crt",
        "/tmp/rootCA.key",
        "--profile",
        "root-ca",
        "--no-password",
        "--insecure",
        "--not-after",
        "87600h",
    ]


def test_build_step_leaf_command_includes_sans_and_validity():
    cmd = cert_gen.build_step_leaf_command(
        "/bin/step",
        "gateway",
        "/tmp/server.crt",
        "/tmp/server.key",
        ["gateway", "localhost", "127.0.0.1"],
        "/tmp/rootCA.crt",
        "/tmp/rootCA.key",
        not_after="8760h",
    )

    assert cmd == [
        "/bin/step",
        "certificate",
        "create",
        "gateway",
        "/tmp/server.crt",
        "/tmp/server.key",
        "--profile",
        "leaf",
        "--ca",
        "/tmp/rootCA.crt",
        "--ca-key",
        "/tmp/rootCA.key",
        "--no-password",
        "--insecure",
        "--not-after",
        "8760h",
        "--san",
        "gateway",
        "--san",
        "localhost",
        "--san",
        "127.0.0.1",
    ]


def test_require_validity_rejects_missing():
    with pytest.raises(RuntimeError, match="certificate.ca_validity"):
        cert_gen.require_validity(None, "ca_validity")
