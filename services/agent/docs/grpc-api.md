<!--
Where: services/agent/docs/grpc-api.md
What: gRPC API contract and behavior notes for the Agent.
Why: Keep runtime control surface and error semantics explicit.
-->
# Agent gRPC API

## 参照（Source of Truth）
- Proto 定義: `services/contracts/proto/agent.proto`
- 実装: `services/agent/internal/api/server.go`

> Generated artifact の配置と生成手順は `proto-generation.md` を参照してください。

## サービス
`esb.agent.v1.AgentService`

## 共通仕様
### `owner_id` は必須
ほぼすべての RPC で `owner_id` が必須です。`container_id` が存在しても `owner_id` が一致しない場合、
Agent は `PermissionDenied` を返します。

### エラーコード（代表例）
- `InvalidArgument`: `owner_id`/`container_id`/`function_name`/`image` が空
- `NotFound`: `container_id` が見つからない
- `PermissionDenied`: `owner_id` が一致しない
- `Internal`: runtime 操作失敗（create/start/list/pull 等）
- `DeadlineExceeded`: `InvokeWorker` の HTTP invoke がタイムアウト
- `ResourceExhausted`: `InvokeWorker` のレスポンスボディが上限超過

## RPC 一覧

### 1) EnsureContainer
目的: 関数コンテナを起動し、接続情報を返します。

入力（抜粋）:
| フィールド | 役割 | 補足 |
| --- | --- | --- |
| `function_name` | 関数名 | 必須 |
| `image` | 実行イメージ | 必須。内部レジストリ参照（例: `registry:5010/...`） |
| `env` | 環境変数 | 任意（map） |
| `owner_id` | 所有権 | 必須 |

出力:
- `WorkerInfo { id, ip_address, port }`

補足:
- Agent はプール管理を行わず、基本的に毎回新規コンテナを作成します。
- 外部レジストリ同期は行いません。`image` が未投入なら runtime pull で失敗します。

#### EnsureContainer のシーケンス
```mermaid
sequenceDiagram
    autonumber
    participant GW as Gateway
    participant AG as Agent
    participant RT as Runtime
    participant IR as Internal Registry

    GW->>AG: EnsureContainer(function_name, image, env, owner_id)
    AG->>RT: Ensure(image)
    RT->>IR: Pull image (if needed)
    RT-->>AG: WorkerInfo
    AG-->>GW: WorkerInfo
```

### 2) DestroyContainer
目的: 指定コンテナを削除します（存在しなければ成功扱いにするケースがあります）。

入力（抜粋）:
- `container_id`（必須）
- `owner_id`（必須）

出力:
- `success: bool`

### 3) PauseContainer / ResumeContainer
目的: warm start のための一時停止/再開 API です。

補足:
- containerd runtime では `task.Pause/Resume` を使用
- docker runtime では現状 `Unimplemented`

### 4) ListContainers
目的: Agent が管理するコンテナ一覧を返します。

入力:
- `owner_id`（必須）

出力:
- `ContainerState[]`（`owner_id` でフィルタ）

### 5) GetContainerMetrics
目的: コンテナメトリクスを取得します（cgroup stats ベース）。

入力:
- `container_id`（必須）
- `owner_id`（必須）

出力:
- `ContainerMetrics`

### 6) InvokeWorker
目的: Worker への HTTP invoke を Agent が代理して行い、結果を返します。

入力（抜粋）:
- `container_id`（必須）
- `path`（任意、空なら RIE 既定）
- `payload`（bytes）
- `headers`（map）
- `timeout_ms`（任意、0/未指定時は既定）
- `owner_id`（必須）

出力:
- `status_code`
- `headers`
- `body`

#### 代理 Invoke のシーケンス
```mermaid
sequenceDiagram
    autonumber
    participant GW as Gateway (AgentInvokeClient)
    participant AG as Agent (AgentServer)
    participant WK as Worker (Lambda RIE)

    GW->>AG: InvokeWorker(container_id, path, payload, headers, timeout_ms, owner_id)
    AG->>AG: getWorkerForOwner() / refreshWorkerCache()
    AG->>WK: HTTP POST http://<ip>:<port><path>
    WK-->>AG: HTTP Response (status, headers, body)
    AG-->>GW: InvokeWorkerResponse (body <= max)
```

#### サイズ上限
`AGENT_INVOKE_MAX_RESPONSE_SIZE`（bytes）でレスポンスサイズを制限します。
超過時は `ResourceExhausted` を返します。

## TLS / 運用系エンドポイント
### mTLS（デフォルト有効）
- 無効化: `AGENT_GRPC_TLS_DISABLED=1`
- 証明書（サーバ側）:
  - `AGENT_GRPC_CERT_PATH`（既定: `/app/config/ssl/server.crt`）
  - `AGENT_GRPC_KEY_PATH`（既定: `/app/config/ssl/server.key`）
  - `AGENT_GRPC_CA_CERT_PATH`（既定: `/usr/local/share/ca-certificates/rootCA.crt`）

### Reflection（デバッグ用）
- 有効化: `AGENT_GRPC_REFLECTION=1`

### Health / Metrics
- gRPC health service を提供
- Prometheus `/metrics` を `AGENT_METRICS_PORT`（既定 `9091`）で公開

---

## Implementation references
- `services/contracts/proto/agent.proto`
- `services/contracts/buf.gen.yaml`
- `services/agent/internal/api/server.go`
- `services/agent/internal/runtime/interface.go`
- `services/agent/cmd/agent/main.go`
- `services/gateway/services/agent_invoke.py`
