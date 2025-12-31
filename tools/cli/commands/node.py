import getpass
import json
import os
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import yaml

from tools.cli.core import logging

NODE_CONFIG_PATH = Path.home() / ".esb" / "nodes.yaml"
KNOWN_HOSTS_PATH = Path.home() / ".esb" / "known_hosts"
PYINFRA_DEPLOY_PATH = Path(__file__).resolve().parents[2] / "pyinfra" / "esb_node_provision.py"
ESB_KEY_DIR = Path.home() / ".esb"
ESB_KEY_PATH = ESB_KEY_DIR / "id_ed25519"
ESB_PUBKEY_PATH = ESB_KEY_DIR / "id_ed25519.pub"

REMOTE_PAYLOAD_PY = """import json
import os
import platform
import socket
import subprocess
from pathlib import Path

def _read_first(path: str) -> str:
    try:
        return Path(path).read_text().strip()
    except Exception:
        return ""

def _first_ip() -> str:
    try:
        out = subprocess.check_output(["/bin/sh", "-c", "hostname -I | awk '{print $1}'"])
        return out.decode().strip()
    except Exception:
        return ""

host = os.environ.get("ESB_NODE_HOST") or _first_ip() or socket.gethostname()
user = os.environ.get("ESB_NODE_USER") or os.environ.get("USER") or "root"
port = int(os.environ.get("ESB_NODE_PORT", "22"))
node_id = _read_first("/etc/machine-id") or socket.gethostname()

key = _read_first("/etc/ssh/ssh_host_ed25519_key.pub")
if not key:
    key = _read_first("/etc/ssh/ssh_host_ecdsa_key.pub")

payload = {
    "id": node_id,
    "hostname": socket.gethostname(),
    "host": host,
    "user": user,
    "port": port,
    "ssh_host_key": key,
    "os_release": _read_first("/etc/os-release"),
    "arch": platform.machine(),
}

print(json.dumps(payload, separators=(",", ":")))
"""

REMOTE_PAYLOAD_CMD = f"""python3 - <<'PY'
{REMOTE_PAYLOAD_PY}
PY"""

REMOTE_DOCTOR_PY = """import json
import os
import platform
import subprocess

def _exists(path: str) -> bool:
    return os.path.exists(path)

def _access_rw(path: str) -> bool:
    return os.access(path, os.R_OK | os.W_OK)

def _cmd_exists(cmd: str) -> bool:
    return subprocess.call(["/bin/sh", "-c", f"command -v {cmd} >/dev/null 2>&1"]) == 0

result = {
    "dev_kvm": _exists("/dev/kvm"),
    "dev_kvm_rw": _access_rw("/dev/kvm"),
    "dev_vhost_vsock": _exists("/dev/vhost-vsock"),
    "dev_vhost_vsock_rw": _access_rw("/dev/vhost-vsock"),
    "dev_tun": _exists("/dev/net/tun"),
    "dev_tun_rw": _access_rw("/dev/net/tun"),
    "cmd_firecracker": _cmd_exists("firecracker"),
    "cmd_firecracker_containerd": _cmd_exists("firecracker-containerd"),
    "cmd_firecracker_ctr": _cmd_exists("firecracker-ctr"),
    "cmd_containerd_shim_aws_firecracker": _cmd_exists("containerd-shim-aws-firecracker"),
    "fc_kernel": _exists(os.environ.get("ESB_FC_KERNEL_PATH", "/var/lib/firecracker-containerd/runtime/default-vmlinux.bin")),
    "fc_rootfs": _exists(os.environ.get("ESB_FC_ROOTFS_PATH", "/var/lib/firecracker-containerd/runtime/default-rootfs.img")),
    "fc_containerd_config": _exists(os.environ.get("ESB_FC_CONTAINERD_CONFIG", "/etc/firecracker-containerd/config.toml")),
    "fc_runtime_config": _exists(os.environ.get("ESB_FC_RUNTIME_CONFIG", "/etc/containerd/firecracker-runtime.json")),
    "fc_runc_config": _exists(os.environ.get("ESB_FC_RUNC_CONFIG", "/etc/containerd/firecracker-runc-config.json")),
    "devmapper_pool": _exists("/dev/mapper/" + os.environ.get("ESB_DEVMAPPER_POOL", "fc-dev-pool2")),
    "cmd_dmsetup": _cmd_exists("dmsetup"),
    "cmd_docker": _cmd_exists("docker"),
    "cmd_containerd": _cmd_exists("containerd"),
    "kernel": platform.release(),
    "arch": platform.machine(),
}

print(json.dumps(result, separators=(",", ":")))
"""


