# オートスケーリングとコンテナ・ライフサイクル管理 (v2.1)

## 概要

Edge Serverless Box (ESB) は、Lambda関数の同時実行リクエストを効率的に処理し、リソース利用効率を最大化するために、高度なオートスケーリングとコンテナ・プーリング機能（v2.1）を備えています。

新機能である **Scale-to-Zero (Active Pruning)** により、アイドル状態のコンテナを能動的に削除し、完全なリソース解放を実現します。再起動時は Gateway 起動時のクリーンアップで状態をリセットします。

## アーキテクチャ

オートスケーリング機能は主に Gateway 内の `PoolManager` と `ContainerPool` によって制御され、Go Agent (gRPC) と連携してコンテナのライフサイクルを管理します。

```mermaid
flowchart TD
    subgraph Gateway ["Gateway"]
        Invoker["Lambda Invoker"] --> PM["PoolManager"]
        PM --> CP["ContainerPool"]
        PM --> Janitor["HeartbeatJanitor"]
        
        CP -- acquire/release --> PM
    end

    subgraph AgentLayer ["Provisioning Layer"]
        direction TB
        PM -- gRPC --> Agent["Go Agent"]
    end

    subgraph Runtime ["Container Runtime"]
        Agent -- containerd/CNI --> Container["Lambda Container"]
    end

    Janitor -- Prune/Reconcile --> Agent
```

### 主要コンポーネント

1.  **`PoolManager`**: Gateway のエントリポイント。全関数のプールを統括し、起動時のクリーンアップと終了時の全削除 (Shutdown) を指示します。
2.  **`ContainerPool`**: 関数ごとのコンテナプール。`asyncio.Condition` による同時実行数制御と、アイドルコンテナの追跡、および待機待ちリクエストの正確な通知を行います。
3.  **`HeartbeatJanitor`**: 定期的にプールを巡回し、以下の責務を担います。
    *   **Active Pruning**: アイドルタイムアウトを超過したコンテナを検出し、削除リストを作成。
    *   **Reconciliation**: Agent に問い合わせた一覧と Gateway 管理下の差分を検出し、孤児コンテナを削除。
4.  **`Go Agent`**: コンテナの作成・削除を担当する実行エンジン。gRPC ベースの高速なエージェントで `containerd` を直接操作します。起動時の Readiness Check (TCP接続確認) により 502 エラーを防ぎます。

## コンテナ・ライフサイクル

v2.1 では、コンテナの状態遷移がより厳密に管理されています。

### 1. Provisioning (起動)
リクエスト受信時、プールに空きコンテナがなく、かつ最大同時実行数 (`max_capacity`) に達していない場合、Gateway は Go Agent に新規コンテナ作成を依頼します。

### 2. Pooling (待機)
リクエスト処理が完了したコンテナはプールに戻され (`release`)、設定されたタイムアウトまでアイドル状態で待機します。これにより後続リクエストのコールドスタートを防ぎます。

### 3. Scale-to-Zero (自動削除)
v2.1 の核となる機能です。

*   **Active Pruning**: Gateway 側の `Janitor` が定期的にプールをチェックします。最終利用時刻 (`last_used_at`) から `GATEWAY_IDLE_TIMEOUT_SECONDS` を経過したコンテナはプールから除外され、Go Agent に対して即座に削除リクエストが送信されます。
*   **Reconciliation**: Janitor は Agent のコンテナ一覧と Gateway 管理下の差分を比較し、孤児コンテナを削除します（`ORPHAN_GRACE_PERIOD_SECONDS` で作成直後は保護）。

```mermaid
sequenceDiagram
    autonumber
    participant Janitor as HeartbeatJanitor
    participant Pool as ContainerPool
    participant Agent as Go Agent
    participant Container

    Janitor->>Pool: Scan pools
    Pool-->>Janitor: Idle list (last_used_at > timeout)
    Janitor->>Agent: Delete idle containers
    Agent-->>Janitor: Deletion result
    Janitor->>Agent: List containers
    Agent-->>Janitor: Active container list
    Janitor->>Janitor: Reconcile orphan containers
    Janitor->>Agent: Delete orphan containers
    Agent-->>Janitor: Deletion result
```

### 4. Startup Cleanup (再起動時の整理)
Gateway 起動時には、Agent に問い合わせて既存コンテナを一括削除します。これにより状態不整合を回避し、クリーンな状態からプールを再構築します。

