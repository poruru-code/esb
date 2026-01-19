# Gateway アーキテクチャレビューと改善計画

`services/gateway` の現状分析に基づき、プロのアーキテクトによる批判的レビューを経て策定されたリファクタリング計画です。単一コンテナという制約の中で「関心の分離 (Separation of Concerns)」を極限まで追求し、テスタビリティと保守性を向上させることを目的としています。

---

## 1. ルーティングとコントローラの分離 (プレゼンテーション層の分離)

**現状 (`main.py`):**
`main.py` (500行+) が FastAPI のエンドポイント定義とビジネスロジック、エラーハンドリング、レスポンス変換を全て抱え込んでいます。

**改善案:**
すべてのルートハンドラを `services/gateway/api/routers/` 配下に移動します。
- `system.py`: ヘルスチェック, 認証
- `lambda_api.py`: AWS互換 Invoke API
- `proxy.py`: Gateway キャッチオールハンドラ

**アーキテクト視点での評価 (Deep Dive):**
*   **妥当性**: 必須。`main.py` は純粋な Composition Root (ワイヤリングのみ) になるべきです。
*   **懸念点**: `proxy.py` が肥大化するリスクがあります。
*   **対策**: `proxy.py` は後述する `RequestProcessor` への委譲のみを行い、ロジック（イベント変換など）を一切持たない「薄いコントローラ」として実装するルールを徹底します。

---

## 2. Invocation のための "Use Case" / Service Layer の導入

**現状:**
「リクエストを受け取り、Lambda用に変換し、実行し、レスポンスを返す」という一連のフローが HTTP ハンドラ内に散在的・手続き的に記述されています。

**改善案:**
このフローを `GatewayRequestProcessor` クラスにカプセル化します。

**アーキテクト視点での評価 (Deep Dive & Testing):**
*   **隠れた結合の指摘**: 単にクラス化するだけでは不十分です。現状の `EventBuilder` は `FastAPI.Request` に依存しており、これをそのまま使うと Service Layer が Web フレームワークに汚染されます。
*   **修正案**: **DTO (`InputContext`) の導入**。
    *   HTTP リクエストから必要な情報（パス、ヘッダ、ボディ、認証情報）だけを抽出した DTO を定義し、Service Layer はこの DTO のみを受け取るようにします。
    *   これにより、テスト時に `Mock(Request)` を作る必要がなくなり、**完全な Social Unit Test** が可能になります。

---

## 3. 依存性注入 (DI) の集約

**現状:**
`main.py` の `lifespan` 関数で手動ワイヤリングを行っています。

**改善案:**
DI ロジックを `core/container.py` に抽出します。

**アーキテクト視点での評価 (Deep Dive):**
*   **Service Locator の排除**: `request.app.state` からインスタンスを取得するのは実質的な Service Locator パターンであり、依存関係が見えにくくなります。
*   **循環参照対策**: `Registry` -> `PoolManager` -> `Invoker` のような相互依存が発生しやすいため、初期化フェーズを明確に分離した Factory パターンまたは軽量な DI コンテナの設計が必要です。
*   **推奨**: `api/deps.py` を完全に Factory のファサードとして機能させ、ハンドラ内では `request.app.state` を直接参照させないようにします。

---

## 4. "Function" ドメインエンティティの統一

**現状:**
設定情報が `dict` のまま各所 (`Registry`, `Matcher`, `Pool`) で使い回されています（Primitive Obsession）。

**改善案:**
Pydantic モデル `FunctionEntity` (または `FunctionConfig`) を導入し、全てのサービス間でこのオブジェクトを受け渡します。

**アーキテクト視点での評価:**
*   **妥当性**: 最も優先度が高いリファクタリングです。型安全性とバリデーションロジックの集約により、バグの温床を排除できます。また、テストデータの作成（Fixture）が標準化され、テスト効率が劇的に向上します。

---

## 5. Observability (可観測性) の分離

**現状:**
アクセスログとトレーシングが混在し、ペイロードログはハンドラにハードコードされています。

**改善案:**
Decorator または Middleware パターンを用いて実装します。

**アーキテクト視点での評価 (Alternative Arch):**
*   **Middleware vs Decorator**: Middleware は Request Body の消費問題があり実装が複雑です。
*   **決定**: `GatewayRequestProcessor` の `process()` メソッドをラップする **Decorator (Interceptor) パターン** を推奨します。これにより、フレームワークに依存せず、かつビジネスロジックを汚さずにログ出力が可能になります。

---

## 6. (New) エラーハンドリングの一貫性

**アーキテクト指摘:**
現在は `global_exception_handler` が全てを拾っていますが、どのレイヤーで例外が発生したかの意味論が失われています。
*   **Service Layer**: ドメイン例外 (`ResourceExhausted`) のみを投げる。
*   **Router Layer**: ドメイン例外を HTTP ステータス (`429`) に変換する責務を持つ。
この境界線を明確に定義すべきです。

---

## 総合評価と推奨ロードマップ

プロのアーキテクトとして、以下の **「実用主義的クリーンアーキテクチャ」** を承認します。
過剰な抽象化（Pipe-and-Filterのフレームワーク化など）は避けつつ、**DTOによる境界づけ** を最重要視します。

**実装ロードマップ:**

1.  **Phase 1: Domain Base (優先度: 高)**
    *   `FunctionEntity` (Pydantic) の導入。辞書回しの廃止。
2.  **Phase 2: Service Layer & DTO (優先度: 最高)**
    *   `InputContext` DTO の定義。
    *   `GatewayRequestProcessor` の実装（Webフレームワーク非依存でテスト可能にする）。
    *   *ここがテスタビリティ向上の要です。*
3.  **Phase 3: Presentation & DI (優先度: 中)**
    *   `main.py` の解体と Router への移動。
    *   DI Factory の整備。
4.  **Phase 4: Cross-Cutting Concerns (優先度: 低)**
    *   Observability Decorator の適用。

この順序で進めることで、最もクリティカルな「ロジックの複雑さ」と「テストのしにくさ」を最初に解消できます。