def _load_nodes() -> dict[str, Any]:
    if not NODE_CONFIG_PATH.exists():
        return {"version": 1, "nodes": []}
    try:
        data = yaml.safe_load(NODE_CONFIG_PATH.read_text())
    except Exception:
        data = None
    if not isinstance(data, dict):
        return {"version": 1, "nodes": []}
    data.setdefault("version", 1)
    data.setdefault("nodes", [])
    if not isinstance(data["nodes"], list):
        data["nodes"] = []
    return data


def _save_nodes(data: dict[str, Any]) -> None:
    NODE_CONFIG_PATH.parent.mkdir(parents=True, exist_ok=True)
    NODE_CONFIG_PATH.write_text(yaml.safe_dump(data, sort_keys=False))


def _known_hosts_line(host: str, port: int, key: str) -> str | None:
    if not key:
        return None
    key = key.strip()
    if not key:
        return None

    parts = key.split()
    if parts and parts[0].startswith("ssh-"):
        host_entry = host if port == 22 else f"[{host}]:{port}"
        return f"{host_entry} {' '.join(parts[:2])}"
    return key


def _update_known_hosts(line: str) -> None:
    if not line:
        return
    KNOWN_HOSTS_PATH.parent.mkdir(parents=True, exist_ok=True)
    existing = ""
    if KNOWN_HOSTS_PATH.exists():
        existing = KNOWN_HOSTS_PATH.read_text()
    if line in existing:
        return
    with KNOWN_HOSTS_PATH.open("a", encoding="utf-8") as f:
        f.write(line + "\n")


def _ensure_local_keypair() -> tuple[Path, Path]:
    ESB_KEY_DIR.mkdir(parents=True, exist_ok=True)
    if ESB_KEY_PATH.exists() and ESB_PUBKEY_PATH.exists():
        return ESB_KEY_PATH, ESB_PUBKEY_PATH

    cmd = [
        "ssh-keygen",
        "-t",
        "ed25519",
        "-f",
        str(ESB_KEY_PATH),
        "-N",
        "",
        "-q",
    ]
    subprocess.run(cmd, check=True)
    os.chmod(ESB_KEY_PATH, 0o600)
    os.chmod(ESB_PUBKEY_PATH, 0o644)
    return ESB_KEY_PATH, ESB_PUBKEY_PATH


def _default_identity_file() -> str | None:
    if ESB_KEY_PATH.exists():
        return str(ESB_KEY_PATH)
    return None


def _normalize_path(value: str | None) -> str | None:
    if not value:
        return None
    return str(Path(value).expanduser())


def _install_public_key(
    host: str,
    user: str,
    port: int,
    password: str,
    public_key: str,
) -> bool:
    try:
        import paramiko
    except Exception as exc:
        logging.warning(f"paramiko not available: {exc}")
        return False

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(
        host,
        port=port,
        username=user,
        password=password,
        timeout=10,
        allow_agent=False,
        look_for_keys=False,
    )

    try:
        stdin, stdout, _ = client.exec_command("echo $HOME")
        home = stdout.read().decode().strip() or f"/home/{user}"

        client.exec_command("mkdir -p ~/.ssh && chmod 700 ~/.ssh")
        client.exec_command("touch ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys")

        sftp = client.open_sftp()
        auth_path = f"{home}/.ssh/authorized_keys"
        try:
            with sftp.open(auth_path, "r") as f:
                existing = f.read().decode()
        except IOError:
            existing = ""

        if public_key.strip() in existing:
            return True

        with sftp.open(auth_path, "a") as f:
            if existing and not existing.endswith("\n"):
                f.write("\n")
            f.write(public_key.strip() + "\n")
        return True
    finally:
        client.close()


