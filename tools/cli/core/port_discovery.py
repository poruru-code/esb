"""
Where: tools/cli/core/port_discovery.py
What: Docker起動後に割り当てられたポートを取得・永続化
Why: 動的ポート割り当て時にテストやprovisionerが正しいポートを参照できるようにする
"""
import json
import os
import subprocess
from pathlib import Path
from typing import Optional

# サービス名とコンテナポートのマッピング
# (環境変数名, サービス名, コンテナポート, モード制限)
# モード制限: None=全モード, ["docker"]=dockerのみ, ["containerd", "firecracker"]=指定モードのみ
PORT_MAPPINGS = [
    # Gateway port: dockerモードはgatewayサービス、containerd/fcはruntime-nodeサービス
    ("ESB_PORT_GATEWAY_HTTPS", "gateway", 443, ["docker"]),
    ("ESB_PORT_GATEWAY_HTTPS", "runtime-node", 443, ["containerd", "firecracker"]),
    # 共通サービス
    ("ESB_PORT_STORAGE", "s3-storage", 9000, None),
    ("ESB_PORT_STORAGE_MGMT", "s3-storage", 9001, None),
    ("ESB_PORT_DATABASE", "database", 8000, None),
    ("ESB_PORT_VICTORIALOGS", "victorialogs", 9428, None),
    # containerd/fc専用
    ("ESB_PORT_REGISTRY", "registry", 5010, ["containerd", "firecracker"]),
    ("ESB_PORT_AGENT_GRPC", "runtime-node", 50051, ["containerd", "firecracker"]),
]


def discover_ports(
    project_name: str,
    compose_files: list[str],
    mode: Optional[str] = None
) -> dict[str, int]:
    """
    docker compose portで割り当てられたポートを取得

    Args:
        project_name: Docker Composeプロジェクト名
        compose_files: 使用するcompose files
        mode: 現在のランタイムモード（containerd/docker/firecracker）

    Returns:
        {環境変数名: ホストポート} の辞書
    """
    result = {}

    # コマンドのベース部分
    base_cmd = ["docker", "compose", "-p", project_name]
    for f in compose_files:
        base_cmd.extend(["-f", f])

    for env_var, service, container_port, mode_filter in PORT_MAPPINGS:
        # モード制限がある場合はスキップ
        if mode_filter and mode and mode not in mode_filter:
            continue

        try:
            cmd = base_cmd + ["port", service, str(container_port)]
            output = subprocess.check_output(
                cmd, text=True, stderr=subprocess.DEVNULL
            ).strip()

            if output:
                # "0.0.0.0:54321" or "[::]:54321" → 54321
                host_port = int(output.split(":")[-1])
                result[env_var] = host_port
        except (subprocess.CalledProcessError, ValueError):
            # サービスが存在しない or ポート公開されていない場合はスキップ
            pass

    return result


def save_ports(env_name: str, ports: dict[str, int]) -> Path:
    """
    ポートマッピングをファイルに永続化

    Args:
        env_name: 環境名
        ports: ポートマッピング辞書

    Returns:
        保存先のPathオブジェクト
    """
    port_file = Path.home() / ".esb" / env_name / "ports.json"
    port_file.parent.mkdir(parents=True, exist_ok=True)
    port_file.write_text(json.dumps(ports, indent=2))
    return port_file


def load_ports(env_name: str) -> dict[str, int]:
    """
    永続化されたポートマッピングを読み込み

    Args:
        env_name: 環境名

    Returns:
        ポートマッピング辞書（ファイルが存在しない場合は空辞書）
    """
    port_file = Path.home() / ".esb" / env_name / "ports.json"
    if port_file.exists():
        return json.loads(port_file.read_text())
    return {}


def apply_ports_to_env(ports: dict[str, int]) -> None:
    """
    ポートマッピングを環境変数に適用

    Args:
        ports: ポートマッピング辞書
    """
    for env_var, port in ports.items():
        os.environ[env_var] = str(port)

    # 派生環境変数も設定
    if "ESB_PORT_GATEWAY_HTTPS" in ports:
        gateway_port = ports["ESB_PORT_GATEWAY_HTTPS"]
        os.environ["GATEWAY_PORT"] = str(gateway_port)
        os.environ["GATEWAY_URL"] = f"https://localhost:{gateway_port}"

    if "ESB_PORT_VICTORIALOGS" in ports:
        vl_port = ports["ESB_PORT_VICTORIALOGS"]
        os.environ["VICTORIALOGS_PORT"] = str(vl_port)
        os.environ["VICTORIALOGS_URL"] = f"http://localhost:{vl_port}"
        os.environ["VICTORIALOGS_QUERY_URL"] = os.environ["VICTORIALOGS_URL"]

    if "ESB_PORT_AGENT_GRPC" in ports:
        agent_port = ports["ESB_PORT_AGENT_GRPC"]
        os.environ["AGENT_GRPC_ADDRESS"] = f"localhost:{agent_port}"


def log_ports(env_name: str, ports: dict[str, int]) -> None:
    """
    ポートマッピングをコンソールに出力

    Args:
        env_name: 環境名
        ports: ポートマッピング辞書
    """
    print(f"\n{'='*60}")
    print(f"[{env_name}] Dynamic Port Assignment:")
    print(f"{'='*60}")
    for env_var, port in sorted(ports.items()):
        print(f"  {env_var}: {port}")
    print(f"{'='*60}\n")
