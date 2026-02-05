<!--
Where: services/gateway/docs/orchestrator-restart-resilience.md
What: Container cleanup strategy on Gateway/Agent restart.
Why: Avoid state drift after restarts and document tradeoffs.
-->
# Gateway / Agent 再起動時のコンテナ整理

## 概要

本基盤は Agent (gRPC) + containerd の構成に移行しており、従来の Python Orchestrator による **Adopt & Sync** は使用しません。現在は **Gateway 起動時のクリーンアップ** と **Janitor のリコンシリエーション** により、再起動後の状態不整合を防ぎます。

## 現在の方針

### 1. Gateway 起動時のクリーンアップ
Gateway は起動時に Agent へ `ListContainers` を実行し、見つかったコンテナを `DestroyContainer` で削除します。これにより、Gateway のインメモリ状態が失われた状態でも確実に再構築できます。

- **利点**: 不整合が起きにくく、運用が安定
- **トレードオフ**: 再起動直後はコールドスタートが発生

### 2. Janitor によるリコンシリエーション
Gateway の Janitor は周期的に Agent の一覧を取得し、Gateway が管理していないコンテナ（孤児）を削除します。

- **保護**: `ORPHAN_GRACE_PERIOD_SECONDS` 以内に作成されたコンテナは削除しません
- **目的**: 起動直後やプロビジョニング中のコンテナを誤削除しないため

## Agent 再起動時の挙動

Agent は containerd 上のコンテナを `ListContainers` で列挙できます。Gateway が稼働中の場合は Janitor がリコンシリエーションを継続するため、必要に応じて削除されます。Gateway も再起動した場合は前述のクリーンアップで全削除されます。

## 確認方法

```bash
# Agent 再起動
docker compose restart agent

# Gateway 再起動
docker compose restart gateway

# コンテナ状態の確認 (containerd)
ctr -n esb-runtime containers list
```

## 関連実装

- Gateway 起動時クリーンアップ: `services/gateway/services/pool_manager.py` の `cleanup_all_containers`
- リコンシリエーション: `services/gateway/services/pool_manager.py` の `reconcile_orphans`
- Janitor ループ: `services/gateway/services/janitor.py`

---

## Implementation references
- `services/gateway/services/pool_manager.py`
- `services/gateway/services/janitor.py`
