#!/usr/bin/env python3
import argparse
import getpass
import os
import shutil
import socket
import subprocess
import sys
from pathlib import Path
from types import ModuleType

import toml

try:
    import grp as _grp
except ImportError:  # pragma: no cover - non-POSIX
    grp_module: ModuleType | None = None
else:
    grp_module = _grp

DEFAULT_CERT_OUTPUT_DIR = "~/.local/share/certs"


def get_local_ip():
    try:
        # 8.8.8.8にダミー接続して自身のルートIPを取得 (パケット送信はしない)
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("8.8.8.8", 80))
            return s.getsockname()[0]
    except Exception:
        return "127.0.0.1"


def resolve_mkcert_path() -> str:
    mkcert_path = shutil.which("mkcert")
    if not mkcert_path:
        return ""
    return mkcert_path


def resolve_sudo_path() -> str:
    sudo_path = shutil.which("sudo")
    if not sudo_path:
        return ""
    return sudo_path


def check_mkcert(mkcert_path: str):
    if not mkcert_path:
        print("Error: mkcert not found.")
        print("Please install mkcert via mise or system package manager.")
        exit(1)


def install_root_ca(mkcert_path: str, output_dir: str):
    print("Installing local Root CA...")
    try:
        subprocess.check_call([mkcert_path, "-install"])
    except subprocess.CalledProcessError as exc:
        sudo_path = resolve_sudo_path()
        if not sudo_path:
            raise RuntimeError(
                "mkcert -install failed and sudo is not available. "
                f"Try running: {mkcert_path} -install"
            ) from exc
        print("mkcert -install failed, retrying with sudo...")
        env = os.environ.copy()
        subprocess.check_call([sudo_path, "-E", mkcert_path, "-install"], env=env)
        ensure_user_ownership(output_dir)


def collect_hosts(host_cfg: dict, local_ip: str | None = None) -> tuple[list[str], list[str]]:
    # Copy lists to avoid mutating TOML config structures.
    domains = list(host_cfg.get("domains", []))
    ips = list(host_cfg.get("ips", []))

    if host_cfg.get("include_local_ip", False):
        if local_ip is None:
            local_ip = get_local_ip()
        if local_ip and local_ip != "127.0.0.1" and local_ip not in ips:
            ips.append(local_ip)

    return domains, ips


def build_mkcert_command(
    mkcert_path: str,
    cert_file: str,
    key_file: str,
    domains: list[str],
    ips: list[str],
    extra_args: list[str] | None = None,
) -> list[str]:
    cmd = [mkcert_path]
    if extra_args:
        cmd.extend(extra_args)
    cmd.extend(["-cert-file", cert_file, "-key-file", key_file])
    cmd.extend(domains)
    cmd.extend(ips)
    return cmd


def generate_cert(
    cert_file: str,
    key_file: str,
    domains: list[str],
    ips: list[str],
    mkcert_path: str,
    label: str,
    extra_args: list[str] | None = None,
) -> None:
    output_dir = os.path.dirname(cert_file)
    cmd = build_mkcert_command(mkcert_path, cert_file, key_file, domains, ips, extra_args)

    print(f"Generating {label} certificate in {output_dir}...")
    print(f"Certificate: {os.path.basename(cert_file)}")
    print(f"Domains: {domains}")
    print(f"IPs: {ips}")

    subprocess.check_call(cmd)
    print(f"{label.capitalize()} certificate generation complete.")


def resolve_host_cfg(config: dict, key: str, fallback: dict) -> dict:
    host_cfg = config.get(key)
    return fallback if not host_cfg else host_cfg


def resolve_repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def ensure_output_dir(output_dir: str) -> None:
    expanded = os.path.expanduser(output_dir)
    owner_hint = str(Path(expanded).parent)
    try:
        os.makedirs(expanded, exist_ok=True)
    except PermissionError as exc:
        if not attempt_fix_permissions(owner_hint):
            raise RuntimeError(
                f"Certificate output directory is not writable: {expanded}. "
                f"Fix ownership (e.g. sudo chown -R $USER:$USER {owner_hint}) and retry."
            ) from exc
    if not os.access(expanded, os.W_OK):
        if not attempt_fix_permissions(owner_hint):
            raise RuntimeError(
                f"Certificate output directory is not writable: {expanded}. "
                f"Fix ownership (e.g. sudo chown -R $USER:$USER {owner_hint}) and retry."
            )
    if not os.access(expanded, os.W_OK):
        raise RuntimeError(
            f"Certificate output directory is not writable: {expanded}. "
            f"Fix ownership (e.g. sudo chown -R $USER:$USER {owner_hint}) and retry."
        )


def current_user_group() -> tuple[str, str]:
    user = os.environ.get("USER") or getpass.getuser()
    group = ""
    if grp_module is not None:
        try:
            group = grp_module.getgrgid(os.getgid()).gr_name
        except KeyError:
            group = ""
    if not group:
        group = user
    return user, group