### 5. Draining (終了時の排出)
Gateway が正常終了 (SIGTERM) する際、管理下の全コンテナに対して削除リクエストを送信し、リソースをクリーンな状態に戻します。

## 設定

オートスケーリングの挙動は環境変数と `template.yaml` で制御します。

### SAM テンプレート設定

`AWS::Serverless::Function` の `ReservedConcurrentExecutions` プロパティが、その関数の最大同時実行数（プールのキャパシティ）として使用されます。

```yaml
MyFunction:
  Type: AWS::Serverless::Function
  Properties:
    ReservedConcurrentExecutions: 5  # 最大5コンテナまでスケールアウト
```

### 環境変数設定

プーリングとタイムアウトの挙動は以下の環境変数で調整します。**二重タイムアウト設計**により、積極的な削除と安全性確保を両立しています。

| 変数名 | 設定箇所 | 説明 | デフォルト値 |
| :--- | :--- | :--- | :--- |
| `GATEWAY_IDLE_TIMEOUT_SECONDS` | Gateway | **Active Pruning 用**。この時間を超えたアイドルコンテナは Gateway が能動的に削除します。 | `300` (5分) |
| `HEARTBEAT_INTERVAL` | Gateway | Janitor の巡回間隔（秒）。gRPC モードではプルーニング/リコンシリエーションの周期に利用されます。 | `30` |
| `ORPHAN_GRACE_PERIOD_SECONDS` | Gateway | 作成直後のコンテナを孤児削除から保護する猶予時間（秒）。 | `60` |


> [!NOTE]
> `GATEWAY_IDLE_TIMEOUT_SECONDS` は、ユーザー体験（コールドスタート回避）とリソース節約のバランスを決める主要なパラメータです。

## 動作フロー詳細

### リクエスト処理フロー
1.  **Request**: Gateway がリクエスト受信
2.  **Acquire**: `ContainerPool` からワーカー取得
    *   *Idleあり*: 即座に取得して `last_used_at` 更新
    *   *Idleなし*: キャパシティに空きがあれば `Provisioning` 実行。満杯であれば `Condition.wait()` により空きが出るまで待機。
3.  **Invoke**: コンテナに対して Lambda 実行
    *   **Reliability**: `try...finally` ブロックにより、タイムアウトや例外発生時でも確実にワーカーがプールに返却または除外（Evict）されます。
4.  **Release**: コンテナをプールに返却 (`last_used_at` 更新)

```mermaid
sequenceDiagram
    autonumber
    participant Client
    participant Gateway
    participant Pool as ContainerPool
    participant Agent as Go Agent
    participant Container

    Client->>Gateway: Invoke request
    Gateway->>Pool: Acquire worker
    alt Idle container available
        Pool-->>Gateway: Return idle container
    else No idle container
        alt Capacity available
            Gateway->>Agent: Provision container
            Agent-->>Gateway: Container ready
        else At max capacity
            Gateway->>Pool: Condition.wait()
            Pool-->>Gateway: Worker released
        end
    end
    Gateway->>Container: Invoke Lambda
    Container-->>Gateway: Response / completion
    Gateway->>Pool: Release worker (update last_used_at)
    Gateway-->>Client: Response
```

### Janitor フロー (周期実行)
1.  **Pruning**: 各プールをスキャン。`last_used_at` > timeout のコンテナをリストアップ。
2.  **Reconciliation**: Agent の一覧と Gateway の管理情報を比較し、孤児を検出。
3.  **Deletion**: 検出したコンテナを gRPC で削除。

```mermaid
sequenceDiagram
    autonumber
    participant Janitor as HeartbeatJanitor
    participant Pool as ContainerPool
    participant Agent as Go Agent

    Janitor->>Pool: Scan idle containers
    Pool-->>Janitor: Idle list (timeout exceeded)
    Janitor->>Agent: List containers
    Agent-->>Janitor: Active container list
    Janitor->>Janitor: Reconcile + build delete list
    Janitor->>Agent: Delete containers (gRPC)
    Agent-->>Janitor: Deletion result
```

## 制限事項

*   **Cold Start**: Scale-to-Zero 状態からの初回リクエストは、コンテナ起動のため数秒の遅延が発生します。
*   **Gateway 再起動時のクリーンアップ**: Gateway 起動時に既存コンテナを削除するため、再起動直後のリクエストはコールドスタートになります。
