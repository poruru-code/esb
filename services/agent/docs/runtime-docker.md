<!--
Where: services/agent/docs/runtime-docker.md
What: Docker runtime behavior and limitations for the Agent.
Why: Docker mode differs from containerd in networking, metrics, and pause semantics.
-->
# Runtime（docker）

## 前提
Agent は `AGENT_RUNTIME=docker` のとき、Docker Engine API を使ってワーカーを作成します。
containerd と違い CNI は使わず、Docker ネットワーク上で IP を解決します。

## 起動の流れ（要点）
- コンテナ名: `{brand}-{env}-{function}-{id}`
  - `brand` は `meta.Slug`（例: `esb`）
- イメージ解決:
  - API で受け取った `image` を使用
  - `image` は内部レジストリ参照（例: `registry:5010/...`）を前提
- Docker ネットワーク: `CONTAINERS_NETWORK` で指定
- `ContainerInspect` をリトライし IP を解決

## 外部レジストリとの関係
- Docker runtime は Source Registry への pull を実行しません。
- 外部イメージ取り込みは `esb deploy --image-prewarm=all` の責務です。
- runtime は内部レジストリからの pull のみを行います。

## 制限事項
- **Pause/Resume は未実装**（gRPC API は互換のために存在）
- **Metrics は未実装**
  - `GetContainerMetrics` は `Internal` になる可能性があります

## IP 解決
Docker runtime では `ContainerInspect` を使い、対象ネットワークの IP を取得します。
取得できない場合、他ネットワークの IP も探索します。

---

## Implementation references
- `services/agent/internal/runtime/docker/runtime.go`
- `services/agent/internal/runtime/interface.go`
- `services/agent/internal/runtime/constants.go`
