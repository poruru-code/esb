ご提示いただいた情報とドキュメントの内容に基づき、**Phase 4 Step 1: オートスケーリングと流量制御の再実装** の詳細な実装プランを再構築しました。

既存の `ContainerPool` で採用されていた **`asyncio.Condition` + `deque` による厳密な FIFO キューイング** のロジックを継承し、Go Agent 環境 (`GrpcBackend`) に適用する設計としています。

---

# Phase 4 Step 1: オートスケーリングと流量制御の再実装 (Re-Design)

## 1. 目的とスコープ

Go Agent 化に伴い失われた「同時実行数制限」と「リクエストキューイング」の機能を Gateway に再実装します。
`asyncio.Semaphore` ではなく、旧仕様の **Condition 変数と待機キュー** を用いた制御ロジックを移植することで、以下の要件を満たします。

* **厳密な順序保証 (FIFO)**: 先に到着したリクエストから順に処理する。
* **公平性**: 特定のリクエストが飢餓状態になるのを防ぐ。
* **タイムアウト制御**: 待機時間が上限を超えたリクエストを安全にキューから離脱させる。

## 2. 実装設計

### 新規コンポーネント: `FunctionThrottle`

「1つの関数」に対する流量制御を行うクラスです。コンテナの管理は行わず、**「実行権限の貸出」のみ** を管理します。

* **状態**:
* `limit`: 同時実行数の上限。
* `current`: 現在実行中のリクエスト数。
* `waiters`: `collections.deque`。待機中の `asyncio.Future` を格納。
* `condition`: `asyncio.Condition`。状態変更の同期に使用。



### 管理コンポーネント: `ConcurrencyManager`

関数ごとの `FunctionThrottle` インスタンスを生成・保持するシングルトン的なクラスです。

---

## 3. 実装タスク詳細

### Task 1.1: 設定値の追加

流量制御に必要な環境変数を定義します。

* **対象ファイル**: `services/gateway/config.py`
* **追加項目**:
```python
class Settings(BaseSettings):
    # ...
    # Default concurrency limit per function
    MAX_CONCURRENT_REQUESTS: int = 10
    # Max time (seconds) to wait in queue
    QUEUE_TIMEOUT_SECONDS: int = 10

```



### Task 1.2: `ConcurrencyManager` の実装 (Core Logic)

旧 `ContainerPool` のロジックを抽出し、リファクタリングして実装します。

* **作成ファイル**: `services/gateway/core/concurrency.py` (新規)
* **実装コード案**:

```python
import asyncio
from collections import deque, defaultdict
from typing import Dict, Optional
from services.gateway.core.exceptions import ResourceExhaustedError

class FunctionThrottle:
    """
    関数単位の流量制御クラス。
    asyncio.Condition と deque を使用して FIFO を保証する。
    """
    def __init__(self, limit: int):
        self.limit = limit
        self.current = 0
        self.condition = asyncio.Condition()
        self.waiters: deque[asyncio.Future] = deque()

    async def acquire(self, timeout: float) -> None:
        async with self.condition:
            # 空き枠があれば即取得
            if self.current < self.limit:
                self.current += 1
                return

            # 空きがない場合、Futureを作成して待機列の最後尾に追加 (FIFO)
            waiter = asyncio.get_running_loop().create_future()
            self.waiters.append(waiter)

            try:
                # 順番が来るか、タイムアウトするまで待つ
                await asyncio.wait_for(waiter, timeout)
                # ここに来た時点で、release() によって current枠 は確保済み（譲渡済み）とみなす
                return
            except asyncio.TimeoutError:
                # タイムアウト時: まだ待機列にいれば削除する
                if waiter in self.waiters:
                    self.waiters.remove(waiter)
                raise ResourceExhaustedError("Request timed out in queue")
            except asyncio.CancelledError:
                # クライアント切断等によるキャンセルのハンドリング
                if waiter in self.waiters:
                    self.waiters.remove(waiter)
                raise

    async def release(self) -> None:
        async with self.condition:
            if self.waiters:
                # 待機者がいる場合:
                # 自分の枠を解放せず、次の待機者に「譲渡」する
                next_waiter = self.waiters.popleft()
                if not next_waiter.done():
                    next_waiter.set_result(None)
                # current は減らさない（権利が移動しただけ）
            else:
                # 待機者がいない場合:
                # 単純にカウントを減らす
                if self.current > 0:
                    self.current -= 1
            
            # 状態変化を通知
            self.condition.notify()

class ConcurrencyManager:
    def __init__(self, default_limit: int, default_timeout: int):
        self._default_limit = default_limit
        self._default_timeout = default_timeout
        self._throttles: Dict[str, FunctionThrottle] = defaultdict(
            lambda: FunctionThrottle(self._default_limit)
        )

    def get_throttle(self, function_name: str) -> FunctionThrottle:
        return self._throttles[function_name]
    
    @property
    def default_timeout(self) -> int:
        return self._default_timeout

```

