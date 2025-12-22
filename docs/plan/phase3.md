提案された「Phase 3: パフォーマンスと拡張性 (Recommended)」について、詳細な実装計画を作成しました。
このフェーズの目的は、**「高負荷時のスループット向上」**と**「部分的な障害が全体に波及しない回復力（Resilience）の確保」**です。

機能的にはPhase 2で完成していますが、このPhase 3を適用することでプロダクション運用に耐えうる品質に引き上げます。

---

### 📋 Phase 3 実装計画概要

1. **キャッシュ機構の刷新**: 自前実装のLRUキャッシュを、検証されたライブラリ（`cachetools`）に置き換え、信頼性を向上。
2. **Circuit Breakerの導入**: 応答しないコンテナへのリクエストを早期に遮断し、リソース枯渇を防ぐ。
3. **HTTPクライアントのチューニング**: コネクションプールの制限を緩和し、コンテナ間通信のレイテンシを削減。

---

### Step 1: キャッシュライブラリの導入と刷新

`ContainerHostCache` の自前実装 (`OrderedDict` ベース) を廃止し、標準的な `cachetools` を使用します。これにより、バグの温床を排除し、TTL（有効期限）管理を正確に行います。

**依存関係の追加:**
`pyproject.toml` に `cachetools` を追加します。

**対象ファイル:** `services/gateway/services/container_cache.py`

**変更内容:**

```python
from typing import Optional
from cachetools import TTLCache
import logging

logger = logging.getLogger("gateway.container_cache")

class ContainerHostCache:
    """
    cachetoolsを使用した堅牢なコンテナホストキャッシュ
    """

    def __init__(
        self,
        max_size: int = 1000,
        ttl_seconds: float = 30.0,
    ):
        # TTLCache: LRUアルゴリズムベースで、かつTTLを持つ
        self._cache = TTLCache(maxsize=max_size, ttl=ttl_seconds)

    def get(self, function_name: str) -> Optional[str]:
        # cachetoolsは期限切れを自動的に処理して KeyError 扱いにする
        return self._cache.get(function_name)

    def set(self, function_name: str, host: str) -> None:
        self._cache[function_name] = host

    def invalidate(self, function_name: str) -> None:
        if function_name in self._cache:
            del self._cache[function_name]
            logger.debug(f"Cache invalidated: {function_name}")

    def clear(self) -> None:
        self._cache.clear()

```

---

### Step 2: Circuit Breaker (サーキットブレーカー) の導入

特定のコンテナがフリーズしたり、過負荷で応答しない場合、Gatewayがタイムアウト待ちでリソースを浪費し続けるのを防ぎます。一定回数失敗したら、一時的にそのコンテナへのアクセスを遮断（Open状態）し、即座にエラーを返します。

今回は軽量な実装を目指し、外部ライブラリを使わずとも実現可能な簡易クラス、または `circuitbreaker` ライブラリの導入を検討します（ここでは依存を増やしすぎない簡易実装例を示します）。

**新規作成:** `services/gateway/core/circuit_breaker.py`

```python
import time
import asyncio
from typing import Callable, Any

class CircuitBreakerOpenError(Exception):
    pass

class CircuitBreaker:
    def __init__(self, failure_threshold: int = 5, recovery_timeout: int = 30):
        self.failure_threshold = failure_threshold
        self.recovery_timeout = recovery_timeout
        self.failures = 0
        self.last_failure_time = 0
        self.state = "CLOSED"  # CLOSED, OPEN, HALF_OPEN

    async def call(self, func: Callable, *args, **kwargs) -> Any:
        if self.state == "OPEN":
            if time.time() - self.last_failure_time > self.recovery_timeout:
                self.state = "HALF_OPEN"
            else:
                raise CircuitBreakerOpenError("Circuit is open")

        try:
            result = await func(*args, **kwargs)
            if self.state == "HALF_OPEN":
                self.reset()
            return result
        except Exception as e:
            self.failures += 1
            self.last_failure_time = time.time()
            if self.failures >= self.failure_threshold:
                self.state = "OPEN"
            raise e

    def reset(self):
        self.failures = 0
        self.state = "CLOSED"

```

**適用:** `services/gateway/services/lambda_invoker.py`

Invoker内で、コンテナごとのCircuit Breakerを管理し、`invoke_function` 内のHTTPコールをラップします。

```python
# LambdaInvokerクラス内

    def __init__(self, ...):
        # ...
        # 関数名ごとのブレーカーを保持
        self.breakers: Dict[str, CircuitBreaker] = {}

    async def invoke_function(self, function_name: str, ...):
        # ...
        
        # ブレーカー取得または作成
        if function_name not in self.breakers:
            self.breakers[function_name] = CircuitBreaker()
        
        breaker = self.breakers[function_name]

        try:
            # ブレーカー経由で実行
            response = await breaker.call(
                self.client.post,
                rie_url,
                content=payload,
                # ...
            )
            return response
        except CircuitBreakerOpenError:
            # 即座にエラーを返す（待機時間ゼロ）
            logger.error(f"Circuit breaker open for {function_name}")
            raise LambdaExecutionError(function_name, "Circuit Breaker Open")

```

---

### Step 3: HTTPクライアントのコネクションプール最適化

デフォルトの `httpx` クライアント設定では、同時接続数やKeep-Alive接続数が制限されており、コンテナ間通信が多いこのシステムではボトルネックになります。

**対象ファイル:** `services/common/core/http_client.py`

**変更内容:**
`limits` パラメータを明示的に設定し、並列数を増やします。

```python
# services/common/core/http_client.py

    def create_async_client(self, **kwargs) -> httpx.AsyncClient:
        # ...
        
        # デフォルトのリミット設定（必要に応じて引数で上書き可能に）
        if "limits" not in kwargs:
            # max_keepalive_connections: プールしておく接続数
            # max_connections: 合計最大接続数（並列リクエスト数に影響）
            kwargs["limits"] = httpx.Limits(
                max_keepalive_connections=20, 
                max_connections=100
            )
            
        return httpx.AsyncClient(verify=verify, **kwargs)

```

---

### ✅ 検証計画 (Verification)

1. **負荷テスト**:
* `locust` 等を使用し、並列リクエスト（例: 50 req/sec）を送信。HTTPクライアントのプール設定変更前後で、スループットやエラー率（Connection pool timeoutなど）が改善するか確認する。


2. **カオスエンジニアリング（擬似障害）**:
* 特定のLambdaコンテナを `docker pause` で一時停止させる。
* その関数へのリクエストを連続して送り、Circuit Breakerが作動して「即座にエラー」が返るようになるか確認する（タイムアウト待ちが発生しなくなること）。
* `docker unpause` 後、一定時間経過で自動復旧することを確認する。


3. **キャッシュ動作確認**:
* Managerへの問い合わせログを確認し、2回目以降のリクエストでManagerへの問い合わせが発生していないことを確認する。


