# Where: tools/python_cli/tests/test_provision_devmapper.py
# What: Static tests for devmapper provisioning script behavior.
# Why: Prevent regressions that reinitialize existing devmapper pools.
from pathlib import Path


def _find_repo_root(start: Path) -> Path:
    for path in [start] + list(start.parents):
        if (path / "tools" / "pyinfra" / "esb_node_provision.py").is_file():
            return path
    raise FileNotFoundError("repo root not found for esb_node_provision.py")


def test_provision_skips_devmapper_create():
    repo_root = _find_repo_root(Path(__file__).resolve())
    provision_script = repo_root / "tools" / "pyinfra" / "esb_node_provision.py"
    content = provision_script.read_text(encoding="utf-8")

    assert "/usr/local/bin/esb-mount-pool.sh" in content
    assert "/etc/systemd/system/esb-storage.service" in content
    assert "Before=containerd.service docker.service firecracker-containerd.service" in content
    assert "Devmapper pool $pool already exists; skipping create" in content
    assert "dmsetup reload" not in content
    assert "dmsetup table" in content
    assert "esb-devmapper.lock" in content


def test_provision_preallocates_backing_files():
    repo_root = _find_repo_root(Path(__file__).resolve())
    provision_script = repo_root / "tools" / "pyinfra" / "esb_node_provision.py"
    content = provision_script.read_text(encoding="utf-8")

    assert "ensure_backing_file" in content
    assert "fallocate -l" in content
    assert "WARN: fallocate failed" in content


def test_provision_defaults_devmapper_udev_disabled():
    repo_root = _find_repo_root(Path(__file__).resolve())
    provision_script = repo_root / "tools" / "pyinfra" / "esb_node_provision.py"
    content = provision_script.read_text(encoding="utf-8")

    assert '_get_bool("esb_devmapper_udev", False)' in content
