<!--
Where: services/runtime-node/docs/README.md
What: Entry point for runtime-node subsystem documentation.
Why: Keep runtime-node operational details near its implementation.
-->
# runtime-node ドキュメント

runtime-node は containerd + CNI + CoreDNS を束ねる **実行ノード**です。
containerd compose では Gateway / Agent / CoreDNS と NetNS を共有し、Lambda ワーカーのネットワークと実行環境を支えます。

## まず読む順序
1. [起動フロー](./startup.md)
2. [ネットワーク設計](./networking.md)
3. [設定（環境変数）](./configuration.md)
4. [Firecracker](./firecracker.md)
5. [devmapper](./devmapper.md)

## 目的別ガイド
| 目的 | 参照先 |
| --- | --- |
| 起動シーケンスを確認したい | [startup.md](./startup.md) |
| CNI/CoreDNS/NAT を確認したい | [networking.md](./networking.md) |
| 設定値を確認したい | [configuration.md](./configuration.md) |
| Firecracker モードを確認したい | [firecracker.md](./firecracker.md) |
| devmapper 事前準備を確認したい | [devmapper.md](./devmapper.md) |

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
