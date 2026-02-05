<!--
Where: docs/README.md
What: System-level documentation index and navigation.
Why: Keep /docs focused on overall architecture and link to subsystem docs.
-->
# システム設計ドキュメント（System-level）

この `docs/` は **全体設計の概要**のみを扱います。詳細は各サブシステムの docs を参照してください。

## システム設計
- [システム仕様（概要）](./spec.md)
- [ランタイムモード概要（Docker / containerd）](./architecture-containerd.md)
- [構成の伝播（概念）](./environment-variables.md)

## Subsystem Docs
### Gateway
- [services/gateway/docs/README.md](../services/gateway/docs/README.md)
### Agent
- [services/agent/docs/README.md](../services/agent/docs/README.md)
### Provisioner
- [services/provisioner/docs/README.md](../services/provisioner/docs/README.md)
### runtime-node
- [services/runtime-node/docs/README.md](../services/runtime-node/docs/README.md)
### CLI
- [cli/docs/architecture.md](../cli/docs/architecture.md)
- [cli/docs/build.md](../cli/docs/build.md)
- [cli/docs/container-management.md](../cli/docs/container-management.md)

## ドキュメントのルール
- 図は Mermaid を使用する
- 各ページの末尾に **Implementation references** を置く
- 仕様の詳細は subsystem docs に寄せ、`docs/` は概要に留める

---

## Implementation references
- `docs/spec.md`
- `docs/architecture-containerd.md`
