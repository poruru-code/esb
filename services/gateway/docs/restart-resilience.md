<!--
Where: services/gateway/docs/restart-resilience.md
What: Container cleanup strategy on Gateway/Agent restart.
Why: Avoid state drift after restarts and document tradeoffs.
-->
# Gateway / Agent 再起動時の整合性維持

## 概要

現行構成は Agent (gRPC) を正本とし、Gateway はインメモリ状態を持つため、再起動時の整合性回復が必要です。  
現在は **Gateway 起動時のクリーンアップ** と **Janitor のリコンシリエーション** により、状態不整合を防ぎます。

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

Agent は runtime（docker / containerd）に依存せず `ListContainers` で管理対象コンテナを列挙できます。Gateway が稼働中の場合は Janitor がリコンシリエーションを継続するため、必要に応じて削除されます。Gateway も再起動した場合は前述のクリーンアップで全削除されます。

## 確認方法

```bash
# Agent 再起動
docker compose restart agent

# Gateway 再起動
docker compose restart gateway

# コンテナ状態の確認 (docker mode)
docker ps --format '{{.Names}}'

# コンテナ状態の確認 (containerd mode, namespace: <brand> / default: esb)
ctr -n <brand> containers list
```

## 関連実装

- Gateway 起動時クリーンアップ: `services/gateway/services/pool_manager.py` の `cleanup_all_containers`
- リコンシリエーション: `services/gateway/services/pool_manager.py` の `reconcile_orphans`
- Janitor ループ: `services/gateway/services/janitor.py`

---

## Implementation references
- `services/gateway/services/pool_manager.py`
- `services/gateway/services/janitor.py`
