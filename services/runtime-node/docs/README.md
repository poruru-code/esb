<!--
Where: services/runtime-node/docs/README.md
What: Entry point for runtime-node subsystem documentation.
Why: Keep runtime-node operational details near its implementation.
-->
# runtime-node ドキュメント

runtime-node は containerd + CNI + CoreDNS を束ねる **実行ノード**です。
Gateway / Agent / CoreDNS と NetNS を共有し、Lambda ワーカーのネットワークと実行環境を支えます。

## 目次
- [起動フロー](./startup.md)
- [ネットワーク設計](./networking.md)
- [devmapper](./devmapper.md)
- [Firecracker](./firecracker.md)
- [設定（環境変数）](./configuration.md)
- [Firecracker Roadmap](./firecracker-roadmap.md)

## 関連
- Agent: [services/agent/docs/runtime-containerd.md](../../agent/docs/runtime-containerd.md)
- System-level: [docs/architecture-containerd.md](../../../docs/architecture-containerd.md)

---

## Implementation references
- `services/runtime-node/entrypoint.sh`
- `services/runtime-node/entrypoint.containerd.sh`
- `services/runtime-node/entrypoint.firecracker.sh`
- `services/runtime-node/entrypoint.common.sh`
- `docker-compose.containerd.yml`
