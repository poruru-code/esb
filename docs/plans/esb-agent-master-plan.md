実装チームに共有するための、詳細な「プロジェクト移行マスタープラン」を策定しました。

このドキュメントは、既存の Python 製 Orchestrator を **Go 言語製のエージェント (ESB-Agent)** へ移行し、最終的に **containerd を直接制御** することで、AWS Lambda の挙動（高速起動・Frozen 状態）をローカルで完全再現することを目的としています。

---

# Project Migration Master Plan: ESB-Agent Transition

## 1. プロジェクト概要

本計画は、`edge-serverless-box` のコアコンポーネントであるコンテナ管理基盤を刷新するものです。
現在の「Docker API を叩く Python サーバー」から、「**containerd を直接制御する Go エージェント**」へ移行し、以下の実現を目指します。

* **AWS Fidelity**: MicroVM (Firecracker) への移行を見据えた、namespaced containerd による完全な隔離とライフサイクル管理。
* **High Performance**: gRPC 通信と Go 言語による並行処理、および containerd 直接操作による起動オーバーヘッドの極小化（ミリ秒オーダー）。
* **Maintainability**: Gateway (Python) と Runtime (Go) の責務を明確に分離し、型安全な gRPC インターフェースで結合する。

## 2. フェーズ分けとマイルストーン

リスクを最小化するため、以下の3フェーズで段階的に実施します。

| フェーズ | 期間目安 | 概要 | 成果物 |
| --- | --- | --- | --- |
| **Phase 0** ✅ | 1 week | **Gateway リファクタリング** *(完了)*<br>

<br>バックエンド抽象化と Legacy Mode の廃止 | Strategy パターン化された Gateway<br>

<br>クリーンな `LambdaInvoker` |
| **Phase 1** | 2 weeks | **Go Agent 基盤構築 (Hybrid)**<br>

<br>gRPC 通信の開通と Go サーバーの立ち上げ | `services/agent` (Go)<br>

<br>`proto` 定義<br>

<br>Gateway との疎通 |
| **Phase 2** | 2 weeks | **containerd Native 化**<br>

<br>Docker API 依存からの脱却と高速化 | containerd 直叩きロジック<br>

<br>Task API (Pause/Resume) 実装 |

---

## 3. 詳細実装計画

### Phase 0: Gateway Refactoring (Python Side)

**目的**: バックエンド（Python PoolManager / Go Agent）を自由に差し替えられるよう、Gateway のコードを整理する。

* **Step 0.1: `InvocationBackend` Protocol の導入**
* `services/gateway/services/lambda_invoker.py` に Protocol を定義。
* 既存の `PoolManager` をこの Protocol に適合させる。


* **Step 0.2: `LegacyBackendAdapter` の導入 (Temporary)**
* Legacy Mode (`HttpContainerManager`) をラップし、`InvocationBackend` として振る舞うアダプタークラスを作成。
* `LambdaInvoker` 内の `if self.pool_manager:` 分岐を削除し、コンストラクタで渡された `backend` だけを使うように修正。


* **Step 0.3: Legacy Mode の完全削除**
* `main.py` を修正し、常に `PoolManager` (または将来の AgentClient) を初期化するように変更。
* `LegacyBackendAdapter` および `HttpContainerManager` 関連ファイルを削除。


* **Step 0.4: `main.py` のクリーンアップ**
* `ProvisionClient` 等のインラインクラスを `services/gateway/services/clients.py` 等へ移動。



### Phase 1: Go Agent Foundation (Architecture Transition)

**目的**: Python/Go 間を gRPC で接続し、新アーキテクチャの骨組みを完成させる。

* **Step 1.1: ディレクトリ構成と `.proto` 定義**
* プロジェクトルートに `proto/agent.proto` を作成。
* `EnsureContainer`, `Heartbeat`, `StopContainer` 等の RPC を定義。
* `buf` または `protoc` を用いて、Go と Python のコード生成を行うワークフローを確立。


* **Step 1.2: Go Agent (Skeleton) の実装**
* `services/agent/cmd/agent/main.go`: gRPC サーバー起動処理。
* `services/agent/internal/api/`: gRPC ハンドラーの実装。
* **重要**: この段階では、Go 側の内部ロジックは「Docker SDK for Go」を使って既存の Orchestrator と同じ動きをするだけで良い（containerd 化は Phase 2）。


