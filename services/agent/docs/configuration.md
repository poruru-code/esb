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
| `ENV` | (必須) | 環境名（コンテナ名・ラベルに使用） |
| `PROJECT_NAME` | (空) | Compose プロジェクト名（例: `esb-dev`）。brand 導出に使用 |
| `CONTAINERS_NETWORK` | (必須) | 接続先ネットワーク。brand 導出にも使用 |
| `ESB_BRAND_SLUG` | (空) | brand を明示指定。指定時は最優先で使用 |

## StackIdentity 解決順
Agent は起動時に brand slug を次の順で 1 回だけ解決し、runtime 名称（namespace/CNI/ラベル/イメージ接頭辞）へ反映します。

1. `ESB_BRAND_SLUG`
2. `PROJECT_NAME` と `ENV`（末尾 `-<env>` / `_<env>` を除去して導出）
3. `CONTAINERS_NETWORK`（末尾 `-external` / `_<env>` を除去して導出）

上記いずれでも解決できない場合は Agent 起動を hard fail します。

## gRPC / TLS
| 変数 | デフォルト | 説明 |
| --- | --- | --- |
| `AGENT_GRPC_TLS_DISABLED` | `0` | `1` で mTLS 無効化 |
| `AGENT_GRPC_CERT_PATH` | `/app/config/ssl/server.crt` | サーバ証明書 |
| `AGENT_GRPC_KEY_PATH` | `/app/config/ssl/server.key` | サーバ秘密鍵 |
| `AGENT_GRPC_CA_CERT_PATH` | `/usr/local/share/ca-certificates/rootCA.crt` | クライアント検証用 CA |
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
| `CNI_CONF_FILE` | `<CNI_CONF_DIR>/10-<brand>-net.conflist` | CNI conf ファイル（brand は StackIdentity 由来） |
| `CNI_SUBNET` | `10.88.0.0/16` | CNI サブネット |
| `CNI_GW_IP` | (空) | CNI GW（`CNI_DNS_SERVER` 未指定時の参照先） |
| `CNI_DNS_SERVER` | `10.88.0.1` | nameserver（解決順: `CNI_DNS_SERVER` -> `CNI_GW_IP` -> `10.88.0.1`） |
| `CNI_NET_DIR` | `/var/lib/cni/networks` | IPAM state の保存先（IP 再解決に使用） |

## レジストリ / 画像
| 変数 | デフォルト | 説明 |
| --- | --- | --- |
| `CONTAINER_REGISTRY` | `registry:5010` | 既定の内部レジストリ |
| `CONTAINER_REGISTRY_INSECURE` | `0` | `1` の場合、内部レジストリ通信を insecure として扱う |
| `IMAGE_PULL_POLICY` | `if-not-present` | `always` / `if-not-present`（不正値は `if-not-present` 扱い） |
| `<BRAND_ENV_PREFIX>_TAG` | `latest` | 既定タグ（例: `ESB_TAG`, `ACME_TAG`）。brand 専用キー未設定時は `ESB_TAG` を fallback |

## 運用上の前提
- `EnsureContainer` は `image` の指定を必須とします（`services/contracts/proto/agent.proto`）。
- Image 関数の外部レジストリ解決は deploy/build 側（producer または `artifactctl deploy`）の責務です。
- Agent 実行時は内部レジストリ参照のみを pull します。

---

## Implementation references
- `services/agent/cmd/agent/main.go`
- `services/agent/internal/config/constants.go`
- `services/agent/internal/identity/stack_identity.go`
- `services/agent/internal/runtime/containerd/runtime.go`
- `services/agent/internal/runtime/containerd/image.go`
