# Where: tools/python_cli/tests/test_provision_docker_config.py
# What: Static checks for Docker daemon and network config in node provision.
# Why: Ensure stable Docker storage and Hyper-V network tuning.
from pathlib import Path


def _find_repo_root(start: Path) -> Path:
    for path in [start] + list(start.parents):
        if (path / "tools" / "pyinfra" / "esb_node_provision.py").is_file():
            return path
    raise FileNotFoundError("repo root not found for esb_node_provision.py")


def test_provision_writes_overlay2_daemon_config():
    repo_root = _find_repo_root(Path(__file__).resolve())
    provision_script = repo_root / "tools" / "pyinfra" / "esb_node_provision.py"
    content = provision_script.read_text(encoding="utf-8")

    assert "daemon.json" in content
    assert "storage-driver" in content
    assert "overlay2" in content


def test_provision_applies_hv_network_tuning():
    repo_root = _find_repo_root(Path(__file__).resolve())
    provision_script = repo_root / "tools" / "pyinfra" / "esb_node_provision.py"
    content = provision_script.read_text(encoding="utf-8")

    assert "ethtool -K eth0 tx-checksumming off" in content


def test_provision_contains_proxy_hooks():
    repo_root = _find_repo_root(Path(__file__).resolve())
    provision_script = repo_root / "tools" / "pyinfra" / "esb_node_provision.py"
    content = provision_script.read_text(encoding="utf-8")

    assert "apt.conf.d/95esb-proxy" in content
    assert "/etc/profile.d/esb-proxy.sh" in content
    assert "docker.service.d/http-proxy.conf" in content