* **Step 1.3: Gateway 側 gRPC Client の実装**
* `services/gateway/services/grpc_backend.py` を作成。
* Phase 0 で作った `InvocationBackend` Protocol を実装し、gRPC 経由で Go Agent を叩くロジックを書く。


* **Step 1.4: 統合テスト**
* `docker-compose.yml` の `orchestrator` を Go 版のビルドに切り替え、`esb up` で E2E テストが通ることを確認。



### Phase 2: Deep Dive (containerd Native)

**目的**: Docker デーモンをバイパスし、containerd を直接制御することで性能と機能を最大化する。

* **Step 2.1: containerd 接続環境の整備**
* Go Agent に `github.com/containerd/containerd` を導入。
* `docker-compose.yml` (および DinD 構成) で、`/run/containerd/containerd.sock` を Agent コンテナにマウント。


* **Step 2.2: 名前空間とスナップショット管理**
* 名前空間 `esb` を定義 (Docker の `moby` 名前空間と分離)。
* Image Pull 処理の実装 (Smart Pulling / Lazy Loading の検討)。


* **Step 2.3: コンテナライフサイクル実装 (The Core)**
* `EnsureContainer` ロジックを `NewContainer` -> `NewTask` -> `Task.Start` のフローに書き換え。
* Lambda RIE 固有の設定（環境変数、エントリポイント）を OCI Spec として定義。


* **Step 2.4: Fast Freeze/Thaw (Pause/Resume)**
* 現在の `docker stop` (SIGTERM) の代わりに、cgroups の freezer を利用した `task.Pause()` / `task.Resume()` を実装。
* これにより、アイドル時の CPU 消費ゼロと、瞬時の復帰を実現。



---

## 4. 技術仕様書 (Technical Specifications)

### 4.1 ディレクトリ構成 (移行後)

```text
.
├── proto/                       # [NEW] gRPC Schema
│   └── agent.proto
├── services/
│   ├── gateway/                 # Python (FastAPI)
│   │   ├── pb/                  # Generated Python gRPC code
│   │   └── services/
│   │       └── grpc_backend.py  # InvocationBackend Impl
│   └── agent/                   # [NEW] Go Agent
│       ├── cmd/agent/           # Entrypoint
│       ├── internal/
│       │   ├── api/             # gRPC Handlers
│       │   └── runtime/         # Container Runtime Interface
│       │       └── containerd/  # Phase 2 Logic
│       ├── go.mod
│       └── Dockerfile
└── ...

```

### 4.2 gRPC Definition (`proto/agent.proto`) Draft

```protobuf
syntax = "proto3";
package esb.agent.v1;

service AgentService {
  // コンテナの確保（起動または再利用）
  rpc EnsureContainer (EnsureContainerRequest) returns (EnsureContainerResponse);
  
  // 定期ハートビート（使用中コンテナの維持）
  rpc Heartbeat (HeartbeatRequest) returns (HeartbeatResponse);
  
  // 明示的な停止・削除
  rpc DestroyContainer (DestroyContainerRequest) returns (DestroyContainerResponse);
}

message EnsureContainerRequest {
  string function_name = 1;
  string image_uri = 2;
  map<string, string> environment = 3;
  // リソース制限などはここで指定
  int32 memory_mb = 4;
}

message EnsureContainerResponse {
  string container_id = 1;
  string ip_address = 2;
  int32 port = 3;
}
// ...

```

## 5. リスク管理と対応

* **開発環境の差異 (Mac/Windows vs Linux)**
* **リスク**: Docker Desktop 環境ではホストから `containerd.sock` が直接見えない。
* **対応**: 開発は基本的に `esb up` で立ち上げた **DinD コンテナ内**、または **WSL 2** で行うことを推奨フローとする。


* **後方互換性**
* Phase 1 の段階では Docker API を使う Go 実装を行うため、containerd が使えない環境へのフォールバック（ドライバ切替機能）を `internal/runtime` インターフェースで吸収する設計にしておくことが望ましい。



## 6. 次のアクション

1. **Phase 0 の着手**: [implementation_plan.md.resolved] に従い、`LambdaInvoker` のリファクタリングを開始してください。
2. **`LegacyBackendAdapter` の実装**: Phase 0 Step 1 で必ず作成し、安全に移行を進めてください。