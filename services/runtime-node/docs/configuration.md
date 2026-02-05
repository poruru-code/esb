<!--
Where: services/runtime-node/docs/configuration.md
What: runtime-node environment variables and operational settings.
Why: Centralize runtime-node tunables used during startup.
-->
# runtime-node 設定（環境変数）

## 起動モード
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `CONTAINERD_RUNTIME` | (空) | `aws.firecracker` を指定すると Firecracker モード |
| `IMAGE_RUNTIME` | `containerd` | 起動時に検証される固定値 |

## CNI / ネットワーク
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `CNI_GW_IP` | `10.88.0.1` | CNI bridge の GW / DNS 既定 |
| `CNI_SUBNET` | `10.88.0.0/16` | CNI サブネット |
| `CNI_DNS_SERVER` | (空) | nameserver 明示指定 |

## devmapper
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `DEVMAPPER_POOL` | (空) | thin-pool 名（存在必須） |
| `DEVMAPPER_UDEV` | `0` | `1` で udev を使用 |

## WireGuard / ルーティング
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `WG_CONTROL_NET` | (空) | WG 経由で解決するネットワーク |
| `WG_CONTROL_GW` | (空) | WG ルートの明示 GW |
| `WG_CONTROL_GW_HOST` | `gateway` | GW を DNS 解決する場合のホスト名 |

## Firecracker
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `VHOST_VSOCK_REQUIRED` | `0` | `1` で `/dev/vhost-vsock` 必須 |
| `FIRECRACKER_FIFO_READER` | `1` | fifo reader の有効化 |

---

## Implementation references
- `services/runtime-node/entrypoint.sh`
- `services/runtime-node/entrypoint.common.sh`
- `docker-compose.containerd.yml`
