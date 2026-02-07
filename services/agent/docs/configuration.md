<!--
Where: services/agent/docs/configuration.md
What: Agent runtime configuration and environment variables.
Why: Centralize tunables used by Agent and its gRPC server.
-->
# Agent 設定（環境変数）

## 主要設定
| 変数 | デフォルト | 説明 |
| --- | --- | --- |
| `AGENT_RUNTIME` | `docker` | 実行ランタイム（`docker` / `containerd`） |
| `PORT` | `50051` | gRPC リッスンポート |
| `AGENT_METRICS_PORT` | `9091` | Prometheus `/metrics` の公開ポート |
| `CONTAINERS_NETWORK` | `bridge` | Docker runtime の接続先ネットワーク |
| `ENV` | `default` | 環境名（コンテナ名・ラベルに使用） |

## gRPC / TLS
| 変数 | デフォルト | 説明 |
| --- | --- | --- |
| `AGENT_GRPC_TLS_DISABLED` | `0` | `1` で mTLS 無効化 |
| `AGENT_GRPC_CERT_PATH` | `/app/config/ssl/server.crt` | サーバ証明書 |
| `AGENT_GRPC_KEY_PATH` | `/app/config/ssl/server.key` | サーバ秘密鍵 |
| `AGENT_GRPC_CA_CERT_PATH` | `meta.RootCACertPath` | クライアント検証用 CA |
| `AGENT_GRPC_REFLECTION` | `0` | `1` で Reflection 有効化 |

## Invoke 代理
| 変数 | デフォルト | 説明 |
| --- | --- | --- |
| `AGENT_INVOKE_MAX_RESPONSE_SIZE` | `10485760` | `InvokeWorker` の最大レスポンスサイズ（bytes） |

## containerd / CNI
| 変数 | デフォルト | 説明 |
| --- | --- | --- |
| `CONTAINERD_SOCKET` | `/run/containerd/containerd.sock` | containerd のソケット |
| `CONTAINERD_RUNTIME` | (空) | `aws.firecracker` などの runtime 名 |
| `CONTAINERD_SNAPSHOTTER` | (空) | snapshotter 強制指定 |
| `CNI_CONF_DIR` | `/etc/cni/net.d` | CNI conf dir |
| `CNI_BIN_DIR` | `/opt/cni/bin` | CNI plugin dir |
| `CNI_CONF_FILE` | `<CNI_CONF_DIR>/10-<cni>.conflist` | CNI conf ファイル |
| `CNI_SUBNET` | `10.88.0.0/16` | CNI サブネット |
| `CNI_GW_IP` | (空) | CNI GW（DNS の既定参照） |
| `CNI_DNS_SERVER` | `10.88.0.1` | nameserver（未指定時は `CNI_GW_IP`） |
| `CNI_NET_DIR` | `/var/lib/cni/networks` | IPAM state の保存先（IP 再解決に使用） |

## レジストリ / 画像
| 変数 | デフォルト | 説明 |
| --- | --- | --- |
| `CONTAINER_REGISTRY` | `registry:5010` | 既定の内部レジストリ |
| `CONTAINER_REGISTRY_INSECURE` | `0` | `1` の場合、内部レジストリ通信を insecure として扱う |
| `<ENV_PREFIX>_TAG` | `latest` | 既定タグ（互換経路用。通常は Gateway から `image` を明示指定） |

## 運用上の前提
- `EnsureContainer` は `image` の指定を必須とします（`proto/agent.proto`）。
- Image 関数の外部レジストリ解決は `esb deploy --image-prewarm=all` 側の責務です。
- Agent 実行時は内部レジストリ参照のみを pull します。

---

## Implementation references
- `services/agent/cmd/agent/main.go`
- `services/agent/internal/config/constants.go`
- `services/agent/internal/runtime/containerd/runtime.go`
