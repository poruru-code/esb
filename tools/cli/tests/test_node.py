import json
from argparse import Namespace
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from tools.cli.commands import node as node_cmd


def _args(**overrides):
    base = dict(
        host=None,
        user=None,
        port=None,
        name=None,
        payload=None,
        password=None,
        identity_file=None,
        skip_key_setup=False,
        ssh_option=[],
        strict=False,
        sudo_password=None,
        sudo_nopasswd=False,
        firecracker_version=None,
        firecracker_containerd_ref=None,
        firecracker_install_dir=None,
        firecracker_runtime_dir=None,
        firecracker_kernel_url=None,
        firecracker_rootfs_url=None,
        firecracker_kernel_path=None,
        firecracker_rootfs_path=None,
        devmapper_pool=None,
        devmapper_dir=None,
        devmapper_data_size=None,
        devmapper_meta_size=None,
        devmapper_base_image_size=None,
        devmapper_udev=None,
        verbose=0,
    )
    base.update(overrides)
    return Namespace(**base)


def test_parse_payload_json():
    payload = {"host": "10.0.0.5", "user": "esb"}
    assert node_cmd._parse_payload(json.dumps(payload)) == payload


def test_parse_payload_yaml():
    raw = "host: 10.0.0.5\nuser: esb\n"
    assert node_cmd._parse_payload(raw) == {"host": "10.0.0.5", "user": "esb"}


def test_parse_payload_invalid():
    with pytest.raises(ValueError):
        node_cmd._parse_payload("not-json")


def test_resolve_node_with_overrides():
    args = _args(
        host="admin@10.0.0.5",
        port=2222,
        name="node-1",
        identity_file="~/.esb/id_ed25519",
    )
    payload = {"id": "node-id", "hostname": "host1", "arch": "x86_64", "os_release": "ubuntu"}
    node = node_cmd._resolve_node(payload, args)

    assert node["host"] == "10.0.0.5"
    assert node["user"] == "admin"
    assert node["port"] == 2222
    assert node["name"] == "node-1"
    assert node["identity_file"] == str(Path("~/.esb/id_ed25519").expanduser())
    assert node["facts"]["arch"] == "x86_64"


def test_upsert_node_updates_existing():
    nodes = [{"id": "node-1", "host": "old"}]
    updated = node_cmd._upsert_node(nodes, {"id": "node-1", "host": "new"})

    assert updated is True
    assert nodes[0]["host"] == "new"


def test_upsert_node_adds_new():
    nodes: list[dict[str, str]] = []
    updated = node_cmd._upsert_node(nodes, {"id": "node-2", "host": "host"})

    assert updated is False
    assert nodes[0]["id"] == "node-2"


def test_run_add_installs_key_and_saves_node(tmp_path):
    key_path = tmp_path / "id_ed25519"
    pub_path = tmp_path / "id_ed25519.pub"
    key_path.write_text("private")
    pub_path.write_text("ssh-ed25519 AAAA testkey")

    payload = {
        "id": "node-1",
        "hostname": "node-1",
        "ssh_host_key": "ssh-ed25519 AAAA remotehost",
    }
    args = _args(host="10.0.0.5", user="esb", port=22, name="node-1", password="pw")
    raw_payload = json.dumps(payload)

    saved: dict[str, object] = {}

    def _save(data):
        saved.update(data)

    with patch.object(node_cmd, "_read_payload", return_value=raw_payload), patch.object(
        node_cmd, "_ensure_local_keypair", return_value=(key_path, pub_path)
    ), patch.object(node_cmd, "_install_public_key", return_value=True) as mock_install, patch.object(
        node_cmd, "_load_nodes", return_value={"version": 1, "nodes": []}
    ), patch.object(node_cmd, "_save_nodes", side_effect=_save) as mock_save, patch.object(
        node_cmd, "_update_known_hosts"
    ) as mock_known:
        node_cmd._run_add(args)

    assert mock_install.called is True
    assert mock_save.called is True
    assert saved["nodes"][0]["identity_file"] == str(key_path)
    mock_known.assert_called_once_with("10.0.0.5 ssh-ed25519 AAAA")


def test_doctor_via_ssh_ok():
    args = _args()
    node = {"host": "10.0.0.5", "user": "esb", "port": 22}
    payload = {
        "dev_kvm": True,
        "dev_vhost_vsock": True,
        "dev_tun": True,
        "cmd_firecracker": True,
        "cmd_firecracker_containerd": True,
        "cmd_firecracker_ctr": True,
        "cmd_containerd_shim_aws_firecracker": True,
        "fc_kernel": True,
        "fc_rootfs": True,
        "fc_containerd_config": True,
        "fc_runtime_config": True,
        "fc_runc_config": True,
        "devmapper_pool": True,
    }
    result = MagicMock(returncode=0, stdout=json.dumps(payload), stderr="")

    with patch.object(node_cmd, "_ssh_options", return_value=[]), patch(
        "tools.cli.commands.node.subprocess.run", return_value=result
    ):
        response = node_cmd._doctor_via_ssh(node, args)

    assert response["ok"] is True
    assert response["cmd_firecracker"] is True
    assert response["cmd_firecracker_containerd"] is True
    assert response["cmd_firecracker_ctr"] is True
    assert response["cmd_containerd_shim_aws_firecracker"] is True
    assert response["fc_kernel"] is True
    assert response["fc_rootfs"] is True
    assert response["fc_containerd_config"] is True
    assert response["fc_runtime_config"] is True
    assert response["fc_runc_config"] is True
    assert response["devmapper_pool"] is True