def _run_remote_python(
    host: str,
    user: str,
    port: int,
    password: str,
    script: str,
) -> tuple[str, str] | None:
    try:
        import paramiko
    except Exception as exc:
        logging.warning(f"paramiko not available: {exc}")
        return None

    client = paramiko.SSHClient()
    client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
    client.connect(
        host,
        port=port,
        username=user,
        password=password,
        timeout=10,
        allow_agent=False,
        look_for_keys=False,
    )
    try:
        stdin, stdout, stderr = client.exec_command("python3 -")
        stdin.write(script)
        stdin.channel.shutdown_write()
        return stdout.read().decode().strip(), stderr.read().decode().strip()
    finally:
        client.close()


def _parse_target(args) -> tuple[str | None, str | None, int]:
    host = args.host
    user = args.user
    port = args.port or 22

    if host and "@" in host:
        user_part, host_part = host.split("@", 1)
        host = host_part
        if not user:
            user = user_part

    if not user:
        user = os.environ.get("USER") or "root"

    return host, user, port


def _ssh_options(args, node: dict[str, Any] | None = None) -> list[str]:
    opts = [
        "-o",
        "StrictHostKeyChecking=accept-new",
        "-o",
        f"UserKnownHostsFile={KNOWN_HOSTS_PATH}",
        "-o",
        "BatchMode=yes",
        "-o",
        "ConnectTimeout=10",
    ]
    if getattr(args, "ssh_option", None):
        for option in args.ssh_option:
            opts.extend(["-o", option])
    identity_file = (
        _normalize_path(getattr(args, "identity_file", None))
        or _normalize_path((node or {}).get("identity_file"))
        or _default_identity_file()
    )
    if identity_file and Path(identity_file).exists():
        opts.extend(["-i", identity_file])
    return opts


