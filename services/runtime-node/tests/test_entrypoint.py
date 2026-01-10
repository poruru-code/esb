# Where: services/runtime-node/tests/test_entrypoint.py
# What: Tests for runtime-node entrypoint safeguards.
# Why: Prevent devmapper pool reinitialization regressions.
from pathlib import Path

ENTRYPOINTS = (
    "services/runtime-node/entrypoint.containerd.sh",
    "services/runtime-node/entrypoint.firecracker.sh",
)


def test_entrypoint_requires_existing_devmapper_pool():
    common = Path("services/runtime-node/entrypoint.common.sh").read_text()
    assert "ensure_devmapper_ready" in common
    assert "dmsetup status" in common
    assert "Run esb node provision." in common
    assert "dmsetup create" not in common
    for path in ENTRYPOINTS:
        script = Path(path).read_text()
        assert "ensure_devmapper_ready" in script


def test_entrypoint_applies_hv_network_guard():
    common = Path("services/runtime-node/entrypoint.common.sh").read_text()
    assert "ensure_hv_network" in common
    assert "tx-checksumming off" in common
    for path in ENTRYPOINTS:
        script = Path(path).read_text()
        assert "ensure_hv_network" in script