def attempt_fix_permissions(path: str) -> bool:
    sudo_path = resolve_sudo_path()
    if not sudo_path:
        return False
    user, group = current_user_group()
    print(f"Attempting to fix permissions with sudo for {path}...")
    try:
        subprocess.check_call([sudo_path, "chown", "-R", f"{user}:{group}", path])
    except subprocess.CalledProcessError:
        return False
    return True


def ensure_user_ownership(output_dir: str) -> None:
    expanded = os.path.expanduser(output_dir)
    if os.access(expanded, os.W_OK):
        return
    attempt_fix_permissions(expanded)


def load_env_defaults(root: Path) -> dict[str, str]:
    """Read branding defaults from config/defaults.env"""
    path = root / "config" / "defaults.env"
    if not path.exists():
        return {}
    defaults = {}
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        defaults[key.strip()] = value.strip()
    return defaults


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Generate development certificates using mkcert")
    parser.add_argument(
        "--config", default="tools/cert-gen/config.toml", help="Path to config file"
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Force regeneration of certificates and CA installation",
    )
    args = parser.parse_args()

    repo_root = resolve_repo_root()
    defaults = load_env_defaults(repo_root)
    cli_cmd = defaults.get("CLI_CMD", "esb")

    config_path = Path(args.config)
    if not config_path.is_absolute():
        config_path = repo_root / config_path

    config = {}
    if config_path.exists():
        config = toml.load(config_path)

    cert_cfg = config.get("certificate", {})
    host_cfg = config.get("hosts", {})

    # Derivation logic for output_dir:
    # 1. Flag (future)
    # 2. config.toml [certificate] output_dir
    # 3. Dynamic branding: ~/.{CLI_CMD}/certs
    # 4. Fallback: ~/.local/share/certs
    output_dir = cert_cfg.get("output_dir")
    if not output_dir:
        output_dir = f"~/.{cli_cmd}/certs"

    output_dir = os.path.expanduser(output_dir)

    # Force CAROOT to follow branding output_dir to avoid stale env vars.
    if os.environ.get("CAROOT") and os.environ["CAROOT"] != output_dir:
        print(f"Overriding CAROOT to {output_dir} (was {os.environ['CAROOT']}).")
    os.environ["CAROOT"] = output_dir
    ensure_output_dir(output_dir)

    mkcert_path = resolve_mkcert_path()
    check_mkcert(mkcert_path)

    # Check and install Root CA
    root_ca_path = os.path.join(os.environ["CAROOT"], "rootCA.pem")

    # Safety check: Docker sometimes creates a directory if the mount target is missing
    if os.path.isdir(root_ca_path):
        print(f"ERROR: {root_ca_path} is a directory, but it should be a file.")
        print("This often happens when Docker mistakenly creates a directory for a file mount.")
        print(f"Please run: sudo rm -rf {root_ca_path}")
        sys.exit(1)

    if args.force or not os.path.exists(root_ca_path):
        install_root_ca(mkcert_path, output_dir)
    else:
        print(f"Root CA exists at {root_ca_path}. Skipping installation. Use --force to reinstall.")

    # Convert/Copy Root CA to .crt for easier use in containers (update-ca-certificates)
    root_ca_crt = os.path.join(os.environ["CAROOT"], "rootCA.crt")
    if os.path.exists(root_ca_path):
        if args.force or not os.path.exists(root_ca_crt):
            print(f"Creating {root_ca_crt} from {root_ca_path}...")
            shutil.copy2(root_ca_path, root_ca_crt)

    # Check and generate Server/Client Certs
    cert_file = os.path.join(output_dir, cert_cfg.get("filename_cert", "server.crt"))
    key_file = os.path.join(output_dir, cert_cfg.get("filename_key", "server.key"))
    client_cert_file = os.path.join(output_dir, cert_cfg.get("filename_client_cert", "client.crt"))
    client_key_file = os.path.join(output_dir, cert_cfg.get("filename_client_key", "client.key"))

    server_domains, server_ips = collect_hosts(host_cfg)
    client_host_cfg = resolve_host_cfg(config, "client_hosts", host_cfg)
    client_domains, client_ips = collect_hosts(client_host_cfg)

    if args.force or not (os.path.exists(cert_file) and os.path.exists(key_file)):
        generate_cert(cert_file, key_file, server_domains, server_ips, mkcert_path, "server")
    else:
        print(
            f"Server certificates exist at {output_dir}. "
            "Skipping generation. Use --force to regenerate."
        )

    if args.force or not (os.path.exists(client_cert_file) and os.path.exists(client_key_file)):
        generate_cert(
            client_cert_file,
            client_key_file,
            client_domains,
            client_ips,
            mkcert_path,
            "client",
            extra_args=["-client"],
        )
    else:
        print(
            f"Client certificates exist at {output_dir}. "
            "Skipping generation. Use --force to regenerate."
        )