def _fetch_payload_via_ssh(args) -> str | None:
    host, user, port = _parse_target(args)
    if not host:
        return None
    password = _normalize_secret(getattr(args, "password", None))
    identity_file = _normalize_path(getattr(args, "identity_file", None)) or _default_identity_file()
    if identity_file:
        setattr(args, "identity_file", identity_file)

    if not password and not identity_file:
        password = _prompt_secret(f"{user}@{host} password")
        setattr(args, "password", password)

    if password:
        logging.step(f"Fetching node payload via SSH ({user}@{host}:{port})")
        result = _run_remote_python(host, user, port, password, REMOTE_PAYLOAD_PY)
        if result is None:
            return None
        stdout, stderr = result
        if stderr:
            logging.error("SSH command failed; falling back to manual payload entry.")
            logging.warning(stderr)
            return None
        return stdout

    cmd = [
        "ssh",
        "-p",
        str(port),
        *(_ssh_options(args)),
        f"{user}@{host}",
        "python3",
        "-",
    ]
    logging.step(f"Fetching node payload via SSH ({user}@{host}:{port})")
    result = subprocess.run(
        cmd,
        input=REMOTE_PAYLOAD_PY,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if result.returncode != 0:
        logging.error("SSH command failed; falling back to manual payload entry.")
        logging.warning(result.stderr.strip())
        return None
    return result.stdout.strip()


def _read_payload(args) -> str:
    if args.payload:
        return args.payload
    if args.host:
        fetched = _fetch_payload_via_ssh(args)
        if fetched:
            return fetched
    if not sys.stdin.isatty():
        return sys.stdin.read()

    logging.step("Remote node payload")
    logging.info("Run the following command on the remote node, then paste the output here.")
    print("\n" + REMOTE_PAYLOAD_CMD + "\n")
    logging.info("Paste the JSON payload and press Ctrl-D when finished.")
    return sys.stdin.read()


def _select_nodes(args) -> list[dict[str, Any]]:
    data = _load_nodes()
    nodes = data.get("nodes", [])

    if args.name:
        nodes = [node for node in nodes if node.get("name") == args.name]
    if args.host:
        host, user, port = _parse_target(args)
        matched = [node for node in nodes if node.get("host") == host]
        if matched:
            nodes = matched
        else:
            nodes = [
                {
                    "id": host,
                    "name": host,
                    "host": host,
                    "port": port,
                    "user": user,
                    "ssh_host_key": "",
                    "facts": {},
                }
            ]
    return nodes


def _parse_payload(raw: str) -> dict[str, Any]:
    raw = raw.strip()
    if not raw:
        raise ValueError("payload is empty")
    if raw.startswith("ESB_NODE_PAYLOAD="):
        raw = raw.split("=", 1)[1].strip()
    try:
        data = json.loads(raw)
        if isinstance(data, dict):
            return data
    except json.JSONDecodeError:
        pass
    try:
        data = yaml.safe_load(raw)
        if isinstance(data, dict):
            return data
    except Exception as exc:
        raise ValueError("payload is not valid JSON/YAML") from exc
    raise ValueError("payload is not valid JSON/YAML")


def _resolve_node(payload: dict[str, Any], args) -> dict[str, Any]:
    host_override = args.host
    user_override = args.user

    if host_override and "@" in host_override:
        user_part, host_part = host_override.split("@", 1)
        host_override = host_part
        if not user_override:
            user_override = user_part

    host = host_override or payload.get("host") or payload.get("ip")
    user = user_override or payload.get("user") or payload.get("username")
    port = args.port or payload.get("port") or 22

    if not host:
        raise ValueError("host is required")
    if not user:
        raise ValueError("user is required")

    node_id = payload.get("id") or payload.get("node_id") or payload.get("hostname") or host
    name = args.name or payload.get("name") or payload.get("hostname") or node_id

    ssh_host_key = payload.get("ssh_host_key") or ""
    identity_file = _normalize_path(getattr(args, "identity_file", None))

    node = {
        "id": str(node_id),
        "name": str(name),
        "host": str(host),
        "port": int(port),
        "user": str(user),
        "ssh_host_key": ssh_host_key.strip(),
        "identity_file": identity_file,
        "facts": {
            "hostname": payload.get("hostname"),
            "os_release": payload.get("os_release"),
            "arch": payload.get("arch"),
        },
        "added_at": datetime.now(timezone.utc).isoformat(),
    }
    return node


def _upsert_node(nodes: list[dict[str, Any]], node: dict[str, Any]) -> bool:
    for idx, existing in enumerate(nodes):
        if existing.get("id") == node["id"] or existing.get("host") == node["host"]:
            nodes[idx] = node
            return True
    nodes.append(node)
    return False


def _run_add(args) -> None:
    raw = _read_payload(args)
    payload = _parse_payload(raw)
    node = _resolve_node(payload, args)

    setup_key = bool(args.host) and not getattr(args, "skip_key_setup", False)
    if setup_key and not node.get("identity_file"):
        try:
            key_path, pub_path = _ensure_local_keypair()
        except Exception as exc:
            logging.warning(f"Failed to generate SSH key: {exc}")
        else:
            public_key = pub_path.read_text().strip()
            password = _normalize_secret(getattr(args, "password", None))
            if not password:
                password = _prompt_secret(f"{node['user']}@{node['host']} password")
            logging.step("Installing SSH key on remote node")
            installed = _install_public_key(
                node["host"], node["user"], int(node["port"]), password, public_key
            )
            if installed:
                node["identity_file"] = str(key_path)
                logging.success(f"SSH key installed for {node['user']}@{node['host']}")
            else:
                logging.warning("Failed to install SSH key; password auth remains required.")

    data = _load_nodes()
    updated = _upsert_node(data["nodes"], node)
    _save_nodes(data)

    host_key_line = _known_hosts_line(node["host"], node["port"], node.get("ssh_host_key", ""))
    if host_key_line:
        _update_known_hosts(host_key_line)

    action = "Updated" if updated else "Added"
    logging.success(f"{action} node: {node['name']} ({node['host']}:{node['port']})")


def _doctor_via_ssh(node: dict[str, Any], args) -> dict[str, Any]:
    host = node["host"]
    user = node["user"]
    port = int(node.get("port", 22))

    cmd = [
        "ssh",
        "-p",
        str(port),
        *(_ssh_options(args, node)),
        f"{user}@{host}",
        "python3",
        "-",
    ]

    try:
        result = subprocess.run(
            cmd,
            input=REMOTE_DOCTOR_PY,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=30,
        )
    except subprocess.TimeoutExpired:
        return {
            "ok": False,
            "error": "ssh command timed out",
        }

    if result.returncode != 0:
        return {
            "ok": False,
            "error": result.stderr.strip() or "ssh failed",
        }

    try:
        payload = json.loads(result.stdout.strip())
    except json.JSONDecodeError:
        return {
            "ok": False,
            "error": "invalid JSON response",
        }

    required = payload.get("dev_kvm") and payload.get("dev_vhost_vsock") and payload.get("dev_tun")
    payload["ok"] = bool(required)
    return payload


def _render_doctor(name: str, host: str, port: int, result: dict[str, Any]) -> None:
    header = f"{name} ({host}:{port})"
    if not result.get("ok"):
        logging.error(f"[FAIL] {header}")
    else:
        logging.success(f"[OK] {header}")

    if result.get("error"):
        logging.warning(f"  error: {result['error']}")
        return

    checks = [
        ("dev_kvm", result.get("dev_kvm")),
        ("dev_kvm_rw", result.get("dev_kvm_rw")),
        ("dev_vhost_vsock", result.get("dev_vhost_vsock")),
        ("dev_vhost_vsock_rw", result.get("dev_vhost_vsock_rw")),
        ("dev_tun", result.get("dev_tun")),
        ("dev_tun_rw", result.get("dev_tun_rw")),
        ("cmd_firecracker", result.get("cmd_firecracker")),
        ("cmd_firecracker_containerd", result.get("cmd_firecracker_containerd")),
        ("cmd_firecracker_ctr", result.get("cmd_firecracker_ctr")),
        ("cmd_containerd_shim_aws_firecracker", result.get("cmd_containerd_shim_aws_firecracker")),
        ("fc_kernel", result.get("fc_kernel")),
        ("fc_rootfs", result.get("fc_rootfs")),
        ("fc_containerd_config", result.get("fc_containerd_config")),
        ("fc_runtime_config", result.get("fc_runtime_config")),
        ("fc_runc_config", result.get("fc_runc_config")),
        ("devmapper_pool", result.get("devmapper_pool")),
        ("cmd_dmsetup", result.get("cmd_dmsetup")),
        ("cmd_docker", result.get("cmd_docker")),
        ("cmd_containerd", result.get("cmd_containerd")),
    ]
    for key, value in checks:
        status = "OK" if value else "MISSING"
        print(f"  - {key}: {status}")


def _run_doctor(args) -> None:
    nodes = _select_nodes(args)
    if not nodes:
        logging.error("No nodes found. Run `esb node add` first.")
        sys.exit(1)

    failures = 0
    for node in nodes:
        result = _doctor_via_ssh(node, args)
        _render_doctor(node["name"], node["host"], node.get("port", 22), result)
        if not result.get("ok"):
            failures += 1

    if failures and getattr(args, "strict", False):
        sys.exit(1)


def _ensure_pyinfra() -> None:
    try:
        import pyinfra  # noqa: F401
    except Exception as exc:
        raise RuntimeError("pyinfra is not installed. Run `uv sync` first.") from exc


def _normalize_secret(value: str | None) -> str | None:
    if value is None:
        return None
    value = value.strip()
    return value or None


def _prompt_secret(label: str) -> str:
    return getpass.getpass(f"{label}: ")


def _set_pyinfra_verbosity(state, verbosity: int) -> None:
    if verbosity > 0:
        state.print_fact_info = True
        state.print_noop_info = True
    if verbosity > 1:
        state.print_input = True
        state.print_fact_input = True
    if verbosity > 2:
        state.print_output = True
        state.print_fact_output = True


def _build_inventory_hosts(
    nodes: list[dict[str, Any]],
    args,
    ssh_password: str | None,
    sudo_password: str | None,
) -> list[tuple[str, dict[str, Any]]]:
    entries: list[tuple[str, dict[str, Any]]] = []
    for node in nodes:
        host = node["host"]
        user = args.user or node.get("user") or "root"
        port = int(args.port or node.get("port") or 22)
        node_sudo_nopasswd = bool(node.get("sudo_nopasswd"))

        data: dict[str, Any] = {
            "ssh_hostname": host,
            "ssh_port": port,
            "ssh_user": user,
            "ssh_known_hosts_file": str(KNOWN_HOSTS_PATH),
            "ssh_strict_host_key_checking": "accept-new",
            "esb_user": user,
        }

        if getattr(args, "firecracker_version", None):
            data["esb_firecracker_version"] = args.firecracker_version
        if getattr(args, "firecracker_containerd_ref", None):
            data["esb_firecracker_containerd_ref"] = args.firecracker_containerd_ref
        if getattr(args, "firecracker_install_dir", None):
            data["esb_firecracker_install_dir"] = args.firecracker_install_dir
        if getattr(args, "firecracker_runtime_dir", None):
            data["esb_firecracker_runtime_dir"] = args.firecracker_runtime_dir
        if getattr(args, "firecracker_kernel_url", None):
            data["esb_firecracker_kernel_url"] = args.firecracker_kernel_url
        if getattr(args, "firecracker_rootfs_url", None):
            data["esb_firecracker_rootfs_url"] = args.firecracker_rootfs_url
        if getattr(args, "firecracker_kernel_path", None):
            data["esb_firecracker_kernel_path"] = args.firecracker_kernel_path
        if getattr(args, "firecracker_rootfs_path", None):
            data["esb_firecracker_rootfs_path"] = args.firecracker_rootfs_path
        if getattr(args, "devmapper_pool", None):
            data["esb_devmapper_pool"] = args.devmapper_pool
        if getattr(args, "devmapper_dir", None):
            data["esb_devmapper_dir"] = args.devmapper_dir
        if getattr(args, "devmapper_data_size", None):
            data["esb_devmapper_data_size"] = args.devmapper_data_size
        if getattr(args, "devmapper_meta_size", None):
            data["esb_devmapper_meta_size"] = args.devmapper_meta_size
        if getattr(args, "devmapper_base_image_size", None):
            data["esb_devmapper_base_image_size"] = args.devmapper_base_image_size
        if getattr(args, "devmapper_udev", None) is not None:
            data["esb_devmapper_udev"] = bool(args.devmapper_udev)

        identity_file = (
            _normalize_path(args.identity_file)
            or _normalize_path(node.get("identity_file"))
            or _default_identity_file()
        )
        if identity_file and Path(identity_file).exists():
            data["ssh_key"] = identity_file
        if ssh_password:
            data["ssh_password"] = ssh_password
        if user != "root":
            data["_sudo"] = True
            if sudo_password:
                data["_sudo_password"] = sudo_password
        if getattr(args, "sudo_nopasswd", False) or node_sudo_nopasswd:
            data["esb_sudo_nopasswd"] = True

        entries.append((host, data))

    return entries


def _run_pyinfra_deploy(
    inventory_hosts: list[tuple[str, dict[str, Any]]],
    deploy_path: Path,
    verbosity: int,
) -> None:
    import pyinfra
    from pyinfra import logger as pyinfra_logger
    from pyinfra.api import Config, State
    from pyinfra.api.connect import connect_all, disconnect_all
    from pyinfra.api.inventory import Inventory
    from pyinfra.api.operations import run_ops
    from pyinfra.api.state import StateStage
    from pyinfra.context import ctx_config, ctx_host, ctx_inventory, ctx_state

    pyinfra_logger.disabled = False
    pyinfra.is_cli = False

    config = Config()
    state = State(check_for_changes=False)
    state.cwd = str(Path.cwd())
    _set_pyinfra_verbosity(state, verbosity)

    inventory = Inventory((inventory_hosts, {}))
    ctx_config.set(config)
    ctx_inventory.set(inventory)
    ctx_state.set(state)
    state.init(inventory, config)

    deploy_code = compile(deploy_path.read_text(), str(deploy_path), "exec")
    old_exec = state.current_exec_filename
    old_deploy = state.current_deploy_filename
    state.current_deploy_filename = str(deploy_path)

    try:
        state.set_stage(StateStage.Connect)
        connect_all(state)
        state.set_stage(StateStage.Prepare)
        with ctx_state.use(state):
            for deploy_host in state.inventory.iter_active_hosts():
                with ctx_host.use(deploy_host):
                    state.current_exec_filename = str(deploy_path)
                    exec(deploy_code, {"__file__": str(deploy_path)})
        state.set_stage(StateStage.Execute)
        run_ops(state, serial=False, no_wait=False)
    finally:
        state.current_exec_filename = old_exec
        state.current_deploy_filename = old_deploy
        try:
            state.set_stage(StateStage.Disconnect)
        except Exception:
            pass
        disconnect_all(state)


def _run_provision(args) -> None:
    _ensure_pyinfra()
    nodes = _select_nodes(args)
    if not nodes:
        logging.error("No nodes found. Run `esb node add` first.")
        sys.exit(1)

    ssh_password = _normalize_secret(args.password)
    sudo_password = _normalize_secret(args.sudo_password)
    has_identity = bool(
        _normalize_path(args.identity_file)
        or _default_identity_file()
        or any(node.get("identity_file") for node in nodes)
    )
    stored_nopasswd = all(node.get("sudo_nopasswd") for node in nodes) if nodes else False

    if ssh_password is None and not has_identity:
        ssh_password = _prompt_secret("SSH password")

    needs_sudo = any((args.user or node.get("user") or "root") != "root" for node in nodes)
    if sudo_password is None and needs_sudo:
        if stored_nopasswd:
            logging.warning(
                "Sudo password not provided; assuming passwordless sudo is already configured."
            )
        elif getattr(args, "sudo_nopasswd", False):
            sudo_password = ssh_password or _prompt_secret(
                "Sudo password (required to enable NOPASSWD)"
            )
        else:
            sudo_password = ssh_password or _prompt_secret("Sudo password")

    inventory_hosts = _build_inventory_hosts(nodes, args, ssh_password, sudo_password)
    deploy_path = PYINFRA_DEPLOY_PATH

    if not deploy_path.exists():
        logging.error(f"Missing pyinfra deploy: {deploy_path}")
        sys.exit(1)

    verbosity = int(getattr(args, "verbose", 0) or 0)
    logging.step("Provisioning node(s) via pyinfra")
    _run_pyinfra_deploy(inventory_hosts, deploy_path, verbosity)

    if getattr(args, "sudo_nopasswd", False):
        data = _load_nodes()
        updated = False
        for node in data.get("nodes", []):
            if args.name and node.get("name") != args.name:
                continue
            if args.host and node.get("host") != _parse_target(args)[0]:
                continue
            node["sudo_nopasswd"] = True
            updated = True
        if updated:
            _save_nodes(data)


def run(args) -> None:
    if args.node_command == "add":
        _run_add(args)
        return
    if args.node_command == "doctor":
        _run_doctor(args)
        return
    if args.node_command == "provision":
        _run_provision(args)
        return

    logging.error(f"Unsupported node command: {args.node_command}")
    sys.exit(1)
