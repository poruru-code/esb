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

ROOT_CA_CERT_FILENAME = "rootCA.crt"
ROOT_CA_KEY_FILENAME = "rootCA.key"


def get_local_ip():
    try:
        # 8.8.8.8にダミー接続して自身のルートIPを取得 (パケット送信はしない)
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("8.8.8.8", 80))
            return s.getsockname()[0]
    except Exception:
        return "127.0.0.1"


def resolve_step_path() -> str:
    step_path = shutil.which("step")
    if not step_path:
        return ""
    return step_path


def resolve_sudo_path() -> str:
    sudo_path = shutil.which("sudo")
    if not sudo_path:
        return ""
    return sudo_path


def check_step(step_path: str):
    if not step_path:
        print("Error: step CLI not found.")
        print("Please install step via mise or system package manager.")
        exit(1)


def install_root_ca(step_path: str, root_ca_cert: str, output_dir: str):
    print("Installing local Root CA...")
    cmd = [step_path, "certificate", "install", root_ca_cert]
    try:
        subprocess.check_call(cmd)
    except subprocess.CalledProcessError as exc:
        sudo_path = resolve_sudo_path()
        if not sudo_path:
            message = (
                "step certificate install failed and sudo is not available. "
                f"Try running: {step_path} certificate install {root_ca_cert}"
            )
            raise RuntimeError(message) from exc
        print("step certificate install failed, retrying with sudo...")
        env = os.environ.copy()
        subprocess.check_call([sudo_path, "-E", *cmd], env=env)
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


def normalize_validity(value: object | None) -> str | None:
    if value is None:
        return None
    text = str(value).strip()
    return text or None


def require_validity(value: str | None, name: str) -> str:
    if not value:
        raise RuntimeError(f"certificate.{name} is required in config")
    return value


def resolve_subject(domains: list[str], ips: list[str], fallback: str) -> str:
    if domains:
        return domains[0]
    if ips:
        return ips[0]
    return fallback


def dedupe_sans(domains: list[str], ips: list[str], subject: str) -> list[str]:
    sans: list[str] = []
    seen: set[str] = set()
    for entry in [*domains, *ips]:
        if entry not in seen:
            sans.append(entry)
            seen.add(entry)
    if subject and subject not in seen:
        sans.insert(0, subject)
    return sans


def build_step_root_ca_command(
    step_path: str,
    subject: str,
    cert_file: str,
    key_file: str,
    not_after: str | None = None,
    overwrite: bool = False,
) -> list[str]:
    cmd = [
        step_path,
        "certificate",
        "create",
        subject,
        cert_file,
        key_file,
        "--profile",
        "root-ca",
        "--no-password",
        "--insecure",
    ]
    if not_after:
        cmd.extend(["--not-after", not_after])
    if overwrite:
        cmd.append("--force")
    return cmd


def build_step_leaf_command(
    step_path: str,
    subject: str,
    cert_file: str,
    key_file: str,
    sans: list[str],
    ca_cert: str,
    ca_key: str,
    not_after: str | None = None,
    overwrite: bool = False,
) -> list[str]:
    cmd = [
        step_path,
        "certificate",
        "create",
        subject,
        cert_file,
        key_file,
        "--profile",
        "leaf",
        "--ca",
        ca_cert,
        "--ca-key",
        ca_key,
        "--no-password",
        "--insecure",
    ]
    if not_after:
        cmd.extend(["--not-after", not_after])
    for san in sans:
        cmd.extend(["--san", san])
    if overwrite:
        cmd.append("--force")
    return cmd


def generate_root_ca(
    cert_file: str,
    key_file: str,
    step_path: str,
    subject: str,
    not_after: str | None,
    overwrite: bool = False,
) -> None:
    output_dir = os.path.dirname(cert_file)
    cmd = build_step_root_ca_command(
        step_path,
        subject,
        cert_file,
        key_file,
        not_after,
        overwrite=overwrite,
    )

    print(f"Generating root CA in {output_dir}...")
    print(f"Certificate: {os.path.basename(cert_file)}")
    print(f"Subject: {subject}")
    if not_after:
        print(f"Valid for: {not_after}")

    subprocess.check_call(cmd)
    print("Root CA generation complete.")


