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
- [E2E Runtime Smoke 設計](./e2e-runtime-smoke.md)

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

### E2E Runner
- [e2e/runner/README.md](../e2e/runner/README.md)

## CI Required Checks
- `quality-gates / python-static`
- `quality-gates / go-lint-agent`
- `quality-gates / go-lint-cli`

## 現在サイクルのスコープ
- 本サイクルは品質ゲート強化（Lint/Type/CI）と設定不整合修正のみを対象とする
- `deploy.go` / `main.go` / `server.go` の大規模分割は対象外（別Issueで追跡）

## ドキュメントのルール
- 図は Mermaid を使用する
- 各ページ末尾に **Implementation references** を置く
- 仕様の詳細は subsystem docs に寄せ、`docs/` は概要に留める

---

## Implementation references
- `docs/spec.md`
- `docs/architecture-containerd.md`
- `docs/e2e-runtime-smoke.md`
- `e2e/runner/README.md`
