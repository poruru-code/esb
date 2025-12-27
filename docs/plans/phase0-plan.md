Legacy Mode を**即座に削除する（アダプターを経由しない）**という前提で、**Phase 0: Gateway Refactoring** の実装プランを再構築しました。

中間ステップがなくなるため、作業は非常にシンプルかつ直線的になります。

---

# Phase 0: Gateway Refactoring (Direct Migration)

## 目的

Legacy Mode (`HttpContainerManager`) を完全に排除し、`LambdaInvoker` が抽象化された `InvocationBackend`（実体は `PoolManager`）のみに依存するように修正します。これにより、コードの見通しを良くし、将来の gRPC 対応への準備を完了させます。

## 実装ステップ

### Step 1: Interface の定義と LambdaInvoker の修正

**ゴール**: `LambdaInvoker` から具体的なマネージャークラスへの依存と条件分岐を削除する。

1. **`InvocationBackend` Protocol の定義**
* `services/gateway/services/lambda_invoker.py` の冒頭に Protocol を定義します。
* `PoolManager` が既に持っているメソッド (`acquire_worker`, `release_worker`, `evict_worker`) に合わせます。


2. **`LambdaInvoker` の修正**
* コンストラクタの引数を刷新します。
* **削除**: `container_manager`, `pool_manager`
* **追加**: `backend: InvocationBackend`


* `invoke_function` メソッド内の `if self.pool_manager:` ブロックを削除し、常に `self.backend.acquire_worker(...)` を呼び出すように変更します。
* エラーハンドリング内の `evict` 処理も `self.backend` 経由に統一します。



```python
# services/gateway/services/lambda_invoker.py (変更イメージ)

class InvocationBackend(Protocol):
    async def acquire_worker(self, function_name: str) -> Any: ...
    async def release_worker(self, function_name: str, worker: Any) -> None: ...
    async def evict_worker(self, function_name: str, worker: Any) -> None: ...

class LambdaInvoker:
    def __init__(self, client, registry, config, backend: InvocationBackend):
        self.client = client
        self.registry = registry
        self.config = config
        self.backend = backend  # ここに PoolManager が入る

```

### Step 2: クライアントクラスの分離 (Cleanup)

**ゴール**: `main.py` の肥大化を防ぐため、インライン定義されているクラスを別ファイルへ移動する。

1. **`services/gateway/services/clients.py` の作成**
* `main.py` 内にある `ProvisionClient` クラスと `HeartbeatClient` クラスをここに移動します。
* 必要な import (`httpx`, `WorkerInfo` 等) を追加します。



### Step 3: Main.py の配線変更

**ゴール**: `PoolManager` を標準バックエンドとして初期化し、Legacy Mode の痕跡を消す。

1. **`services/gateway/main.py` の修正**
* `ENABLE_CONTAINER_POOLING` フラグによる `if` 分岐を削除します。
* `HttpContainerManager` の初期化コードを削除します。
* `ProvisionClient`, `HeartbeatClient` を `services.clients` から import します。
* 常に `PoolManager` を初期化し、`LambdaInvoker` の `backend` 引数として渡します。



```python
# services/gateway/main.py (変更イメージ)

# ...
from .services.clients import ProvisionClient, HeartbeatClient
from .services.pool_manager import PoolManager

@asynccontextmanager
async def lifespan(app: FastAPI):
    # ...
    
    # 常に PoolManager を初期化
    provision_client = ProvisionClient(client, config.ORCHESTRATOR_URL)
    backend_manager = PoolManager(provision_client, config_loader)
    
    # ... (Janitor初期化など)

    lambda_invoker = LambdaInvoker(
        client=client,
        registry=function_registry,
        config=config,
        backend=backend_manager,  # Backendとして注入
    )

```

### Step 4: ファイル削除

**ゴール**: 不要になったコードを物理削除する。

以下のファイルを削除します。

* `services/gateway/services/container_manager.py`
* `services/gateway/services/container_cache.py`

---

## 確認事項 (Checklist)

* [ ] `LambdaInvoker` がシンプルになり、`if` 分岐が消滅していること。
* [ ] `main.py` が `PoolManager` のみを使用していること。
* [ ] `esb up` で起動し、既存のテスト機能（Echo関数など）が正常に動作すること。
* [ ] (`ENABLE_CONTAINER_POOLING` 環境変数は無視されるようになるため、`.env` から削除してもよい)

このプランで実装を開始してください。