def test_doctor_via_ssh_invalid_json():
    args = _args()
    node = {"host": "10.0.0.5", "user": "esb", "port": 22}
    result = MagicMock(returncode=0, stdout="not-json", stderr="")

    with patch.object(node_cmd, "_ssh_options", return_value=[]), patch(
        "tools.cli.commands.node.subprocess.run", return_value=result
    ):
        response = node_cmd._doctor_via_ssh(node, args)

    assert response["ok"] is False
    assert response["error"] == "invalid JSON response"


def test_run_doctor_no_nodes_exits():
    args = _args()
    with patch.object(node_cmd, "_select_nodes", return_value=[]):
        with pytest.raises(SystemExit) as exc:
            node_cmd._run_doctor(args)
    assert exc.value.code == 1


def test_run_doctor_strict_exits_on_failure():
    args = _args(strict=True)
    nodes = [{"name": "node-1", "host": "10.0.0.5", "port": 22}]
    with patch.object(node_cmd, "_select_nodes", return_value=nodes), patch.object(
        node_cmd, "_doctor_via_ssh", return_value={"ok": False, "error": "fail"}
    ), patch.object(node_cmd, "_render_doctor"):
        with pytest.raises(SystemExit) as exc:
            node_cmd._run_doctor(args)
    assert exc.value.code == 1


def test_run_provision_sudo_nopasswd_no_prompt(tmp_path):
    key_path = tmp_path / "id_ed25519"
    key_path.write_text("key")
    args = _args(identity_file=str(key_path), sudo_nopasswd=True)
    nodes = [
        {
            "name": "node-1",
            "host": "10.0.0.5",
            "user": "esb",
            "port": 22,
            "sudo_nopasswd": True,
        }
    ]

    captured = {}

    def _capture(inventory_hosts, deploy_path, verbosity):
        captured["inventory_hosts"] = inventory_hosts
        captured["deploy_path"] = deploy_path
        captured["verbosity"] = verbosity

    with patch.object(node_cmd, "_ensure_pyinfra"), patch.object(
        node_cmd, "_select_nodes", return_value=nodes
    ), patch.object(node_cmd, "_prompt_secret", side_effect=AssertionError("prompted")), patch.object(
        node_cmd, "_run_pyinfra_deploy", side_effect=_capture
    ), patch.object(node_cmd, "_default_identity_file", return_value=None):
        node_cmd._run_provision(args)

    data = captured["inventory_hosts"][0][1]
    assert data["ssh_key"] == str(key_path)
    assert data["_sudo"] is True
    assert data["esb_sudo_nopasswd"] is True


def test_run_provision_prompts_for_sudo(tmp_path):
    key_path = tmp_path / "id_ed25519"
    key_path.write_text("key")
    args = _args(identity_file=str(key_path))
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]

    captured = {}

    def _capture(inventory_hosts, deploy_path, verbosity):
        captured["inventory_hosts"] = inventory_hosts

    with patch.object(node_cmd, "_ensure_pyinfra"), patch.object(
        node_cmd, "_select_nodes", return_value=nodes
    ), patch.object(node_cmd, "_prompt_secret", return_value="sudopass") as mock_prompt, patch.object(
        node_cmd, "_run_pyinfra_deploy", side_effect=_capture
    ), patch.object(node_cmd, "_default_identity_file", return_value=None):
        node_cmd._run_provision(args)

    assert mock_prompt.called is True
    data = captured["inventory_hosts"][0][1]
    assert data["_sudo_password"] == "sudopass"


def test_run_provision_prompts_for_sudo_when_bootstrapping_nopasswd(tmp_path):
    key_path = tmp_path / "id_ed25519"
    key_path.write_text("key")
    args = _args(identity_file=str(key_path), sudo_nopasswd=True)
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]

    captured = {}

    def _capture(inventory_hosts, deploy_path, verbosity):
        captured["inventory_hosts"] = inventory_hosts

    with patch.object(node_cmd, "_ensure_pyinfra"), patch.object(
        node_cmd, "_select_nodes", return_value=nodes
    ), patch.object(node_cmd, "_prompt_secret", return_value="sudopass") as mock_prompt, patch.object(
        node_cmd, "_run_pyinfra_deploy", side_effect=_capture
    ), patch.object(node_cmd, "_default_identity_file", return_value=None):
        node_cmd._run_provision(args)

    assert mock_prompt.called is True
    data = captured["inventory_hosts"][0][1]
    assert data["_sudo_password"] == "sudopass"


