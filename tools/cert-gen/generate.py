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
import yaml

from tools.branding.branding import BrandingError, derive_branding

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


def generate_cert(config, output_dir, mkcert_path: str):
    cert_cfg = config.get("certificate", {})
    host_cfg = config.get("hosts", {})

    output_dir = os.path.expanduser(output_dir)
    os.makedirs(output_dir, exist_ok=True)
    if not os.access(output_dir, os.W_OK):
        raise RuntimeError(
            f"Certificate output directory is not writable: {output_dir}. "
            "Fix ownership or permissions and retry."
        )

    cert_file = os.path.join(output_dir, cert_cfg.get("filename_cert", "server.crt"))
    key_file = os.path.join(output_dir, cert_cfg.get("filename_key", "server.key"))

    # ドメインとIPの収集
    domains = host_cfg.get("domains", [])
    ips = host_cfg.get("ips", [])

    if host_cfg.get("include_local_ip", False):
        local_ip = get_local_ip()
        if local_ip not in ips and local_ip != "127.0.0.1":
            ips.append(local_ip)

    # mkcert引数構築
    cmd = [mkcert_path, "-cert-file", cert_file, "-key-file", key_file]
    cmd.extend(domains)
    cmd.extend(ips)

    print(f"Generating certificates in {output_dir}...")
    print(f"Domains: {domains}")
    print(f"IPs: {ips}")

    subprocess.check_call(cmd)
    print("Certificate generation complete.")


def resolve_repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def load_branding(repo_root: Path):
    branding_path = repo_root / "config" / "branding.yaml"
    if not branding_path.exists():
        raise RuntimeError(f"Branding config not found: {branding_path}")

    data = yaml.safe_load(branding_path.read_text()) or {}
    brand = str(data.get("brand", "")).strip()
    if not brand:
        raise RuntimeError("Branding config must define 'brand'")

    try:
        return derive_branding(brand)
    except BrandingError as exc:
        raise RuntimeError(f"Invalid branding config: {exc}") from exc


def resolve_output_dir(config: dict, branding) -> str:
    cert_cfg = config.get("certificate", {})
    output_dir = cert_cfg.get("output_dir", DEFAULT_CERT_OUTPUT_DIR)
    if branding and output_dir == DEFAULT_CERT_OUTPUT_DIR:
        output_dir = f"~/{branding.paths['home_dir']}/certs"
    return output_dir


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
    config_path = Path(args.config)
    if not config_path.is_absolute():
        config_path = repo_root / config_path
    config = toml.load(config_path)
    branding = load_branding(repo_root)
    cert_cfg = config.get("certificate", {})
    output_dir = os.path.expanduser(resolve_output_dir(config, branding))

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

    # Check and generate Server Certs
    cert_file = os.path.join(output_dir, cert_cfg.get("filename_cert", "server.crt"))
    key_file = os.path.join(output_dir, cert_cfg.get("filename_key", "server.key"))

    if args.force or not (os.path.exists(cert_file) and os.path.exists(key_file)):
        generate_cert(config, output_dir, mkcert_path)
    else:
        print(
            f"Certificates exist at {output_dir}. Skipping generation. Use --force to regenerate."
        )
