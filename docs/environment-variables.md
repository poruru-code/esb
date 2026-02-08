<!--
Where: docs/environment-variables.md
What: System-level overview of configuration propagation.
Why: Explain how env config flows without duplicating subsystem details.
-->
# 環境変数と構成の伝播（概要）

本基盤の設定は 3 段階で伝播します。
詳細な環境変数は各サブシステム docs を参照してください。

## 伝播レイヤ
1. Host / Compose
   - `.env` -> `docker-compose.*.yml` -> 各コンテナ環境変数
2. Service Configuration
   - Gateway / Agent / runtime-node / Provisioner が環境変数を読み込み内部設定へ
3. Worker Injection
   - Gateway が Lambda ワーカーへエンドポイント/設定を注入

## deploy 時の設定反映
`esb deploy` は staging config を生成後、実行中 stack の runtime target へ同期します。
このため通常運用では `CONFIG_DIR` の手動指定は不要です。
ただし `esb deploy --build-only` では同期と provisioner 実行を行わないため、
実行系コンテナへ反映するには通常 deploy（build-only なし）が必要です。

## 詳細（subsystem docs）
- Gateway: [services/gateway/docs/configuration.md](../services/gateway/docs/configuration.md)
- Agent: [services/agent/docs/configuration.md](../services/agent/docs/configuration.md)
- runtime-node: [services/runtime-node/docs/configuration.md](../services/runtime-node/docs/configuration.md)
- Provisioner: [services/provisioner/docs/configuration.md](../services/provisioner/docs/configuration.md)
- CLI: [cli/docs/architecture.md](../cli/docs/architecture.md)
- E2E runner env: [e2e/runner/README.md](../e2e/runner/README.md)

---

## Implementation references
- `docker-compose.containerd.yml`
- `docker-compose.docker.yml`
- `cli/internal/usecase/deploy/runtime_config.go`
- `services/gateway/config.py`
