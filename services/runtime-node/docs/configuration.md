<!--
Where: services/runtime-node/docs/configuration.md
What: runtime-node environment variables and startup knobs.
Why: Keep docs aligned with entrypoint.sh and entrypoint.common.sh.
-->
# runtime-node 設定（環境変数）

## モード切替
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `IMAGE_RUNTIME` | `containerd` | `entrypoint.sh` の必須チェック値 |
| `CONTAINERD_RUNTIME` | (空) | 空/`containerd` または `aws.firecracker` |
| `CONTAINERD_BIN` | `containerd` | 起動バイナリ |
| `CONTAINERD_CONFIG` | mode 依存 | containerd 設定ファイル |

## cgroup / カーネル
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `CGROUP_DELEGATION` | `1` | cgroup v2 delegation の有効化 |
| `CGROUP_PARENT` | `esb` | 親 cgroup 名 |
| `CGROUP_LEAF` | `runtime-node` | leaf cgroup 名 |
| `CGROUP_CONTROLLERS` | `cpu io memory pids` | 有効化 controller |

## ネットワーク / WG 経路
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `WG_CONTROL_NET` | (空) | 追加 route 対象 CIDR |
| `WG_CONTROL_GW` | (空) | route 用明示 gateway |
| `WG_CONTROL_GW_HOST` | `gateway` | gateway 未指定時の名前解決先 |
| `CNI_SUBNET` | (空) | CNI サブネット（未指定時は Agent が出力する identity file から解決） |
| `CNI_BRIDGE` | (空) | CNI bridge 名（未指定時は Agent identity file から解決） |

## devmapper / Firecracker 補助
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `DEVMAPPER_POOL` | (空) | thin-pool 名（指定時は存在必須） |
| `DEVMAPPER_UDEV` | `0` | `1` で udev 経路を使用 |
| `FIRECRACKER_FIFO_READER` | `1` | Firecracker FIFO reader を有効化 |
| `VHOST_VSOCK_REQUIRED` | `0` | `1` で `/dev/vhost-vsock` 必須 |

## 補足
- `apply_cni_nat` は `CNI_SUBNET` / `CNI_BRIDGE` を解決して iptables ルールを適用します。
- `CNI_SUBNET` / `CNI_BRIDGE` が空の場合、`/var/lib/cni/esb-cni.env` を再読込して追従します（起動後の再適用ループあり）。

---

## Implementation references
- `services/runtime-node/entrypoint.sh`
- `services/runtime-node/entrypoint.containerd.sh`
- `services/runtime-node/entrypoint.firecracker.sh`
- `services/runtime-node/entrypoint.common.sh`
