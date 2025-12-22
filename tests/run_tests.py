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


from dotenv import load_dotenv

# プロジェクトルートを取得
PROJECT_ROOT = Path(__file__).parent.parent.resolve()
CERTS_DIR = PROJECT_ROOT / "certs"


def load_environment(env_file_path: Path):
    """環境変数ファイルを読み込む (python-dotenv使用)"""
    if env_file_path.exists():
        print(f"Loading environment variables from {env_file_path}")
        # override=False: 既存の環境変数（シェルから渡されたもの）を優先
        load_dotenv(env_file_path, override=False)
    else:
        print(f"Warning: Environment file {env_file_path} not found.")


# 設定
GATEWAY_PORT = os.environ.get("GATEWAY_PORT", "443")
GATEWAY_URL = f"https://localhost:{GATEWAY_PORT}"

SCYLLADB_PORT = os.environ.get("SCYLLADB_PORT", "8001")
SCYLLADB_API_URL = f"http://localhost:{SCYLLADB_PORT}"

VICTORIALOGS_PORT = os.environ.get("VICTORIALOGS_PORT", "9428")

# Constants
MAX_RETRIES = 60
RETRY_INTERVAL = 3  # seconds
HEALTH_CHECK_TIMEOUT = 5
SSL_CERT_VALIDITY_DAYS = 365
SSL_KEY_SIZE = 4096


def run_command(cmd: list[str], check: bool = True) -> subprocess.CompletedProcess:
    """コマンドを実行し、結果を返す"""
    print(f"  > {' '.join(cmd)}")
    try:
        return subprocess.run(cmd, cwd=PROJECT_ROOT, check=check)
    except FileNotFoundError:
        print(f"Error: Command not found: {cmd[0]}")
        sys.exit(1)


def get_compose_command() -> list[str]:
    """使用可能な docker compose コマンドを判定し、プロジェクト名を指定"""
    # 1. 'docker compose' を試行
    try:
        result = subprocess.run(
            ["docker", "compose", "version"],
            capture_output=True,
            check=False,
        )
        if result.returncode == 0:
            return ["docker", "compose", "--project-name", "edge-serverless-box"]
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
            return ["docker-compose", "--project-name", "edge-serverless-box"]
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
        key_size=SSL_KEY_SIZE,
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
        .not_valid_after(datetime.now(timezone.utc) + timedelta(days=SSL_CERT_VALIDITY_DAYS))
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

        response = requests.get(f"{GATEWAY_URL}/health", timeout=HEALTH_CHECK_TIMEOUT, verify=False)
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


def generate_lambda_files():
    """SAMテンプレートからDockerfile/configを生成"""
    print("[1.3/4] Generating Lambda files from SAM template...")

    cmd = [sys.executable, "-m", "tools.generator.main", "--config", "tests/e2e/generator.yml"]

    try:
        subprocess.run(cmd, cwd=PROJECT_ROOT, check=True)
    except subprocess.CalledProcessError as e:
        print(f"Error generating Lambda files: {e}")
        sys.exit(1)


def build_lambda_images():
    """すべてのLambda関数イメージをビルド

    tests/e2e/functions/ ディレクトリをスキャンし、
    Dockerfile が存在するサブディレクトリをビルド対象とする。
    イメージ名は 'lambda-{ディレクトリ名}' とする。
    """
    print("[1.5/4] Building Lambda function images...")

    functions_dir = PROJECT_ROOT / "tests" / "e2e" / "functions"
    if not functions_dir.exists():
        print("  Warning: tests/e2e/functions/ not found, skipping Lambda build")
        return

    lambda_functions = []
    for subdir in sorted(functions_dir.iterdir()):
        if subdir.is_dir():
            dockerfile = subdir / "Dockerfile"
            if dockerfile.exists():
                name = f"lambda-{subdir.name}"
                lambda_functions.append((name, str(dockerfile.relative_to(PROJECT_ROOT))))

    if not lambda_functions:
        print("  Warning: No Dockerfiles found in tests/e2e/functions/")
        return

    for name, dockerfile in lambda_functions:
        print(f"  Building {name}...")
        print(f"  > docker build -t {name}:latest -f {dockerfile} .")
        run_command(["docker", "build", "-t", f"{name}:latest", "-f", dockerfile, "."])

    print("  Lambda images built successfully!")


