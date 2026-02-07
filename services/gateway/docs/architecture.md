<!--
Where: services/gateway/docs/architecture.md
What: Gateway architecture, flows, and key collaborations.
Why: Provide a concise, code-grounded overview of request handling.
-->
# Gateway アーキテクチャ

## 概要
Gateway は FastAPI の HTTP エンドポイントとして動作し、**RouteMatcher -> PoolManager -> LambdaInvoker** の順に
リクエストを処理します。ワーカーの起動/削除は **Agent（gRPC）** に委譲されます。

## クラス構造（主要コンポーネント）

```mermaid
classDiagram
    class GatewayRequestProcessor {
      +process_request(context)
    }
    class LambdaInvoker {
      +invoke_function(name, payload, timeout)
    }
    class PoolManager {
      +acquire_worker()
      +release_worker()
      +cleanup_all_containers()
    }
    class ContainerPool
    class HeartbeatJanitor
    class GrpcProvisionClient
    class AgentInvokeClient

    GatewayRequestProcessor --> LambdaInvoker
    LambdaInvoker --> PoolManager : InvocationBackend
    PoolManager --> ContainerPool
    HeartbeatJanitor --> PoolManager
    PoolManager --> GrpcProvisionClient
    LambdaInvoker --> AgentInvokeClient : optional
```

## リクエスト処理フロー

```mermaid
sequenceDiagram
    autonumber
    participant Client
    participant GW as FastAPI
    participant RM as RouteMatcher
    participant PM as PoolManager
    participant GP as GrpcProvisionClient
    participant AG as Agent (gRPC)
    participant WK as Worker (Lambda RIE)

    Client->>GW: HTTP Request
    GW->>RM: match(route)
    RM-->>GW: function_name
    GW->>PM: acquire_worker(function_name)
    PM->>GP: ensure_container(function_name)
    GP->>AG: EnsureContainer(function_name, image, env, owner_id)
    Note over PM,GP: image は functions.yml 由来（内部レジストリ参照）
    AG-->>GP: WorkerInfo(ip, port)
    GP-->>PM: WorkerInfo

    alt AGENT_INVOKE_PROXY=1
        GW->>AG: InvokeWorker(container_id, payload)
        AG->>WK: HTTP POST /invocations
        WK-->>AG: response
        AG-->>GW: InvokeWorkerResponse
    else direct invoke
        GW->>WK: HTTP POST /invocations
        WK-->>GW: response
    end

    GW->>PM: release_worker()
    GW-->>Client: HTTP Response
```

## Image 関数の運用フロー（正式）

```mermaid
flowchart LR
    A[esb deploy --image-prewarm=all] --> B[Source Registry から pull]
    B --> C[Internal Registry registry:5010 へ push]
    C --> D[functions.yml に内部参照 image を出力]
    D --> E[Runtime invoke 時は image を pull]
```

## Image 関数の設定フィールド

| フィールド | 由来 | 役割 |
| --- | --- | --- |
| `image` | functions.yml | 実行時に Agent/Runtime が pull する内部レジストリ参照 |

## 重要ポイント
- **プール管理**は Gateway が実施（Agent は常に新規作成）
- **起動時クリーンアップ**: Gateway 起動時に Agent に `ListContainers` を投げ、
  既存コンテナを削除して状態を揃える（コールドスタート増加のトレードオフあり）
- **L7 invoke 代理**: `AGENT_INVOKE_PROXY=1` で Agent 経由の HTTP proxy を使用可能
- **Image 関数**: 外部レジストリ同期は deploy 側の責務。runtime は内部レジストリのみを参照

---

## Implementation references
- `services/gateway/main.py`
- `services/gateway/services/pool_manager.py`
- `services/gateway/services/container_pool.py`
- `services/gateway/services/janitor.py`
- `services/gateway/services/grpc_provision.py`
- `services/gateway/services/agent_invoke.py`
- `services/gateway/services/lambda_invoker.py`
- `services/gateway/services/processor.py`
