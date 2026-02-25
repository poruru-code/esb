<!--
Where: docs/container-runtime-operations.md
What: Runtime-side container lifecycle and troubleshooting guide.
Why: Keep platform operations guidance separate from producer tooling implementation docs.
-->
# コンテナ運用とランタイム管理

本ドキュメントは Gateway / Agent / runtime-node の運用手順を扱います。  

## スコープ
- 対象: スタック起動、ログ確認、ランタイム状態確認、クリーンアップ
- 非対象: artifact 生成ツールの内部設計

## ライフサイクル責務

- **Gateway**: ルーティング、障害遮断、呼び出し制御
- **Agent**: コンテナ作成/破棄、再利用、runtime 連携
- **runtime-node**: containerd + CNI + DNS 実行基盤

詳細:
- Gateway: `services/gateway/docs/architecture.md`
- Autoscaling/Janitor: `services/gateway/docs/autoscaling.md`
- Agent: `services/agent/docs/architecture.md`
- runtime-node: `services/runtime-node/docs/README.md`

## 日常運用コマンド（最小セット）

`<project>` は compose project 名（通常は `esb-<env>`）、`<brand>` は runtime namespace 名です。

確認方法:
- `docker ps --format '{{.Names}}' | grep -E '(gateway|agent)$'`
- `echo "${ENV_PREFIX:-ESB}"`

### スタック起動

```bash
# Control-plane 起動（Docker mode）
docker compose -f docker-compose.docker.yml up -d

# Control-plane 起動（containerd mode）
docker compose -f docker-compose.containerd.yml up -d
```

### ログ・状態確認

```bash
# Gateway / Agent ログ
docker logs <project>-gateway
docker logs <project>-agent

# containerd 側の状態確認（containerd mode）
ctr -n <brand> containers list
```

### クリーンアップ

```bash
# 関連イメージのみ削除
docker images | grep -E "^(esb-|lambda-)" | awk '{print $3}' | xargs docker rmi

# dangling イメージ削除
docker image prune -f
```

## トラブルシューティング

### 1. コンテナが起動しない
1. `docker logs <project>-gateway`
2. `docker logs <project>-agent`
3. `ctr -n <brand> containers list`

### 2. 古いコードが実行される
1. 生成系ツールで関数イメージを再ビルド
2. artifact apply を再実行
3. 実行モードに対応する compose で再起動

### 3. Image 関数で `503` が出る
原因:
- 内部レジストリに対象イメージが投入されていない

対応:
1. `esb-ctl deploy --artifact <artifact.yml>` を再実行

### 4. `<untagged>` イメージが増える
- `docker image prune -f` で中間レイヤーを整理

---

## Implementation references
- `docker-compose.docker.yml`
- `docker-compose.containerd.yml`
- `services/gateway/services/pool_manager.py`
- `services/gateway/services/janitor.py`
- `services/agent/internal/runtime`
- `services/runtime-node/docs/README.md`
- `pkg/artifactcore/prepare_images.go`
