<!--
Where: services/agent/docs/runtime-containerd.md
What: Containerd runtime behavior and networking details for the Agent.
Why: Containerd mode has unique CNI/DNS/snapshotter wiring that needs to be documented.
-->
# Runtime（containerd）

## 前提
Agent は `AGENT_RUNTIME=containerd` のとき、containerd を直接操作してワーカーを起動します。

containerd モードの compose では、Agent は以下を共有する前提で構成されています（重要）:
- `network_mode: service:runtime-node`（runtime-node の NetNS を共有）
- `pid: service:runtime-node`（runtime-node の PIDNS を共有）
- `/run/containerd/containerd.sock` を共有ボリュームでマウント

これにより Agent から `/proc/<pid>/ns/net` を参照し、CNI の add/del を実行できます。

## 起動時（CNI config の生成）
Agent 起動時に CNI の `.conflist` を生成します。

- 生成先: `CNI_CONF_DIR`（既定: `/etc/cni/net.d`）
- 生成ファイル名: `10-<meta.RuntimeCNIName>.conflist`
- サブネット: `CNI_SUBNET`（未指定なら既定 `10.88.0.0/16`）
- DNS nameserver: `CNI_DNS_SERVER` → `CNI_GW_IP` → 既定 `10.88.0.1`

生成される構成は `bridge` + `portmap` の 2 プラグイン構成です（`ipMasq: true`）。

## Ensure（ワーカー起動の要点）
### イメージ解決
`EnsureContainerRequest.image` が空の場合、Agent が関数名からイメージ名を解決します。

- レジストリ: `CONTAINER_REGISTRY`（既定: `registry:5010`）
- タグ: `<ENV_PREFIX>_TAG`（例: `ESB_TAG`、既定: `latest`）

### コンテナ名 / ラベル
- コンテナ名: `esb-{env}-{function}-{id}`（短い hex ID）
- ラベル: function/env/owner などを付与（Janitor/ownership に使用）

### ネットワーク（CNI）
Task を start した後、以下で CNI をセットアップします:
- netns: `/proc/<taskPid>/ns/net`
- `cni.Setup(ctx, containerID, netnsPath)`
- `cni.Result` から IPv4 を抽出し、`WorkerInfo.ip_address` として返却

### `/etc/resolv.conf` の注入
ワーカーの `/etc/resolv.conf` を Agent 側で生成したファイルに bind-mount します。
nameserver は `CNI_DNS_SERVER` / `CNI_GW_IP` を参照し、未設定時は既定 `10.88.0.1` です。

### Snapshotter（overlay/devmapper）
- `CONTAINERD_SNAPSHOTTER` が指定されていればそれを使用します。
- `CONTAINERD_RUNTIME=aws.firecracker` の場合、既定で `devmapper` を選びます（それ以外は `overlayfs`）。

## List（IP の再解決）
Agent の `ListContainers` では、CNI の IPAM state から IP を再解決します。

- `CNI_NET_DIR`（既定: `/var/lib/cni/networks`）
- 参照パス: `<CNI_NET_DIR>/<networkName>/<containerID>`

`networkName` は CNI 設定（conflist）の `name` から決定し、取得できない場合は `meta.RuntimeCNIName` を使います。

---

## Implementation references
- `services/agent/cmd/agent/main.go`
- `services/agent/internal/cni/generator.go`
- `services/agent/internal/runtime/containerd/runtime.go`
- `services/agent/internal/runtime/image_naming.go`
- `docker-compose.containerd.yml`
