**Phase 1: Go Agent Foundation (Architecture Transition)** の詳細実装プランを提示します。

このフェーズのゴールは、**Gateway (Python) と Agent (Go) を gRPC で接続し、Docker SDK を用いて既存機能（コンテナ起動）を Go 側で再現すること**です。まだ containerd を直接操作するフェーズ (Phase 2) ではありませんが、アーキテクチャの骨格を完成させます。

---

# Phase 1: Go Agent Foundation 実装プラン

## 1. ディレクトリ構成の変更

以下のように `proto` と `services/agent` を新設します。

```text
.
├── proto/                       # [NEW] gRPC定義
│   └── agent.proto
├── services/
│   ├── agent/                   # [NEW] Go実装
│   │   ├── cmd/agent/main.go
│   │   ├── internal/
│   │   │   ├── api/             # gRPC Handler
│   │   │   └── runtime/         # Docker操作ロジック
│   │   ├── go.mod
│   │   └── Dockerfile
│   └── gateway/                 # [Modified] Python実装
│       ├── pb/                  # [NEW] 生成されたPythonコード
│       └── services/grpc_backend.py
└── ...

```

---

## Step 1: Protocol Buffers 定義 (`proto/`)

Gateway と Agent 間の通信契約を定義します。

**Task 1.1: `proto/agent.proto` の作成**

```protobuf
syntax = "proto3";

package esb.agent.v1;
option go_package = "github.com/poruru/edge-serverless-box/services/agent/pkg/api/v1";

service AgentService {
  // コンテナを確保し、接続情報を返す (なければ起動、あれば再利用)
  rpc EnsureContainer (EnsureContainerRequest) returns (WorkerInfo);
  
  // 明示的にコンテナを停止・削除する
  rpc DestroyContainer (DestroyContainerRequest) returns (DestroyContainerResponse);
}

message EnsureContainerRequest {
  string function_name = 1;
  string image = 2;
  map<string, string> env = 3;
}

message DestroyContainerRequest {
  string function_name = 1;
  string container_id = 2;
}

message DestroyContainerResponse {
  bool success = 1;
}

message WorkerInfo {
  string id = 1;
  string name = 2;
  string ip_address = 3;
  int32 port = 4;
}

```

**Task 1.2: コード生成ツールの準備**
`protoc` コマンドを使用して、Go と Python のコードを生成するスクリプト（または `lefthook` / `Makefile`）を用意します。

* **Go出力先**: `services/agent/pkg/api/v1/`
* **Python出力先**: `services/gateway/pb/`

---

## Step 2: Go Agent の実装 (`services/agent/`)

Go エージェントの基盤を作成します。Phase 1 では `Docker SDK for Go` を使用して、Python 版 `Orchestrator` のロジックを移植します。

**Task 2.1: モジュール初期化と依存関係**

```bash
cd services/agent
go mod init github.com/poruru/edge-serverless-box/services/agent
go get google.golang.org/grpc
go get github.com/docker/docker/client

```

**Task 2.2: `internal/runtime/docker.go` (Docker操作)**
コンテナの起動ロジックを実装します。

* `Ensure` メソッド:
1. `docker ps` で `label=esb_function={function_name}` を検索。
2. 存在すればその IP を返す。
3. 存在しなければ `docker run` (または `create` + `start`) を実行。
4. ネットワーク接続 (bridge network) や環境変数の注入を行う。
5. 起動したコンテナの IP を取得して返す。



**Task 2.3: `internal/api/server.go` (gRPC Server)**
自動生成された `UnimplementedAgentServiceServer` を埋め込み、各 RPC メソッドを実装します。`runtime` パッケージのメソッドを呼び出します。

**Task 2.4: `cmd/agent/main.go**`
TCP リスナーを作成し、gRPC サーバーを起動します。

---

## Step 3: Gateway Client の実装 (`services/gateway/`)

Phase 0 で定義した `InvocationBackend` Protocol に準拠した gRPC クライアントを実装します。

**Task 3.1: `services/gateway/pb/` のセットアップ**
生成された `agent_pb2.py` と `agent_pb2_grpc.py` を配置し、import できるようにします。

**Task 3.2: `services/gateway/services/grpc_backend.py` の実装**

```python
import grpc
from typing import Any
from services.common.models.internal import WorkerInfo
from services.gateway.pb import agent_pb2, agent_pb2_grpc

class GrpcBackend:
    def __init__(self, agent_address: str):
        self.channel = grpc.aio.insecure_channel(agent_address)
        self.stub = agent_pb2_grpc.AgentServiceStub(self.channel)

    async def acquire_worker(self, function_name: str) -> WorkerInfo:
        # TODO: RegistryからImage/Envを取得してリクエストを組む必要がある
        # Phase 1ではConfigから渡すか、RegistryをBackendに渡す設計が必要
        req = agent_pb2.EnsureContainerRequest(
            function_name=function_name,
            image="...", # ここは呼び出し元やRegistryとの連携が必要
            env={}
        )
        resp = await self.stub.EnsureContainer(req)
        
        return WorkerInfo(
            id=resp.id,
            name=resp.name,
            ip_address=resp.ip_address,
            port=resp.port
        )

    async def release_worker(self, function_name: str, worker: Any) -> None:
        # Agent側で管理するため、Gateway側では何もしない
        pass

    async def evict_worker(self, function_name: str, worker: Any) -> None:
        req = agent_pb2.DestroyContainerRequest(
            function_name=function_name,
            container_id=worker.id
        )
        await self.stub.DestroyContainer(req)
        
    async def close(self):
        await self.channel.close()

```

**Task 3.3: `main.py` の修正**
`config.USE_GRPC_BACKEND` (仮) のようなフラグ、またはデフォルト動作として、`GrpcBackend` を `LambdaInvoker` に渡すように変更します。

---

## Step 4: 統合とデプロイ (`docker-compose.yml`)

**Task 4.1: `docker-compose.yml` の更新**
既存の `orchestrator` (Python) を `agent` (Go) に置き換えます。

```yaml
services:
  agent:
    build: 
      context: .
      dockerfile: services/agent/Dockerfile
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock # Phase 1はまだDocker Socketが必要
    environment:
      - PORT=50051
    networks:
      - internal_network

  gateway:
    environment:
      - AGENT_ENDPOINT=agent:50051
    depends_on:
      - agent

```

## 検証項目 (Definition of Done)

1. **gRPC 疎通確認**: Gateway から Go Agent へリクエストが飛び、エラーなく応答すること。
2. **コンテナ起動**: `esb up` 後、関数を呼び出すと Go Agent 経由で Docker コンテナが立ち上がり、Lambda (RIE) の応答が返ってくること。
3. **既存機能の維持**: E2E テスト (Echo Function など) が Pass すること。

このプランで実装を進めてください。特に Task 1.1 の Proto 定義から開始をお願いします。