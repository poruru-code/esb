<!--
Where: services/gateway/docs/README.md
What: Entry point for Gateway subsystem documentation.
Why: Keep Gateway-specific behavior and APIs close to service code.
-->
# Gateway ドキュメント

Gateway は HTTP エントリポイントとして、認証・ルーティング・Lambda invoke を担う FastAPI サービスです。
Agent と gRPC で連携し、ワーカーの起動/削除/監視を行います。

Gateway 実装は主に以下へ分割されています。
- `main.py`: app assembly
- `lifecycle.py`: startup/shutdown orchestration
- `middleware.py`: cross-cutting HTTP concerns
- `routes.py`: endpoint handlers

## まず読む順序
1. [アーキテクチャ](./architecture.md)
2. [設定（環境変数）](./configuration.md)
3. [セキュリティ / 認証](./security.md)
4. [オートスケーリング](./autoscaling.md)
5. [レジリエンス](./resilience.md)
6. [再起動時の整理](./restart-resilience.md)
7. [ネットワーク最適化](./network-optimization.md)

## 目的別ガイド
| 目的 | 参照先 |
| --- | --- |
| ルーティング/Invoke フローを追いたい | [architecture.md](./architecture.md) |
| 環境変数を確認したい | [configuration.md](./configuration.md) |
| 認証仕様を確認したい | [security.md](./security.md) |
| プール挙動を確認したい | [autoscaling.md](./autoscaling.md) |
| 障害時挙動を確認したい | [resilience.md](./resilience.md) |
| 再起動時の整合性を確認したい | [restart-resilience.md](./restart-resilience.md) |

## 関連
- Agent: [services/agent/docs/architecture.md](../../agent/docs/architecture.md)
- runtime-node: [services/runtime-node/docs/networking.md](../../runtime-node/docs/networking.md)
- System-level: [docs/spec.md](../../../docs/spec.md)

---

## Implementation references
- `services/gateway/main.py`
- `services/gateway/lifecycle.py`
- `services/gateway/middleware.py`
- `services/gateway/routes.py`
- `services/gateway/config.py`