### Task 1.3: `GrpcBackend` への統合

`GrpcBackend` クラス内で `Invoke` を実行する前後で、この `Throttle` を操作するように修正します。

* **対象ファイル**: `services/gateway/services/grpc_backend.py`
* **変更点**:
1. `__init__` で `ConcurrencyManager` を初期化。
2. `invoke` メソッドを `try...finally` ブロックで囲み、確実に `release()` されるようにする。



```python
    # invoke メソッド内の修正イメージ
    async def invoke(self, function_name: str, payload: dict) -> ExecutionResult:
        throttle = self.concurrency_manager.get_throttle(function_name)
        
        # 1. 実行権の取得 (ここで待たされる可能性がある)
        await throttle.acquire(timeout=self.concurrency_manager.default_timeout)
        
        try:
            # 2. 実際の Invoke (Go Agent へ)
            request = agent_pb2.InvokeRequest(
                function_name=function_name,
                payload=json.dumps(payload).encode("utf-8")
            )
            response = await self.stub.Invoke(request)
            
            # ... レスポンス処理 ...
            return result
            
        finally:
            # 3. 実行権の解放 (または次への譲渡)
            # 非同期メソッドなので await が必要だが、context manager外なので taskとして流すか
            # あるいは acquire/release 自体を async context manager にラップするのを推奨
            await throttle.release()

```

> **実装のヒント**: `FunctionThrottle` 自体を非同期コンテキストマネージャ (`__aenter__`, `__aexit__`) にすると、`grpc_backend.py` のコードが非常にきれいになります。

### Task 1.4: エラーハンドリングの確認

`ResourceExhaustedError` が発生した際、HTTP ステータスコード **429 (Too Many Requests)** または **503 (Service Unavailable)** が返却されることを、例外ハンドラ (`main.py` または `api/deps.py`) で確認・修正します。

---

## 4. 検証プラン

### ユニットテスト (`services/gateway/tests/test_concurrency.py`)

新規にテストファイルを作成し、以下のシナリオを検証します。

1. **FIFO順序**: 制限数1の状態で、リクエスト A, B, C を投げ、A終了後に B が開始し、B終了後に C が開始すること。
2. **タイムアウト**: 制限数一杯の状態でリクエストを投げ、指定時間内に空きが出なければ `ResourceExhaustedError` になること。
3. **キャンセル安全性**: 待機中のリクエストがキャンセルされた場合、適切に `waiters` から削除され、システムが停止しないこと。

### E2E テスト (復活)

以下のスキップされていたテストのコメントアウトを外し、パスすることを確認します。

* `tests/scenarios/autoscaling/test_e2e_autoscaling.py`
* `test_concurrent_queueing`: 最重要。この実装によりパスするはずです。


* `tests/scenarios/autoscaling/test_scale_out.py`
* `test_respects_max_capacity`: 設定した `MAX_CONCURRENT_REQUESTS` 以上にリクエストが並列実行されないこと。



---

## 5. 完了基準

* `services/gateway/core/concurrency.py` が作成されている。
* `GrpcBackend` がリクエスト時に `throttle.acquire` 待ちを行うようになっている。
* `pytest tests/scenarios/autoscaling/test_e2e_autoscaling.py` が **PASS** する。

このプランに従い、実装を開始してください。