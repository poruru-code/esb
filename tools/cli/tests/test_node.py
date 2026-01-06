# Where: tools/cli/tests/test_node.py
# What: Tests for node CLI behavior.
# Why: Ensure node workflows remain stable.
import json
from argparse import Namespace
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from tools.cli.commands import node as node_cmd
from tools.cli import config as cli_config


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
        require_up=False,
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
        wg_conf=None,
        wg_subnet=None,
        wg_runtime_ip=None,
        wg_endpoint_port=None,
        verbose=0,
    )
    base.update(overrides)
    return Namespace(**base)


def test_build_inventory_hosts_injects_proxy(monkeypatch):
    monkeypatch.setenv("HTTP_PROXY", "http://proxy.internal:3128")
    monkeypatch.setenv("NO_PROXY", "127.0.0.1")
    nodes = [{"id": "node-1", "host": "10.0.0.5"}]
    args = _args()

    hosts = node_cmd._build_inventory_hosts(nodes, args, ssh_password=None, sudo_password=None)
    _, data = hosts[0]

    assert data["esb_http_proxy"] == "http://proxy.internal:3128"
    assert "localhost" in data["esb_no_proxy"].split(",")


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


def test_upsert_node_preserves_existing_fields():
    nodes = [
        {
            "id": "node-1",
            "host": "10.0.0.5",
            "sudo_nopasswd": True,
            "facts": {"arch": "x86_64"},
        }
    ]
    updated = node_cmd._upsert_node(
        nodes,
        {"id": "node-1", "host": "10.0.0.5", "facts": {"hostname": "node-1"}},
    )

    assert updated is True
    assert nodes[0]["sudo_nopasswd"] is True
    assert nodes[0]["facts"]["arch"] == "x86_64"
    assert nodes[0]["facts"]["hostname"] == "node-1"


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
        node_cmd, "_ssh_key_auth_ok", side_effect=[False, True]
    ), patch.object(node_cmd, "_load_nodes", return_value={"version": 1, "nodes": []}), patch.object(
        node_cmd, "_save_nodes", side_effect=_save
    ) as mock_save, patch.object(node_cmd, "_update_known_hosts") as mock_known:
        node_cmd._run_add(args)

    assert mock_install.called is True
    assert mock_save.called is True
    assert saved["nodes"][0]["identity_file"] == str(key_path)
    mock_known.assert_called_once_with("10.0.0.5 ssh-ed25519 AAAA")


def test_run_add_reinstalls_key_when_auth_fails(tmp_path):
    key_path = tmp_path / "id_ed25519"
    pub_path = tmp_path / "id_ed25519.pub"
    key_path.write_text("private")
    pub_path.write_text("ssh-ed25519 AAAA testkey")

    payload = {
        "id": "node-1",
        "hostname": "node-1",
        "ssh_host_key": "ssh-ed25519 AAAA remotehost",
    }
    args = _args(
        host="10.0.0.5",
        user="esb",
        port=22,
        name="node-1",
        password="pw",
        identity_file=str(key_path),
    )
    raw_payload = json.dumps(payload)

    saved: dict[str, object] = {}

    def _save(data):
        saved.update(data)

    with patch.object(node_cmd, "_read_payload", return_value=raw_payload), patch.object(
        node_cmd, "_install_public_key", return_value=True
    ) as mock_install, patch.object(
        node_cmd, "_ssh_key_auth_ok", side_effect=[False, True]
    ), patch.object(
        node_cmd, "_load_nodes", return_value={"version": 1, "nodes": []}
    ), patch.object(node_cmd, "_save_nodes", side_effect=_save), patch.object(
        node_cmd, "_update_known_hosts"
    ):
        node_cmd._run_add(args)

    assert mock_install.called is True
    assert saved["nodes"][0]["identity_file"] == str(key_path)


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
        "cmd_wg": True,
        "cmd_wg_quick": True,
        "wg_conf": True,
        "wg_up": True,
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


def test_doctor_via_ssh_uses_sudo_when_nopasswd():
    args = _args()
    node = {"host": "10.0.0.5", "user": "esb", "port": 22, "sudo_nopasswd": True}
    payload = {"dev_kvm": True, "dev_vhost_vsock": True, "dev_tun": True}
    result = MagicMock(returncode=0, stdout=json.dumps(payload), stderr="")

    with patch.object(node_cmd, "_ssh_options", return_value=[]), patch(
        "tools.cli.commands.node.subprocess.run", return_value=result
    ) as mock_run:
        node_cmd._doctor_via_ssh(node, args)

    assert "sudo" in mock_run.call_args[0][0]


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


def test_run_doctor_require_up_exits_when_down():
    args = _args(strict=True, require_up=True)
    nodes = [{"name": "node-1", "host": "10.0.0.5", "port": 22}]
    payload = {"ok": True, "node_up": False}

    with patch.object(node_cmd, "_select_nodes", return_value=nodes), patch.object(
        node_cmd, "_doctor_via_ssh", return_value=payload
    ), patch.object(node_cmd, "_render_doctor"):
        with pytest.raises(SystemExit) as exc:
            node_cmd._run_doctor(args)
    assert exc.value.code == 1


