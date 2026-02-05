<!--
Where: docs/environment-variables.md
What: System-level overview of configuration propagation.
Why: Explain how env config flows without duplicating subsystem details.
-->
# 環境変数と構成の伝播（概要）

本基盤の設定は **3 段階**で伝播します。詳細な環境変数は各サブシステム docs を参照してください。

## 伝播レイヤ
1. **Host / Compose**
   - `.env` → `docker-compose.*.yml` → 各コンテナ環境変数
2. **Service Configuration**
   - Gateway / Agent / runtime-node / Provisioner が環境変数を読み込み内部設定へ
3. **Worker Injection**
   - Gateway が Lambda ワーカーへエンドポイント/設定を注入

## 詳細（各 subsystem）
- Gateway: [services/gateway/docs/configuration.md](../services/gateway/docs/configuration.md)
- Agent: [services/agent/docs/configuration.md](../services/agent/docs/configuration.md)
- runtime-node: [services/runtime-node/docs/configuration.md](../services/runtime-node/docs/configuration.md)
- Provisioner: [services/provisioner/docs/configuration.md](../services/provisioner/docs/configuration.md)
- CLI: [cli/docs/build.md](../cli/docs/build.md)

---

## Implementation references
- `docker-compose.containerd.yml`
- `docker-compose.docker.yml`
- `services/gateway/config.py`