def generate_leaf_cert(
    cert_file: str,
    key_file: str,
    sans: list[str],
    step_path: str,
    ca_cert: str,
    ca_key: str,
    label: str,
    subject: str,
    not_after: str | None,
    overwrite: bool = False,
) -> None:
    output_dir = os.path.dirname(cert_file)
    cmd = build_step_leaf_command(
        step_path,
        subject,
        cert_file,
        key_file,
        sans,
        ca_cert,
        ca_key,
        not_after,
        overwrite=overwrite,
    )

    print(f"Generating {label} certificate in {output_dir}...")
    print(f"Certificate: {os.path.basename(cert_file)}")
    print(f"Subject: {subject}")
    print(f"SANs: {sans}")
    if not_after:
        print(f"Valid for: {not_after}")

    subprocess.check_call(cmd)
    print(f"{label.capitalize()} certificate generation complete.")


def resolve_host_cfg(config: dict, key: str, fallback: dict) -> dict:
    host_cfg = config.get(key)
    return fallback if not host_cfg else host_cfg


def resolve_repo_root() -> Path:
    return Path(__file__).resolve().parents[2]


def resolve_brand_home_dir() -> str:
    repo_root = resolve_repo_root()
    repo_root_str = str(repo_root)
    if repo_root_str not in sys.path:
        sys.path.insert(0, repo_root_str)

    from e2e.runner.branding import brand_home_dir

    return brand_home_dir()


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


def current_user_chown_spec() -> str:
    getuid = getattr(os, "getuid", None)
    getgid = getattr(os, "getgid", None)
    if callable(getuid) and callable(getgid):
        return f"{getuid()}:{getgid()}"
    user, group = current_user_group()
    return f"{user}:{group}"


def attempt_fix_permissions(path: str) -> bool:
    sudo_path = resolve_sudo_path()
    if not sudo_path:
        return False
    user_spec = current_user_chown_spec()
    print(f"Attempting to fix permissions with sudo for {path}...")
    try:
        subprocess.check_call([sudo_path, "chown", "-R", user_spec, "--", path])
    except subprocess.CalledProcessError:
        return False
    return True


