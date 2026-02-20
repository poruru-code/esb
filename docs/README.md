<!--
Where: docs/README.md
What: System-level documentation index and onboarding navigation.
Why: Help new contributors find the source-of-truth document without guessing.
-->
# システムドキュメント案内

`docs/` は **システム全体の設計・運用方針**を扱います。  
実装詳細は各サブシステム配下の docs を参照してください。

## 初回セットアップ導線
- 開発環境セットアップと基本コマンドはリポジトリ直下の `README.md` を参照
- E2E 実行の詳細は `e2e/runner/README.md` を参照

## 新規着任者向け: まず読む順序
1. [システム仕様（概要）](./spec.md)
2. [ランタイムモード概要（Docker / containerd）](./architecture-containerd.md)
3. [コンテナ運用とランタイム管理](./container-runtime-operations.md)
4. [CLI アーキテクチャ](../cli/docs/architecture.md)
5. [E2E Runtime Smoke 設計](./e2e-runtime-smoke.md)

## 目的別ナビゲーション
| 目的 | 最初に読むドキュメント |
| --- | --- |
| 全体構成を把握したい | [spec.md](./spec.md) |
| Docker/containerd の差分を知りたい | [architecture-containerd.md](./architecture-containerd.md) |
| ランタイム障害の運用手順を確認したい | [container-runtime-operations.md](./container-runtime-operations.md) |
| artifact-first の運用手順を確認したい | [artifact-operations.md](./artifact-operations.md) |
| CLI 分離の実行手順を確認したい | [cli-separation-checklist.md](./cli-separation-checklist.md) |
| イメージ設計・ビルド方針を確認したい | [docker-image-architecture.md](./docker-image-architecture.md) |
| Trace/ログ連携の仕様を確認したい | [trace-propagation.md](./trace-propagation.md), [local-logging-adapter.md](./local-logging-adapter.md) |
| ディレクトリ責務分離を確認したい | [repo-layout-contract.md](./repo-layout-contract.md) |
| Branding 運用を確認したい | [branding-generator.md](./branding-generator.md) |

## システムドキュメント一覧
- [spec.md](./spec.md)
- [architecture-containerd.md](./architecture-containerd.md)
- [container-runtime-operations.md](./container-runtime-operations.md)
- [artifact-operations.md](./artifact-operations.md)
- [cli-separation-checklist.md](./cli-separation-checklist.md)
- [docker-image-architecture.md](./docker-image-architecture.md)
- [environment-variables.md](./environment-variables.md)
- [e2e-runtime-smoke.md](./e2e-runtime-smoke.md)
- [local-logging-adapter.md](./local-logging-adapter.md)
- [trace-propagation.md](./trace-propagation.md)
- [repo-layout-contract.md](./repo-layout-contract.md)
- [branding-generator.md](./branding-generator.md)

## サブシステム実装ドキュメント
- Gateway: [services/gateway/docs/README.md](../services/gateway/docs/README.md)
- Agent: [services/agent/docs/README.md](../services/agent/docs/README.md)
- Provisioner: [services/provisioner/docs/README.md](../services/provisioner/docs/README.md)
- runtime-node: [services/runtime-node/docs/README.md](../services/runtime-node/docs/README.md)
- CLI: [cli/docs/architecture.md](../cli/docs/architecture.md), [cli/docs/build.md](../cli/docs/build.md), [cli/docs/container-management.md](../cli/docs/container-management.md)
- E2E Runner: [e2e/runner/README.md](../e2e/runner/README.md)

## 品質ゲート（最低限）
- `quality-gates / python-static`
- `quality-gates / go-lint-agent`
- `quality-gates / go-lint-cli`

## ドキュメント運用ルール
- システム全体の方針は `docs/`、実装詳細は subsystem docs に置く
- 図は Mermaid を使用する
- 各ページ末尾に **Implementation references** を置く

---

## Implementation references
- `docs/spec.md`
- `docs/architecture-containerd.md`
- `docs/container-runtime-operations.md`
- `docs/artifact-operations.md`
- `cli/docs/architecture.md`
