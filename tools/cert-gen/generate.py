#!/usr/bin/env python3
import argparse
import os
import shutil
import socket
import subprocess
import sys

import toml


def get_local_ip():
    try:
        # 8.8.8.8にダミー接続して自身のルートIPを取得 (パケット送信はしない)
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.connect(("8.8.8.8", 80))
            return s.getsockname()[0]
    except Exception:
        return "127.0.0.1"


def check_mkcert():
    if not shutil.which("mkcert"):
        print("Error: mkcert not found.")
        print("Please install mkcert via mise or system package manager.")
        exit(1)


def install_root_ca():
    print("Installing local Root CA...")
    subprocess.check_call(["mkcert", "-install"])


def generate_cert(config_path):
    config = toml.load(config_path)
    cert_cfg = config.get("certificate", {})
    host_cfg = config.get("hosts", {})

    output_dir = os.path.expanduser(cert_cfg.get("output_dir", "~/.esb/certs"))
    os.makedirs(output_dir, exist_ok=True)

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
    cmd = ["mkcert", "-cert-file", cert_file, "-key-file", key_file]
    cmd.extend(domains)
    cmd.extend(ips)

    print(f"Generating certificates in {output_dir}...")
    print(f"Domains: {domains}")
    print(f"IPs: {ips}")

    subprocess.check_call(cmd)
    print("Certificate generation complete.")


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

    # Load config
    config = toml.load(args.config)
    cert_cfg = config.get("certificate", {})
    output_dir = os.path.expanduser(cert_cfg.get("output_dir", "~/.esb/certs"))

    # Ensure CAROOT is set to output_dir if not present
    if "CAROOT" not in os.environ:
        os.environ["CAROOT"] = output_dir

    check_mkcert()

    # Check and install Root CA
    root_ca_path = os.path.join(os.environ["CAROOT"], "rootCA.pem")

    # Safety check: Docker sometimes creates a directory if the mount target is missing
    if os.path.isdir(root_ca_path):
        print(f"ERROR: {root_ca_path} is a directory, but it should be a file.")
        print("This often happens when Docker mistakenly creates a directory for a file mount.")
        print(f"Please run: sudo rm -rf {root_ca_path}")
        sys.exit(1)

    if args.force or not os.path.exists(root_ca_path):
        install_root_ca()
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
        generate_cert(args.config)
    else:
        print(
            f"Certificates exist at {output_dir}. Skipping generation. Use --force to regenerate."
        )
