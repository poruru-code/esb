# アーキテクト批評レポート: Gateway リファクタリング計画の評価

本文書は `docs/reports/gateway_refactoring_report.md` に対する、プロのソフトウェアアーキテクトとしての批評的評価です。制約条件（コンテナ分離不可）を踏まえ、提案が根本原因を解決しているか、より良い代替案がないかを検証します。

---

## Executive Summary (総括)

**評価: 方向性は正しいが、真の複雑性の所在を見落としている**

レポートは `main.py` の「God Object」問題を正しく指摘し、Router/Service/DI の分離を提案しています。しかし、コードベースを詳細に調査した結果、**最も複雑なコードは `main.py` ではなく `LambdaInvoker.invoke_function` (168行) にある**ことが判明しました。

提案された `GatewayRequestProcessor` は、この複雑なメソッドを「呼び出す」だけであり、その内部の複雑さ（ネストした `try/except`、Circuit Breaker、gRPC フォールバック、Worker の acquire/release/evict）は解消されません。

**結論**: 現行レポートのリファクタリングは必要条件であるが、十分条件ではない。`LambdaInvoker` のリファクタリングを追加すべき。

---

## 詳細評価

### 1. 評価対象: `main.py` の分離提案

**レポートの主張**: `main.py` が God Object である。Router に分離すべき。

**批評**:
*   **正しい点**: `main.py` (537行) はコンポジションルートとして肥大化しており、分離は妥当。
*   **見落としている点**: `gateway_handler` 自体は約80行であり、ロジックの大半は `invoker.invoke_function()` への委譲です。ハンドラはすでに「薄い」。問題は呼び出し先にあります。

**結論**: この提案は**表層的**です。Router に移動しても、根本的な複雑さは `LambdaInvoker` に残ったままです。

---

### 2. 評価対象: `GatewayRequestProcessor` の導入

**レポートの主張**: サービス層を導入し、テスタビリティを向上させる。

**批評**:
*   **正しい点**: DTO (`InputContext`) の導入により、`FastAPI.Request` への依存を排除できる。これはテスタビリティ向上に有効。
*   **見落としている点**: `GatewayRequestProcessor.process()` が `LambdaInvoker.invoke_function()` をそのまま呼び出す場合、テスト時に結局 `LambdaInvoker` をモックする必要があり、テスト困難の問題は移動しただけ。

**根本原因**: `LambdaInvoker.invoke_function` が以下を一手に担っている:
1. Worker の取得 (`backend.acquire_worker`)
2. Circuit Breaker 状態管理
3. gRPC or HTTP の分岐
4. RIE への POST 実行
5. エラー判定ロジック（ステータスコード、Lambda論理エラー）
6. Worker の release/evict

**これは「God Method」であり、これこそが分解すべき対象です。**

---

### 3. 評価対象: `FunctionEntity` の導入

**レポートの主張**: Pydantic モデルで辞書回しを排除。

**批評**:
*   **全面的に同意**。これは最も効果的な改善の一つです。
*   すでに `EventBuilder` は `APIGatewayProxyEvent` (Pydantic) を使用しており、この方向性は実績があります。
*   `FunctionRegistry` の戻り値を `FunctionEntity` にすることで、型安全性とテストの容易さが劇的に向上します。

**結論**: **最優先で実施すべき**。

---

### 4. 見落とされている根本問題

#### 4.1 `LambdaInvoker.invoke_function` の複雑性

このメソッドは168行あり、以下のパターンで根本的にリファクタリング可能です:

**推奨: 「Composed Method」パターンの適用**

```python
# Before: 168行の巨大メソッド
async def invoke_function(self, ...):
    # 全てがここに...

# After: 明確に分離された小さなメソッド群
async def invoke_function(self, ...):
    worker = await self._acquire_worker(function_name)
    try:
        response = await self._execute_with_breaker(function_name, worker, payload)
        return self._validate_response(response)
    except Exception as e:
        await self._handle_failure(function_name, worker, e)
        raise
    finally:
        await self._release_or_evict(function_name, worker)
```

**効果**:
*   各メソッドが単一責務を持つ
*   個別にテスト可能
*   Circuit Breaker ロジックが `_execute_with_breaker` に隔離される

#### 4.2 `lifespan` の複雑性

122行の手動ワイヤリングは DI コンテナに移動すべきという提案は正しいですが、**gRPC チャネルのライフサイクル管理**が暗黙的です。

**推奨**: `GrpcChannelManager` のような専用クラスを導入し、チャネルの作成・クローズを明示的に管理する。

---

## 改訂版ロードマップ

| Phase | 提案（元レポート） | 追加提案（本批評） |
|-------|-------------------|-------------------|
| 1 | `FunctionEntity` 導入 | **同意** (最優先) |
| 2 | `GatewayRequestProcessor` + DTO | DTO 導入は正しいが、**先に `LambdaInvoker` の Composed Method 化が必要** |
| 3 | Router 分離 | これは最後で良い（効果は小さい） |
| 4 | Observability Decorator | 優先度は低いが妥当 |
| **New** | - | **`LambdaInvoker.invoke_function` の分解** (最重要) |

---

## 最終評価

| 観点 | 評価 |
|------|------|
| 問題の特定 | △ 表層的（`main.py`）は正しいが、真の複雑性（`LambdaInvoker`）を見落としている |
| 解決策の妥当性 | ○ 提案された手法（DTO, DI）は妥当 |
| 優先順位 | × Router 分離が Phase 3 は遅すぎるのではなく、逆に**効果が薄い作業が高い優先度についている** |
| テスタビリティ | △ DTO だけでは不十分。Invoker のリファクタが必要 |
| 代替案の検討 | × Pipe-and-Filter への言及はあるが、Composed Method のようなより実践的なパターンへの言及がない |

**最終判定**: 本レポートをそのまま実施するのではなく、**`LambdaInvoker` の Composed Method 化を Phase 1.5 として挿入することを強く推奨**します。これにより、Service Layer 導入時のテスト容易性が飛躍的に向上します。