def start_containers(build: bool = False, dind: bool = False):
    """Docker Composeでコンテナを起動"""
    # Lambda イメージを先にビルド（--build 指定時）
    if build:
        build_lambda_images()

    print("[2/4] Starting containers...")

    if dind:
        compose_files = ["docker-compose.dind.yml"]
    else:
        # 非DinDモード: docker-compose.test.yml で tests/e2e/config をマウント
        compose_files = ["docker-compose.yml", "tests/docker-compose.test.yml"]

    cmd = get_compose_command()
    for f in compose_files:
        cmd.extend(["-f", f])
    cmd.extend(["up", "-d"])

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
    if dind:
        compose_files = ["docker-compose.dind.yml"]
    else:
        compose_files = ["docker-compose.yml", "tests/docker-compose.test.yml"]

    cmd = get_compose_command()
    for f in compose_files:
        cmd.extend(["-f", f])
    cmd.extend(["down", "--remove-orphans", "-v"])
    run_command(cmd, check=False)


def reset_containers(dind: bool = False):
    """完全にクリーンアップ（イメージも削除）"""
    print("Resetting environment (removing containers, volumes, and images)...")

    # Lambdaコンテナなどはstop_containersで消えるが、念のためstop_containersも呼ぶか、
    # あるいはdown --rmi allですべて消えるのを期待するか。
    # ここでは安全のため stop_containers のロジック（Lambda削除）は流用せず、
    # Composeの強力な cleanup に任せるが、LambdaコンテナがCompose管理外の場合は残る可能性がある。
    # しかし --remove-orphans があるのでネットワーク上のものは消えるはず。
    # 念のため既存の stop_containers を呼んでから reset するのが安全だが、
    # ユーザーの要望は `down --volumes --rmi all --remove-orphans` なのでそれを素直に実装する。

    if dind:
        compose_files = ["docker-compose.dind.yml"]
    else:
        compose_files = ["docker-compose.yml", "tests/docker-compose.test.yml"]

    cmd = get_compose_command()
    for f in compose_files:
        cmd.extend(["-f", f])
    cmd.extend(["down", "--volumes", "--rmi", "all", "--remove-orphans"])
    run_command(cmd, check=False)


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
        "--reset",
        action="store_true",
        help="Remove all containers, volumes, and images before running",
    )
    parser.add_argument(
        "--dind", action="store_true", help="Use DinD mode (docker-compose.dind.yml)"
    )
    parser.add_argument(
        "--env-file",
        type=Path,
        default=PROJECT_ROOT / "tests" / ".env.test",
        help="Path to .env file (default: tests/.env.test)",
    )

    args = parser.parse_args()

    # 環境変数をロード
    load_environment(args.env_file)

    # グローバル変数を更新
    global GATEWAY_PORT, GATEWAY_URL, SCYLLADB_PORT, SCYLLADB_API_URL, VICTORIALOGS_PORT
    GATEWAY_PORT = os.environ.get("GATEWAY_PORT", "443")
    GATEWAY_URL = f"https://localhost:{GATEWAY_PORT}"
    SCYLLADB_PORT = os.environ.get("SCYLLADB_PORT", "8001")
    SCYLLADB_API_URL = f"http://localhost:{SCYLLADB_PORT}"
    VICTORIALOGS_PORT = os.environ.get("VICTORIALOGS_PORT", "9428")

    print("=== Sample DinD Lambda E2E Test Runner ===")
    print(f"Project Root: {PROJECT_ROOT}")
    print(
        f"Options: build={args.build}, cleanup={args.cleanup}, reset={args.reset}, dind={args.dind}"
    )
    print()

    try:
        # リセット要求があれば実行
        if args.reset:
            reset_containers(dind=args.dind)
            # イメージを削除したため、再ビルドを強制
            args.build = True

        # --reset 時、または生成ファイルが不足している場合は再生成
        functions_dir = PROJECT_ROOT / "tests/e2e/functions"
        generated_files_exist = any(functions_dir.glob("*/Dockerfile"))

        if args.reset or not generated_files_exist:
            if not generated_files_exist and not args.reset:
                print("[!] Generated files missing, entering auto-generation mode...")
            generate_lambda_files()

        # SSL証明書生成
        print("[1/4] Checking SSL certificates...")
        import ipaddress  # noqa: F401 - used in generate_ssl_certificate

        generate_ssl_certificate()

        # コンテナ起動
        start_containers(build=args.build, dind=args.dind)

        # ヘルスチェック待機
        if not wait_for_scylladb():
            # ログを表示
            if args.dind:
                compose_files = ["docker-compose.dind.yml"]
            else:
                compose_files = ["docker-compose.yml", "docker-compose.test.yml"]
            cmd = get_compose_command()
            for f in compose_files:
                cmd.extend(["-f", f])
            cmd.extend(["logs", "database"])
            run_command(cmd, check=False)
            return 1

        if not wait_for_gateway():
            # ログを表示
            if args.dind:
                compose_files = ["docker-compose.dind.yml"]
            else:
                compose_files = ["docker-compose.yml", "docker-compose.test.yml"]
            cmd = get_compose_command()
            for f in compose_files:
                cmd.extend(["-f", f])
            cmd.extend(["logs"])
            run_command(cmd, check=False)
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
