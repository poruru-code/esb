#!/usr/bin/env python3
"""
Sample DinD Lambda E2E Test Runner

クロスプラットフォーム対応のテストランナー。
Windows/Linux/macOS で動作します。

Usage:
    python tests/run_tests.py [--build] [--cleanup] [--dind]
"""

import argparse
import os
import socket
import subprocess
import sys
import time
from datetime import datetime, timedelta, timezone
from pathlib import Path


# プロジェクトルートを取得
PROJECT_ROOT = Path(__file__).parent.parent.resolve()
CERTS_DIR = PROJECT_ROOT / "certs"


# .env.test があれば読み込む（簡易ローダー）
env_test_path = PROJECT_ROOT / ".env.test"
if env_test_path.exists():
    print(f"Loading environment variables from {env_test_path}")
    with open(env_test_path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            if "=" in line:
                key, value = line.split("=", 1)
                os.environ.setdefault(key.strip(), value.strip())

# フォールバック（.env.testがない場合用）
os.environ.setdefault("JWT_SECRET_KEY", "dev-secret-key-change-in-production")
os.environ.setdefault("X_API_KEY", "dev-api-key-change-in-production")
os.environ.setdefault("AUTH_USER", "admin")
os.environ.setdefault("AUTH_PASS", "password")
os.environ.setdefault("LOG_LEVEL", "DEBUG")
os.environ.setdefault("CONTAINERS_NETWORK", "test-network")
os.environ.setdefault("LAMBDA_NETWORK", "test-internal-network")
os.environ.setdefault("EXTERNAL_NETWORK", "test-external")
os.environ.setdefault("MANAGER_URL", "http://localhost:8081")  # テストからはlocalhostでアクセス
os.environ.setdefault("GATEWAY_INTERNAL_URL", "https://localhost:443")

# 設定
GATEWAY_PORT = os.environ.get("GATEWAY_PORT", "443")
GATEWAY_URL = f"https://localhost:{GATEWAY_PORT}"

SCYLLADB_PORT = os.environ.get("SCYLLADB_PORT", "8001")
SCYLLADB_API_URL = f"http://localhost:{SCYLLADB_PORT}"

VICTORIALOGS_PORT = os.environ.get("VICTORIALOGS_PORT", "9428")
MAX_RETRIES = 60
RETRY_INTERVAL = 3  # seconds


def run_command(cmd: list[str], check: bool = True) -> subprocess.CompletedProcess:
    """コマンドを実行し、結果を返す"""
    print(f"  > {' '.join(cmd)}")
    try:
        return subprocess.run(cmd, cwd=PROJECT_ROOT, check=check)
    except FileNotFoundError:
        print(f"Error: Command not found: {cmd[0]}")
        sys.exit(1)


def get_compose_command() -> list[str]:
    """使用可能な docker compose コマンドを判定"""
    # 1. 'docker compose' を試行
    try:
        result = subprocess.run(
            ["docker", "compose", "version"],
            capture_output=True,
            check=False,
        )
        if result.returncode == 0:
            return ["docker", "compose"]
    except FileNotFoundError:
        pass

    # 2. 'docker-compose' を試行
    try:
        result = subprocess.run(
            ["docker-compose", "version"],
            capture_output=True,
            check=False,
        )
        if result.returncode == 0:
            return ["docker-compose"]
    except FileNotFoundError:
        pass

    print("Error: Neither 'docker compose' nor 'docker-compose' was found.")
    print("Please install Docker Compose and try again.")
    sys.exit(1)


def get_local_ip() -> str:
    """ローカルIPアドレスを取得"""
    try:
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.connect(("8.8.8.8", 80))
        ip = s.getsockname()[0]
        s.close()
        return ip
    except Exception:
        return "127.0.0.1"


def generate_ssl_certificate():
    """自己署名SSL証明書を生成 (SAN対応)"""
    import ipaddress

    cert_file = CERTS_DIR / "server.crt"
    key_file = CERTS_DIR / "server.key"

    if cert_file.exists() and key_file.exists():
        print("Using existing SSL certificates")
        return

    print("Generating self-signed SSL certificate with SAN...")

    from cryptography import x509
    from cryptography.x509.oid import NameOID, ExtendedKeyUsageOID
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import rsa

    # RSA秘密鍵を生成
    private_key = rsa.generate_private_key(
        public_exponent=65537,
        key_size=4096,
    )

    # SAN (Subject Alternative Name) を構築
    hostname = socket.gethostname()
    local_ip = get_local_ip()

    san_list = [
        x509.DNSName("localhost"),
        x509.DNSName(hostname),
        x509.IPAddress(ipaddress.IPv4Address("127.0.0.1")),
    ]

    # ローカルIPが127.0.0.1でなければ追加
    if local_ip != "127.0.0.1":
        san_list.append(x509.IPAddress(ipaddress.IPv4Address(local_ip)))

    print(f"  SAN: localhost, {hostname}, 127.0.0.1, {local_ip}")

    # 証明書を構築
    subject = issuer = x509.Name(
        [
            x509.NameAttribute(NameOID.COUNTRY_NAME, "JP"),
            x509.NameAttribute(NameOID.STATE_OR_PROVINCE_NAME, "Tokyo"),
            x509.NameAttribute(NameOID.LOCALITY_NAME, "Minato"),
            x509.NameAttribute(NameOID.ORGANIZATION_NAME, "Development"),
            x509.NameAttribute(NameOID.COMMON_NAME, "localhost"),
        ]
    )

    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(private_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.now(timezone.utc))
        .not_valid_after(datetime.now(timezone.utc) + timedelta(days=365))
        .add_extension(
            x509.SubjectAlternativeName(san_list),
            critical=False,
        )
        .add_extension(
            x509.BasicConstraints(ca=False, path_length=None),
            critical=True,
        )
        .add_extension(
            x509.ExtendedKeyUsage([ExtendedKeyUsageOID.SERVER_AUTH]),
            critical=False,
        )
        .sign(private_key, hashes.SHA256())
    )

    # ディレクトリ作成
    CERTS_DIR.mkdir(parents=True, exist_ok=True)

    # 証明書を保存
    with open(cert_file, "wb") as f:
        f.write(cert.public_bytes(serialization.Encoding.PEM))

    # 秘密鍵を保存
    with open(key_file, "wb") as f:
        f.write(
            private_key.private_bytes(
                encoding=serialization.Encoding.PEM,
                format=serialization.PrivateFormat.TraditionalOpenSSL,
                encryption_algorithm=serialization.NoEncryption(),
            )
        )

    print(f"  Certificate saved to: {cert_file}")
    print(f"  Private key saved to: {key_file}")