def test_run_doctor_require_up_passes_when_up():
    args = _args(strict=True, require_up=True)
    nodes = [{"name": "node-1", "host": "10.0.0.5", "port": 22}]
    payload = {"ok": True, "node_up": True}

    with patch.object(node_cmd, "_select_nodes", return_value=nodes), patch.object(
        node_cmd, "_doctor_via_ssh", return_value=payload
    ), patch.object(node_cmd, "_render_doctor"):
        node_cmd._run_doctor(args)


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
    ), patch.object(node_cmd, "_ensure_wireguard_configs"), patch.object(
        node_cmd, "_prompt_secret", side_effect=AssertionError("prompted")
    ), patch.object(
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
    ), patch.object(node_cmd, "_ensure_wireguard_configs"), patch.object(
        node_cmd, "_prompt_secret", return_value="sudopass"
    ) as mock_prompt, patch.object(
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
    ), patch.object(node_cmd, "_ensure_wireguard_configs"), patch.object(
        node_cmd, "_prompt_secret", return_value="sudopass"
    ) as mock_prompt, patch.object(
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
    ), patch.object(node_cmd, "_ensure_wireguard_configs"), patch.object(
        node_cmd, "_prompt_secret", return_value="sudopass"
    ), patch.object(
        node_cmd, "_run_pyinfra_deploy", side_effect=_capture
    ), patch.object(node_cmd, "_default_identity_file", return_value=None):
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
    ), patch.object(node_cmd, "_ensure_wireguard_configs"), patch.object(
        node_cmd, "_prompt_secret", side_effect=AssertionError("prompted")
    ), patch.object(
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
    ), patch.object(node_cmd, "_ensure_wireguard_configs"), patch.object(
        node_cmd, "_prompt_secret", return_value="sudopass"
    ), patch.object(
        node_cmd, "_run_pyinfra_deploy", side_effect=_capture
    ), patch.object(node_cmd, "_default_identity_file", return_value=None):
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


def test_run_up_requires_firecracker_mode():
    args = _args()
    with patch("tools.cli.commands.node.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_CONTAINERD):
        with pytest.raises(SystemExit) as exc:
            node_cmd._run_up(args)
    assert exc.value.code == 1


def test_run_up_starts_remote_compose(tmp_path, monkeypatch):
    args = _args()
    monkeypatch.setenv("ESB_CONTROL_HOST", "10.99.0.1")
    monkeypatch.setenv("CONTAINER_REGISTRY", "10.99.0.1:5010")
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]
    compose_path = tmp_path / "docker-compose.node.yml"
    remote_path = "/home/esb/.esb/compose/docker-compose.node.yml"

    with patch("tools.cli.commands.node.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_FIRECRACKER), patch(
        "tools.cli.commands.node._select_nodes", return_value=nodes
    ), patch(
        "tools.cli.commands.node.cli_compose.resolve_compose_files", return_value=[compose_path]
    ), patch(
        "tools.cli.commands.node._upload_compose_files", return_value=[remote_path]
    ), patch(
        "tools.cli.commands.node._upload_support_files"
    ), patch(
        "tools.cli.commands.node._run_remote_command"
    ) as mock_run:
        node_cmd._run_up(args)

    assert mock_run.called is True
    command = mock_run.call_args[0][2]
    assert "docker compose" in command
    assert "ESB_CONTROL_HOST=" in command
    assert "CONTAINER_REGISTRY=" in command
    assert f"-f {remote_path}" in command


def test_run_up_pulls_before_start(tmp_path, monkeypatch):
    args = _args()
    monkeypatch.setenv("ESB_CONTROL_HOST", "10.99.0.1")
    monkeypatch.setenv("CONTAINER_REGISTRY", "10.99.0.1:5010")
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]
    compose_path = tmp_path / "docker-compose.node.yml"
    remote_path = "/home/esb/.esb/compose/docker-compose.node.yml"

    with patch("tools.cli.commands.node.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_FIRECRACKER), patch(
        "tools.cli.commands.node._select_nodes", return_value=nodes
    ), patch(
        "tools.cli.commands.node.cli_compose.resolve_compose_files", return_value=[compose_path]
    ), patch(
        "tools.cli.commands.node._upload_compose_files", return_value=[remote_path]
    ), patch(
        "tools.cli.commands.node._upload_support_files"
    ), patch(
        "tools.cli.commands.node._run_remote_command"
    ) as mock_run:
        node_cmd._run_up(args)

    assert mock_run.call_count == 5
    down_cmd = mock_run.call_args_list[0][0][2]
    cleanup_cmd = mock_run.call_args_list[1][0][2]
    runtime_cmd = mock_run.call_args_list[2][0][2]
    pull_cmd = mock_run.call_args_list[3][0][2]
    up_cmd = mock_run.call_args_list[4][0][2]
    assert "docker compose" in down_cmd
    assert " down" in down_cmd
    assert "docker rm -f esb-runtime-node esb-agent esb-local-proxy" in cleanup_cmd
    assert " up -d runtime-node" in runtime_cmd
    assert "docker compose" in pull_cmd
    assert " pull" in pull_cmd
    assert f"-f {remote_path}" in pull_cmd
    assert "docker compose" in up_cmd
    assert " up -d --force-recreate" in up_cmd
    assert f"-f {remote_path}" in up_cmd