def ensure_user_ownership(output_dir: str) -> None:
    expanded = os.path.expanduser(output_dir)
    if not os.path.exists(expanded):
        return

    needs_fix = not os.access(expanded, os.W_OK)
    scan_error = False

    def onerror(_: OSError) -> None:
        nonlocal scan_error
        scan_error = True

    if not needs_fix:
        for root, dirs, _files in os.walk(expanded, onerror=onerror):
            for name in dirs:
                candidate = os.path.join(root, name)
                if not os.access(candidate, os.W_OK):
                    needs_fix = True
                    break
            if needs_fix:
                break

    if not needs_fix and not scan_error:
        return
    if not attempt_fix_permissions(expanded):
        print(
            f"Warning: failed to fix ownership/permissions under {expanded}. "
            "Some cleanup operations may require sudo.",
            file=sys.stderr,
        )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Generate development certificates using step-cli")
    parser.add_argument(
        "--config", default="tools/cert-gen/config.toml", help="Path to config file"
    )
    parser.add_argument(
        "--output-dir",
        help=(
            "Override certificate output directory. "
            "Takes precedence over config.toml [certificate].output_dir and CERT_DIR env."
        ),
    )
    parser.add_argument(
        "--force",
        action="store_true",
        help="Force regeneration of certificates and CA installation",
    )
    parser.add_argument(
        "--skip-root-ca-install",
        action="store_true",
        help="Skip local trust-store installation of the generated Root CA",
    )
    args = parser.parse_args()

    repo_root = resolve_repo_root()
    brand_dir = repo_root / resolve_brand_home_dir()

    config_path = Path(args.config)
    if not config_path.is_absolute():
        config_path = repo_root / config_path

    config = {}
    if config_path.exists():
        config = toml.load(config_path)

    cert_cfg = config.get("certificate", {})
    host_cfg = config.get("hosts", {})

    # Derivation logic for output_dir:
    # 1. --output-dir
    # 2. config.toml [certificate] output_dir
    # 3. CERT_DIR env
    # 4. Repo root: <repo_root>/.<brand>/certs
    output_dir = args.output_dir or cert_cfg.get("output_dir") or os.environ.get("CERT_DIR")
    if not output_dir:
        output_dir = str(brand_dir / "certs")

    output_dir = os.path.expanduser(output_dir)

    # Force CAROOT to follow branding output_dir to avoid stale env vars.
    if os.environ.get("CAROOT") and os.environ["CAROOT"] != output_dir:
        print(f"Overriding CAROOT to {output_dir} (was {os.environ['CAROOT']}).")
    os.environ["CAROOT"] = output_dir
    ensure_output_dir(output_dir)

    step_path = resolve_step_path()
    check_step(step_path)

    # Check and install Root CA
    root_ca_cert = os.path.join(os.environ["CAROOT"], ROOT_CA_CERT_FILENAME)
    root_ca_key = os.path.join(os.environ["CAROOT"], ROOT_CA_KEY_FILENAME)

    # Safety check: Docker sometimes creates a directory if the mount target is missing
    if os.path.isdir(root_ca_cert):
        print(f"ERROR: {root_ca_cert} is a directory, but it should be a file.")
        print("This often happens when Docker mistakenly creates a directory for a file mount.")
        print(f"Please run: sudo rm -rf {root_ca_cert}")
        sys.exit(1)
    if os.path.isdir(root_ca_key):
        print(f"ERROR: {root_ca_key} is a directory, but it should be a file.")
        print("This often happens when Docker mistakenly creates a directory for a file mount.")
        print(f"Please run: sudo rm -rf {root_ca_key}")
        sys.exit(1)

    ca_validity = require_validity(normalize_validity(cert_cfg.get("ca_validity")), "ca_validity")
    server_validity = require_validity(
        normalize_validity(cert_cfg.get("server_validity")), "server_validity"
    )
    client_validity = require_validity(
        normalize_validity(cert_cfg.get("client_validity")), "client_validity"
    )
    root_ca_subject = "ESB Local CA"

    if args.force or not (os.path.exists(root_ca_cert) and os.path.exists(root_ca_key)):
        generate_root_ca(
            root_ca_cert,
            root_ca_key,
            step_path,
            root_ca_subject,
            ca_validity,
            overwrite=args.force,
        )
        if args.skip_root_ca_install:
            print("Skipping local Root CA install (--skip-root-ca-install).")
        else:
            install_root_ca(step_path, root_ca_cert, output_dir)
    else:
        print(f"Root CA exists at {root_ca_cert}. Skipping generation. Use --force to regenerate.")

    # Check and generate Server/Client Certs
    cert_file = os.path.join(output_dir, cert_cfg.get("filename_cert", "server.crt"))
    key_file = os.path.join(output_dir, cert_cfg.get("filename_key", "server.key"))
    client_cert_file = os.path.join(output_dir, cert_cfg.get("filename_client_cert", "client.crt"))
    client_key_file = os.path.join(output_dir, cert_cfg.get("filename_client_key", "client.key"))

    server_domains, server_ips = collect_hosts(host_cfg)
    client_host_cfg = resolve_host_cfg(config, "client_hosts", host_cfg)
    client_domains, client_ips = collect_hosts(client_host_cfg)
    server_subject = resolve_subject(server_domains, server_ips, "localhost")
    client_subject = resolve_subject(client_domains, client_ips, "client")
    server_sans = dedupe_sans(server_domains, server_ips, server_subject)
    client_sans = dedupe_sans(client_domains, client_ips, client_subject)

    if args.force or not (os.path.exists(cert_file) and os.path.exists(key_file)):
        generate_leaf_cert(
            cert_file,
            key_file,
            server_sans,
            step_path,
            root_ca_cert,
            root_ca_key,
            "server",
            server_subject,
            server_validity,
            overwrite=args.force,
        )
    else:
        print(
            f"Server certificates exist at {output_dir}. "
            "Skipping generation. Use --force to regenerate."
        )

    if args.force or not (os.path.exists(client_cert_file) and os.path.exists(client_key_file)):
        generate_leaf_cert(
            client_cert_file,
            client_key_file,
            client_sans,
            step_path,
            root_ca_cert,
            root_ca_key,
            "client",
            client_subject,
            client_validity,
            overwrite=args.force,
        )
    else:
        print(
            f"Client certificates exist at {output_dir}. "
            "Skipping generation. Use --force to regenerate."
        )

    # Ensure repo-scoped brand dir is writable after sudo operations.
    ensure_user_ownership(str(brand_dir))