def check_gateway_health() -> bool:
    """Gatewayのヘルスチェック"""
    try:
        import requests

        response = requests.get(f"{GATEWAY_URL}/health", timeout=5, verify=False)
        return response.status_code == 200
    except Exception:
        return False


def wait_for_gateway() -> bool:
    """Gatewayの起動を待機"""
    print("[3/4] Waiting for Gateway to be ready...")

    for i in range(1, MAX_RETRIES + 1):
        if check_gateway_health():
            print("Gateway is ready!")
            return True
        print(f"Waiting for Gateway... ({i}/{MAX_RETRIES})")
        time.sleep(RETRY_INTERVAL)

    print("Error: Gateway failed to start within timeout.")
    return False


def check_scylladb_health() -> bool:
    """ScyllaDB (Alternator) のヘルスチェック - Docker Health Status ベース"""
    try:
        import docker

        client = docker.from_env()
        container = client.containers.get("onpre-database")
        health = container.attrs.get("State", {}).get("Health", {})
        status = health.get("Status", "unknown")
        return status == "healthy"
    except Exception:
        return False


def wait_for_scylladb() -> bool:
    """ScyllaDBの起動を待機 (Docker Health Check)"""
    print("[2.5/4] Waiting for ScyllaDB (Docker Health) to be ready...")

    for i in range(1, MAX_RETRIES + 1):
        if check_scylladb_health():
            print("ScyllaDB is healthy!")
            return True
        print(f"Waiting for ScyllaDB... ({i}/{MAX_RETRIES})")
        time.sleep(RETRY_INTERVAL)

    print("Error: ScyllaDB failed to become healthy within timeout.")
    return False


