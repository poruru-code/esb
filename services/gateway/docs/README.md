<!--
Where: services/gateway/docs/README.md
What: Entry point for Gateway subsystem documentation.
Why: Keep Gateway-specific behavior and APIs close to the service code.
-->
# Gateway ドキュメント

Gateway は HTTP エントリポイントとして、認証・ルーティング・Lambda invoke を担う FastAPI サービスです。
Agent と gRPC で連携し、ワーカーの起動/削除/監視を行います。

## 目次
- [アーキテクチャ](./architecture.md)
- [設定（環境変数）](./configuration.md)
- [セキュリティ / 認証](./security.md)
- [レジリエンス](./resilience.md)
- [オートスケーリング](./autoscaling.md)
- [再起動時の整理](./orchestrator-restart-resilience.md)
- [コンテナキャッシュ](./container-cache.md)
- [ネットワーク最適化](./network-optimization.md)

## 関連
- Agent: [services/agent/docs/architecture.md](../../agent/docs/architecture.md)
- runtime-node: [services/runtime-node/docs/networking.md](../../runtime-node/docs/networking.md)
- System-level: [docs/spec.md](../../../docs/spec.md)

---

## Implementation references
- `services/gateway/main.py`
- `services/gateway/services/*`
- `services/gateway/config.py`
