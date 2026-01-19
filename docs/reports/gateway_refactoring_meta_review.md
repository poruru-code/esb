# Gateway リファクタリング計画: 三重批評レポート

両レポート (`gateway_refactoring_report.md` / `gateway_refactoring_critique.md`) に対する、プロのアーキテクトとしての批評的メタレビューです。

---

## Round 1: 構造的・論理的整合性の分析

### 1.1 両レポートの関係性

| 観点 | Report 1 (提案) | Report 2 (批評) |
|------|-----------------|-----------------|
| 対象 | `main.py` (God Object) | `LambdaInvoker.invoke_function` (God Method) |
| 提案 | Router/Service/DI 分離 | Composed Method パターン |
| 共通合意 | `FunctionEntity` 最優先 | ✓ |

**論理的整合性**: 両レポートは**補完関係**にあります。Report 1 は水平分割（レイヤー分離）、Report 2 は垂直分割（メソッド分解）を提案しています。

### 1.2 見落とされている複雑性

両レポートとも、以下の複雑性を見落としています:

**`ContainerPool.acquire` (50行)**: `asyncio.Condition` を使った複雑なロック/アンロックパターンがあります。

```python
async def acquire(self, provision_callback):
    async with self._cv:
        while True:
            if self._idle_workers:
                return self._idle_workers.popleft()
            if len(self._all_workers) + self._provisioning_count < self.max_capacity:
                self._provisioning_count += 1
                break
            await asyncio.wait_for(self._cv.wait(), timeout=remaining)
    # I/O outside lock
    workers = await provision_callback(...)
    async with self._cv:
        ...
```

この「ロックを外してI/Oを行い、再度ロックを取得する」パターンは、並行性のバグ（race condition）を生みやすい設計です。

**Round 1 結論**: 両レポートの視野は `main.py` と `LambdaInvoker` に限定されており、`ContainerPool` の並行性制御の複雑さが盲点になっています。

---

## Round 2: 技術的妥当性の検証

### 2.1 Report 1 の検証

| 提案 | 検証 | 評価 |
|------|------|------|
| Router 分離 | `gateway_handler` は80行で既に薄い | △ 効果は限定的 |
| `InputContext` DTO | `EventBuilder.build` は既に `Request` に依存 | ○ 妥当 |
| `FunctionEntity` | `FunctionRegistry` は `dict` を返している | ○ 必須 |
| DI 集約 | `lifespan` は122行で手動ワイヤリング | ○ 妥当 |

### 2.2 Report 2 の検証

| 提案 | 検証 | 評価 |
|------|------|------|
| Composed Method | `invoke_function` は168行 | ○ 効果大 |
| `_acquire_worker` 分離 | 実際には `backend.acquire_worker` が存在 | △ 既に抽象化されている |
| `_execute_with_breaker` | Circuit Breaker は `_get_breaker` + `breaker.call` で分離済み | △ 過剰提案の恐れ |

**Round 2 発見**: Report 2 の「Composed Method」提案は、既存の抽象化（`InvocationBackend` プロトコル、`CircuitBreaker` クラス）を考慮していません。`invoke_function` の複雑さの約40%は**エラーハンドリング分岐**であり、これは Composed Method では解消されません。

### 2.3 代替案: Result Type パターン

`invoke_function` の複雑さの根本原因は、「成功」「論理エラー」「接続エラー」「タイムアウト」など複数の結果状態を `try/except` で処理していることです。

**提案**: Rust の `Result<T, E>` に相当する `InvocationResult` 型を導入し、例外ではなく型で結果を表現する。

```python
@dataclass
class InvocationResult:
    success: bool
    response: Optional[httpx.Response] = None
    error_type: Optional[str] = None  # "circuit_open", "timeout", "connection", "lambda_error"
    error_detail: Optional[str] = None
```

---

## Round 3: 実装可能性とリスク評価

### 3.1 ロードマップの矛盾

| Report 1 | Report 2 |
|----------|----------|
| Phase 1: `FunctionEntity` | 同意 |
| Phase 2: `GatewayRequestProcessor` + DTO | **批判**: 先に `LambdaInvoker` 分解が必要 |
| Phase 3: Router 分離 | 効果薄いと批判 |
| Phase 4: Observability | 同意 |

**問題点**: Phase 2 と Report 2 の「Phase 1.5」が衝突しています。どちらを先にすべきか明確でありません。

### 3.2 リスク分析

| リスク | 両レポートでの言及 | 評価 |
|--------|-------------------|------|
| 並行性バグ (`ContainerPool`) | なし | ❌ 重大な見落とし |
| gRPC チャネルリーク | Report 2 で言及 | ○ |
| テスト戦略 | 不十分 | ❌ |

特に、**テスト戦略の欠如**が深刻です。「テスタビリティ向上」を目標に掲げながら、具体的なテスト計画（何をどのレベルでテストするか）がありません。

### 3.3 テスト戦略の提案

1. **Unit Test**: `FunctionEntity` のバリデーションロジック
2. **Integration Test**: `GatewayRequestProcessor` + モック `Invoker`
3. **Contract Test**: `InvocationBackend` プロトコルの実装適合性チェック
4. **Stress Test**: `ContainerPool` の並行性リグレッション

---

## 統合的最終評価

| 観点 | Report 1 | Report 2 | 総合 |
|------|----------|----------|------|
| 問題特定 | △ 表層的 | ○ より深い | ○ |
| 解決策 | ○ 妥当 | △ 既存抽象化を無視 | △ |
| ロードマップ | ○ 具体的 | △ 衝突あり | △ |
| リスク分析 | × なし | △ 部分的 | × |
| テスト計画 | × なし | × なし | ❌ |

### 最終推奨ロードマップ（統合版）

1. **Phase 0: テスト基盤整備** (追加)
    * `LambdaInvoker` 既存テストの把握と拡充計画
2. **Phase 1: `FunctionEntity`** (両レポート合意)
3. **Phase 1.5: `InvocationResult` 型導入** (新規提案)
    * `invoke_function` のエラーハンドリングを例外から型へ移行
4. **Phase 2: `InputContext` DTO + `GatewayRequestProcessor`**
5. **Phase 3: DI リファクタリング** (Router 分離は後回し)
6. **Phase 4: Observability / Router 分離**

### 最終判定

両レポートは「どこに問題があるか」を的確に指摘していますが、「どう解決するか」の具体性と「どう検証するか」のテスト戦略が欠落しています。実装に移る前に、**テスト計画書**を別途作成することを強く推奨します。