def start_containers(build: bool = False, dind: bool = False):
    """Docker Composeでコンテナを起動"""
    print("[2/4] Starting containers...")

    compose_file = "docker-compose.dind.yml" if dind else "docker-compose.yml"
    cmd = get_compose_command() + ["-f", compose_file, "up", "-d"]

    if build:
        cmd.append("--build")

    run_command(cmd)


def stop_containers(dind: bool = False):
    """Docker Composeでコンテナを停止（冪等性確保）"""
    print("Cleaning up containers...")

    # オンデマンド Lambda コンテナを動的に検索して停止・削除
    # 末尾が 'onpre-internal-network' で終わるネットワークから lambda-* コンテナを検索
    try:
        import docker

        client = docker.from_env()

        # 動的にネットワークを検索
        for network in client.networks.list():
            if network.name.endswith("onpre-internal-network"):
                print(f"  Found internal network: {network.name}")
                network.reload()
                containers = network.attrs.get("Containers", {})
                for container_id, info in containers.items():
                    name = info.get("Name", "")
                    if name.startswith("lambda-"):
                        print(f"  Removing Lambda container: {name}")
                        try:
                            client.containers.get(name).remove(force=True)
                        except Exception:
                            pass
                break
    except ImportError:
        # docker パッケージがない場合はフォールバック
        pass

    # Docker Compose で管理されているコンテナを停止
    compose_file = "docker-compose.dind.yml" if dind else "docker-compose.yml"
    run_command(
        get_compose_command() + ["-f", compose_file, "down", "--remove-orphans", "-v"],
        check=False,
    )


def run_tests() -> int:
    """pytestでE2Eテストを実行"""
    print("[4/4] Running E2E tests...")

    # test_e2e.py に現在の環境変数を渡す
    env = os.environ.copy()
    env["GATEWAY_PORT"] = str(GATEWAY_PORT)
    env["VICTORIALOGS_PORT"] = str(VICTORIALOGS_PORT)
    # GATEWAY_URLなどはtest_e2e.py内で再構築されるが、URL自体を渡しても良い。
    # ここではポートを渡すことで整合性を取る。

    result = subprocess.run(
        [sys.executable, "-m", "pytest", "tests/test_e2e.py", "-v"],
        cwd=PROJECT_ROOT,
        check=False,
        env=env,
    )
    return result.returncode


def main():
    # 警告を抑制
    import warnings
    import urllib3

    urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
    warnings.filterwarnings("ignore", category=DeprecationWarning)

    parser = argparse.ArgumentParser(description="Sample DinD Lambda E2E Test Runner")
    parser.add_argument("--build", action="store_true", help="Rebuild images before running tests")
    parser.add_argument("--cleanup", action="store_true", help="Stop containers after tests")
    parser.add_argument(
        "--dind", action="store_true", help="Use DinD mode (docker-compose.dind.yml)"
    )

    args = parser.parse_args()

    print("=== Sample DinD Lambda E2E Test Runner ===")
    print(f"Project Root: {PROJECT_ROOT}")
    print(f"Options: build={args.build}, cleanup={args.cleanup}, dind={args.dind}")
    print()

    try:
        # SSL証明書生成
        print("[1/4] Checking SSL certificates...")
        import ipaddress  # noqa: F401 - used in generate_ssl_certificate

        generate_ssl_certificate()

        # コンテナ起動
        start_containers(build=args.build, dind=args.dind)

        # ヘルスチェック待機
        if not wait_for_scylladb():
            # ログを表示
            compose_file = "docker-compose.dind.yml" if args.dind else "docker-compose.yml"
            run_command(
                get_compose_command() + ["-f", compose_file, "logs", "database"], check=False
            )
            return 1

        if not wait_for_gateway():
            # ログを表示
            compose_file = "docker-compose.dind.yml" if args.dind else "docker-compose.yml"
            run_command(get_compose_command() + ["-f", compose_file, "logs"], check=False)
            return 1

        # テスト実行
        exit_code = run_tests()

        # 結果表示
        print()
        if exit_code == 0:
            print("🎉 Tests passed successfully!")
        else:
            print("❌ Tests failed.")

        return exit_code

    finally:
        # クリーンアップ
        if args.cleanup:
            stop_containers(dind=args.dind)


if __name__ == "__main__":
    sys.exit(main())
