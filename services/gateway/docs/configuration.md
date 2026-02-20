<!--
Where: services/gateway/docs/configuration.md
What: Gateway environment variables and defaults.
Why: Keep operator-facing configuration aligned with services/gateway/config.py.
-->
# Gateway 設定（環境変数）

## 必須
| 変数 | 説明 |
| --- | --- |
| `JWT_SECRET_KEY` | JWT 署名キー（32文字以上） |
| `X_API_KEY` | 認証 API キー |
| `AUTH_USER` / `AUTH_PASS` | 認証ユーザー情報 |
| `CONTAINERS_NETWORK` | 互換のため必須のネットワーク名（containerd では直接参照しない経路あり） |
| `GATEWAY_INTERNAL_URL` | Lambda から参照する Gateway URL（未指定時は entrypoint が `DATA_PLANE_HOST` から補完） |

## Agent 連携
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `AGENT_GRPC_ADDRESS` | `agent:50051` | Agent gRPC 接続先 |
| `AGENT_INVOKE_PROXY` | `false` | `true` で Agent L7 代理 invoke |
| `AGENT_GRPC_TLS_ENABLED` | `false` | Gateway->Agent の mTLS（アプリ既定。compose では `1` を明示設定） |
| `AGENT_GRPC_TLS_CA_CERT_PATH` | `/app/config/ssl/rootCA.crt` | CA 証明書 |
| `AGENT_GRPC_TLS_CERT_PATH` | `/app/config/ssl/client.crt` | クライアント証明書 |
| `AGENT_GRPC_TLS_KEY_PATH` | `/app/config/ssl/client.key` | クライアント秘密鍵 |
| `GATEWAY_OWNER_ID` | `HOSTNAME` or `gateway` | Agent 資源の所有者 ID |

## 実行制御
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `LAMBDA_INVOKE_TIMEOUT` | `30.0` | invoke timeout（秒） |
| `DEFAULT_MAX_CAPACITY` | `1` | 関数ごとの既定最大同時実行 |
| `DEFAULT_MIN_CAPACITY` | `0` | 関数ごとの既定最小常駐 |
| `POOL_ACQUIRE_TIMEOUT` | `30.0` | acquire 待機上限（秒） |
| `HEARTBEAT_INTERVAL` | `30` | Janitor 間隔（秒） |
| `GATEWAY_IDLE_TIMEOUT_SECONDS` | `300` | idle 削除判定 |
| `ORPHAN_GRACE_PERIOD_SECONDS` | `60` | orphan 削除猶予 |
| `ENABLE_CONTAINER_PAUSE` | `false` | idle pause を有効化 |
| `PAUSE_IDLE_SECONDS` | `30` | pause 判定秒数 |

## 設定ファイル監視
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `CONFIG_RELOAD_ENABLED` | `true` | runtime-config 監視を有効化 |
| `CONFIG_RELOAD_INTERVAL` | `1.0` | 監視間隔（秒） |
| `CONFIG_RELOAD_LOCK_TIMEOUT` | `5.0` | reload lock timeout |

## 認証/パス
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `AUTH_ENDPOINT_PATH` | `/user/auth/v1` | 認証 endpoint path |
| `JWT_EXPIRES_DELTA` | `3000` | JWT 有効期限（秒） |

## Lambda への注入系
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `DATA_PLANE_HOST` | (空) | data-plane host（未指定時は `entrypoint.sh` が CNI identity から解決） |
| `S3_ENDPOINT` | (空) | Lambda 注入用 S3 endpoint |
| `S3_PRESIGN_ENDPOINT` | (空) | Lambda 注入用の presign 専用公開 S3 endpoint（通常の S3 API 呼び出しには使わない） |
| `DYNAMODB_ENDPOINT` | (空) | Lambda 注入用 Dynamo endpoint |
| `GATEWAY_VICTORIALOGS_URL` | (空) | Lambda 注入用 logs endpoint |

## 補助設定
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `UVICORN_BIND_ADDR` | `0.0.0.0:8000` | bind address |
| `UVICORN_WORKERS` | `4` | uvicorn workers |
| `RUNTIME_CONFIG_DIR` | `/app/runtime-config` | runtime config dir |
| `SEED_CONFIG_DIR` | `/app/seed-config` | seed config dir |
| `ROUTING_CONFIG_PATH` | `/app/runtime-config/routing.yml` | routing path |
| `FUNCTIONS_CONFIG_PATH` | `/app/runtime-config/functions.yml` | functions path |
| `RESOURCES_CONFIG_PATH` | `/app/runtime-config/resources.yml` | resources path |
| `VICTORIALOGS_URL` | (空) | Gateway 自身の送信先 |
| `LOG_PAYLOADS` | `false` | payload ログ出力 |

## 互換/予約設定
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `MAX_CONCURRENT_REQUESTS` | `10` | 互換目的の設定値（現行主経路では未使用） |
| `QUEUE_TIMEOUT_SECONDS` | `10` | 互換目的の設定値（現行主経路では未使用） |

---

## Implementation references
- `services/gateway/config.py`
- `services/gateway/lifecycle.py`
- `services/gateway/services/config_reloader.py`
- `services/gateway/services/grpc_channel.py`
- `docker-compose.docker.yml`
- `docker-compose.containerd.yml`
