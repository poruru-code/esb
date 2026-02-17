<!--
Where: services/agent/docs/README.md
What: Entry point for Agent subsystem documentation.
Why: Keep agent-specific design details close to the implementation.
-->
# Agent ドキュメント

Agent は Gateway からの gRPC リクエストを受け、Lambda ワーカーコンテナ（Docker / containerd）の
ライフサイクル（作成/削除/一覧/メトリクス/Invoke 代理）を管理します。

Image 関数は deploy 時 prewarm を前提とし、実行時 Agent は内部レジストリ参照を扱います。

## このディレクトリのスコープ
- 対象: `services/agent`（Agent 本体）
- 対象外: Gateway 側の HTTP API 詳細（`services/gateway/docs` を参照）

## まず読む順序
1. [アーキテクチャ](./architecture.md)
2. [gRPC API](./grpc-api.md)
3. [Runtime: containerd](./runtime-containerd.md)
4. [Runtime: docker](./runtime-docker.md)
5. [設定（環境変数）](./configuration.md)
6. [Proto Generation](./proto-generation.md)

## 目的別ガイド
| 目的 | 参照先 |
| --- | --- |
| API 契約を確認したい | [grpc-api.md](./grpc-api.md) |
| containerd 実装を追いたい | [runtime-containerd.md](./runtime-containerd.md) |
| docker モード差分を確認したい | [runtime-docker.md](./runtime-docker.md) |
| 環境変数を確認したい | [configuration.md](./configuration.md) |
| proto 生成手順を確認したい | [proto-generation.md](./proto-generation.md) |

## 関連
- System-level: [docs/spec.md](../../../docs/spec.md)
- Gateway: [services/gateway/docs/architecture.md](../../gateway/docs/architecture.md)
- runtime-node: [services/runtime-node/docs/startup.md](../../runtime-node/docs/startup.md)

---

## Implementation references
- `services/contracts/proto/agent.proto`
- `tools/gen_proto.py`
- `services/agent/cmd/agent/main.go`
- `services/agent/internal/api/server.go`
- `services/agent/internal/runtime/interface.go`