def test_run_up_uploads_support_files(tmp_path, monkeypatch):
    args = _args()
    monkeypatch.setenv("ESB_CONTROL_HOST", "10.99.0.1")
    monkeypatch.setenv("CONTAINER_REGISTRY", "10.99.0.1:5010")
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]
    compose_path = tmp_path / "docker-compose.node.yml"
    remote_path = "/home/esb/.esb/compose/docker-compose.node.yml"

    with patch("tools.cli.commands.node.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_FIRECRACKER), patch(
        "tools.cli.commands.node._select_nodes", return_value=nodes
    ), patch(
        "tools.cli.commands.node.cli_compose.resolve_compose_files", return_value=[compose_path]
    ), patch(
        "tools.cli.commands.node._upload_compose_files", return_value=[remote_path]
    ), patch(
        "tools.cli.commands.node._upload_support_files"
    ) as mock_support, patch(
        "tools.cli.commands.node._run_remote_command"
    ):
        node_cmd._run_up(args)

    assert mock_support.called is True


def test_run_up_uses_docker_compose_plugin(tmp_path, monkeypatch):
    args = _args()
    monkeypatch.setenv("ESB_CONTROL_HOST", "10.99.0.1")
    monkeypatch.setenv("CONTAINER_REGISTRY", "10.99.0.1:5010")
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]
    compose_path = tmp_path / "docker-compose.node.yml"
    remote_path = "/home/esb/.esb/compose/docker-compose.node.yml"

    with patch("tools.cli.commands.node.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_FIRECRACKER), patch(
        "tools.cli.commands.node._select_nodes", return_value=nodes
    ), patch(
        "tools.cli.commands.node.cli_compose.resolve_compose_files", return_value=[compose_path]
    ), patch(
        "tools.cli.commands.node._upload_compose_files", return_value=[remote_path]
    ), patch(
        "tools.cli.commands.node._upload_support_files"
    ), patch(
        "tools.cli.commands.node._run_remote_command"
    ) as mock_run:
        node_cmd._run_up(args)

    commands = [call_args[0][2] for call_args in mock_run.call_args_list]
    compose_commands = [cmd for cmd in commands if "compose" in cmd]
    assert compose_commands
    assert all("docker compose" in cmd for cmd in compose_commands)


def test_run_remote_command_quotes_command():
    args = _args()
    node = {"host": "10.0.0.5", "user": "esb", "port": 22}
    command = "mkdir -p ~/.esb/compose"

    with patch.object(node_cmd, "_ssh_options", return_value=[]), patch(
        "tools.cli.commands.node.subprocess.run"
    ) as mock_run:
        node_cmd._run_remote_command(node, args, command)

    expected = ["ssh", "-p", "22", "esb@10.0.0.5", "sh", "-c", node_cmd.shlex.quote(command)]
    assert mock_run.call_args[0][0] == expected


def test_run_up_requires_control_host(monkeypatch):
    args = _args()
    monkeypatch.delenv("ESB_CONTROL_HOST", raising=False)
    monkeypatch.delenv("GATEWAY_INTERNAL_URL", raising=False)
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]

    with patch("tools.cli.commands.node.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_FIRECRACKER), patch(
        "tools.cli.commands.node._select_nodes", return_value=nodes
    ):
        with pytest.raises(SystemExit) as exc:
            node_cmd._run_up(args)
    assert exc.value.code == 1


def test_run_up_uses_gateway_internal_url(tmp_path, monkeypatch):
    args = _args()
    monkeypatch.delenv("ESB_CONTROL_HOST", raising=False)
    monkeypatch.delenv("CONTAINER_REGISTRY", raising=False)
    monkeypatch.setenv("GATEWAY_INTERNAL_URL", "https://10.99.0.1:443")
    nodes = [{"name": "node-1", "host": "10.0.0.5", "user": "esb", "port": 22}]
    compose_path = tmp_path / "docker-compose.node.yml"
    remote_path = "/home/esb/.esb/compose/docker-compose.node.yml"

    with patch("tools.cli.commands.node.runtime_mode.get_mode", return_value=cli_config.ESB_MODE_FIRECRACKER), patch(
        "tools.cli.commands.node._select_nodes", return_value=nodes
    ), patch(
        "tools.cli.commands.node.cli_compose.resolve_compose_files", return_value=[compose_path]
    ), patch(
        "tools.cli.commands.node._upload_compose_files", return_value=[remote_path]
    ), patch(
        "tools.cli.commands.node._upload_support_files"
    ), patch(
        "tools.cli.commands.node._run_remote_command"
    ) as mock_run:
        node_cmd._run_up(args)

    command = mock_run.call_args[0][2]
    assert "ESB_CONTROL_HOST=" in command
    assert "10.99.0.1" in command
    assert "CONTAINER_REGISTRY=" in command
    assert "10.99.0.1:5010" in command