def test_run_provision_passes_firecracker_settings(tmp_path):
    key_path = tmp_path / "id_ed25519"
    key_path.write_text("key")
    args = _args(
        identity_file=str(key_path),
        sudo_nopasswd=True,
        firecracker_version="1.14.0",
        firecracker_containerd_ref="deadbeef",
        firecracker_install_dir="/opt/firecracker",
    )
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]

    captured = {}

    def _capture(inventory_hosts, deploy_path, verbosity):
        captured["inventory_hosts"] = inventory_hosts

    with patch.object(node_cmd, "_ensure_pyinfra"), patch.object(
        node_cmd, "_select_nodes", return_value=nodes
    ), patch.object(node_cmd, "_run_pyinfra_deploy", side_effect=_capture), patch.object(
        node_cmd, "_default_identity_file", return_value=None
    ):
        node_cmd._run_provision(args)

    data = captured["inventory_hosts"][0][1]
    assert data["esb_firecracker_version"] == "1.14.0"
    assert data["esb_firecracker_containerd_ref"] == "deadbeef"
    assert data["esb_firecracker_install_dir"] == "/opt/firecracker"


def test_run_provision_skips_prompt_when_nopasswd_set(tmp_path):
    key_path = tmp_path / "id_ed25519"
    key_path.write_text("key")
    args = _args(identity_file=str(key_path))
    nodes = [
        {
            "name": "node-1",
            "host": "10.0.0.5",
            "user": "esb",
            "port": 22,
            "sudo_nopasswd": True,
        }
    ]

    captured = {}

    def _capture(inventory_hosts, deploy_path, verbosity):
        captured["inventory_hosts"] = inventory_hosts

    with patch.object(node_cmd, "_ensure_pyinfra"), patch.object(
        node_cmd, "_select_nodes", return_value=nodes
    ), patch.object(node_cmd, "_prompt_secret", side_effect=AssertionError("prompted")), patch.object(
        node_cmd, "_run_pyinfra_deploy", side_effect=_capture
    ), patch.object(node_cmd, "_default_identity_file", return_value=None):
        node_cmd._run_provision(args)

    data = captured["inventory_hosts"][0][1]
    assert data["esb_sudo_nopasswd"] is True


def test_run_provision_passes_firecracker_assets_and_devmapper(tmp_path):
    key_path = tmp_path / "id_ed25519"
    key_path.write_text("key")
    args = _args(
        identity_file=str(key_path),
        sudo_nopasswd=True,
        firecracker_runtime_dir="/var/lib/firecracker-containerd/runtime",
        firecracker_kernel_url="https://example.com/kernel",
        firecracker_rootfs_url="https://example.com/rootfs",
        firecracker_kernel_path="/var/lib/firecracker-containerd/runtime/vmlinux",
        firecracker_rootfs_path="/var/lib/firecracker-containerd/runtime/rootfs.img",
        devmapper_pool="fc-dev-pool-test",
        devmapper_dir="/var/lib/containerd/devmapper-test",
        devmapper_data_size="5G",
        devmapper_meta_size="1G",
        devmapper_base_image_size="5GB",
        devmapper_udev=False,
    )
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]

    captured = {}

    def _capture(inventory_hosts, deploy_path, verbosity):
        captured["inventory_hosts"] = inventory_hosts

    with patch.object(node_cmd, "_ensure_pyinfra"), patch.object(
        node_cmd, "_select_nodes", return_value=nodes
    ), patch.object(node_cmd, "_run_pyinfra_deploy", side_effect=_capture), patch.object(
        node_cmd, "_default_identity_file", return_value=None
    ):
        node_cmd._run_provision(args)

    data = captured["inventory_hosts"][0][1]
    assert data["esb_firecracker_runtime_dir"] == "/var/lib/firecracker-containerd/runtime"
    assert data["esb_firecracker_kernel_url"] == "https://example.com/kernel"
    assert data["esb_firecracker_rootfs_url"] == "https://example.com/rootfs"
    assert data["esb_firecracker_kernel_path"] == "/var/lib/firecracker-containerd/runtime/vmlinux"
    assert data["esb_firecracker_rootfs_path"] == "/var/lib/firecracker-containerd/runtime/rootfs.img"
    assert data["esb_devmapper_pool"] == "fc-dev-pool-test"
    assert data["esb_devmapper_dir"] == "/var/lib/containerd/devmapper-test"
    assert data["esb_devmapper_data_size"] == "5G"
    assert data["esb_devmapper_meta_size"] == "1G"
    assert data["esb_devmapper_base_image_size"] == "5GB"
    assert data["esb_devmapper_udev"] is False
