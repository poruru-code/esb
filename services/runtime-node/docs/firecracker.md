<!--
Where: services/runtime-node/docs/firecracker.md
What: Firecracker mode behavior in runtime-node.
Why: Summarize required runtime flags and key artifacts.
-->
# Firecracker モード

## 概要
runtime-node は `CONTAINERD_RUNTIME=aws.firecracker` を指定すると、
Firecracker shim / firecracker-containerd 構成で起動します。

## 有効化
```bash
CONTAINERD_RUNTIME=aws.firecracker docker compose -f docker-compose.containerd.yml up -d
```

## 重要ポイント
- `entrypoint.firecracker.sh` が起動されます
- `firecracker-containerd` 設定: `/etc/firecracker-containerd/config.toml`
- `firecracker` / `jailer` バイナリが同梱されます（Dockerfile で取得）
- `VHOST_VSOCK_REQUIRED=1` の場合、`/dev/vhost-vsock` が必須になります
- `DEVMAPPER_POOL` は事前作成済み thin-pool を指定する必要があります

## 関連ファイル
- `services/runtime-node/firecracker-containerd.toml`
- `services/runtime-node/firecracker-runtime.json`
- `services/runtime-node/firecracker-runc-config.json`

## Implementation references
- `services/runtime-node/Dockerfile.containerd`
- `services/runtime-node/entrypoint.firecracker.sh`
- `services/runtime-node/firecracker-containerd.toml`
