<!--
Where: services/gateway/docs/configuration.md
What: Gateway environment variables and configuration notes.
Why: Provide a stable reference for operators and developers.
-->
# Gateway 設定（環境変数）

## 必須（起動に必要）
| 変数 | 説明 |
| --- | --- |
| `JWT_SECRET_KEY` | JWT 署名キー（32 文字以上） |
| `X_API_KEY` | 認証 API 用の API キー |
| `AUTH_USER` / `AUTH_PASS` | 認証エンドポイントの検証ユーザー |
| `CONTAINERS_NETWORK` | ワーカーが属するネットワーク |
| `GATEWAY_INTERNAL_URL` | Lambda から参照する Gateway URL |

## gRPC / Agent 連携
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `AGENT_GRPC_ADDRESS` | `agent:50051` | Agent の gRPC アドレス |
| `AGENT_INVOKE_PROXY` | `false` | `true` で Agent 経由 invoke |
| `AGENT_GRPC_TLS_ENABLED` | `false` | Gateway → Agent の mTLS |
| `AGENT_GRPC_TLS_CA_CERT_PATH` | `/app/config/ssl/rootCA.crt` | CA 証明書 |
| `AGENT_GRPC_TLS_CERT_PATH` | `/app/config/ssl/client.crt` | クライアント証明書 |
| `AGENT_GRPC_TLS_KEY_PATH` | `/app/config/ssl/client.key` | クライアント秘密鍵 |

## 実行 / パフォーマンス
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `LAMBDA_INVOKE_TIMEOUT` | `30.0` | Lambda invoke タイムアウト |
| `DEFAULT_MAX_CAPACITY` | `1` | 既定の最大同時実行 |
| `DEFAULT_MIN_CAPACITY` | `0` | 既定の最小常駐数 |
| `POOL_ACQUIRE_TIMEOUT` | `30.0` | acquire の待ち時間 |

## レジリエンス
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `CIRCUIT_BREAKER_THRESHOLD` | `5` | 連続失敗閾値 |
| `CIRCUIT_BREAKER_RECOVERY_TIMEOUT` | `30.0` | 復旧待機時間 |
| `HEARTBEAT_INTERVAL` | `30` | Janitor の巡回間隔 |
| `GATEWAY_IDLE_TIMEOUT_SECONDS` | `300` | アイドル削除判定 |
| `ORPHAN_GRACE_PERIOD_SECONDS` | `60` | 孤児保護猶予 |
| `ENABLE_CONTAINER_PAUSE` | `false` | idle pause 有効化 |
| `PAUSE_IDLE_SECONDS` | `30` | pause 判定時間 |

## サービス解決（Lambda 注入）
| 変数 | 既定 | 説明 |
| --- | --- | --- |
| `DATA_PLANE_HOST` | `10.88.0.1` | data plane の GW/DNS |
| `S3_ENDPOINT` | (空) | Lambda へ注入する S3 URL |
| `DYNAMODB_ENDPOINT` | (空) | Lambda へ注入する Dynamo URL |
| `GATEWAY_VICTORIALOGS_URL` | (空) | Lambda へ注入する VictoriaLogs URL |

---

## Implementation references
- `services/gateway/config.py`
- `services/gateway/main.py`
- `docker-compose.docker.yml`
- `docker-compose.containerd.yml`
