<!--
Where: docs/reports/cli_architecture_review_reassessment.md
What: Objective reassessment of architecture_review.md.resolved against current CLI implementation.
Why: Provide strict, evidence-based judgment with alternatives in three passes.
-->
# CLI アーキテクチャレビュー再評価（3 Passes）

対象: `cli/`（Go CLI）  
参照: `architecture_review.md.resolved`, `docs/developer/cli-architecture.md`

---

## Pass 1: 事実整合性チェック（指摘ごとの妥当性）

- **Findings**
- 妥当: `runUp` は build/provision/wait/prompt を内包し、単なるディスパッチではない (`cli/internal/app/up.go`)
- 妥当: `internal/app` は 52 ファイルでコマンド/テスト/ヘルパーが混在し、層分離が弱い (`cli/internal/app`)
- 一部妥当: generator は「Python workflow の Go 実装」と明記され移植色が強いが、Go に不適な構造が実害化している証拠は薄い (`cli/internal/generator/generate.go`, `cli/internal/generator/go_builder.go`)
- 妥当: `Dependencies` はサービスロケータに近く nil チェック前提 (`cli/internal/app/app.go`, `cli/internal/app/up.go`)
- 妥当: 依存の手動ワイヤリングがエントリポイントに集中 (`cli/cmd/esb/cli.go`, `cli/cmd/esb/main.go`)
- 妥当: `EnsureAuthCredentials`/`DiscoverAndPersistPorts` が副作用の大きいグローバル関数で DI を迂回 (`cli/internal/app/auth.go`, `cli/internal/app/ports.go`)
- 妥当: プロンプトがコマンド処理内に入り、非対話実行がフラグ運用に依存 (`cli/internal/app/command_context.go`, `cli/internal/app/project.go`, `cli/internal/app/env.go`)
- 一部妥当: 起動時に Docker クライアントを必ず生成するため軽量コマンドでも初期化が走るが、現状の実コストは限定的 (`cli/cmd/esb/main.go`, `cli/internal/compose/client.go`)
- 妥当: 出力が `fmt` と UI helper で混在し、出力仕様が一貫しない (`cli/internal/app/*.go`, `cli/internal/ui`)

- **Counterpoints**
- generator/compose/state はパッケージ分割されており、CLI 全体が完全なモノリスとは言い切れない (`cli/internal/generator`, `cli/internal/compose`, `cli/internal/state`)
- Kong は `required/enum` 等のタグと `Validate()` による拡張バリデーションを提供するため、検証を構造体側へ寄せる余地はある (Context7: `/alecthomas/kong`)

- **Alternatives**
- Kong の `Validate()` を導入し、入力検証を CLI 構造体に集約
- `runUp` 相当のオーケストレーションを `workflow` に切り出し、CLI は DTO 組み立てに限定

---

## Pass 2: 設計品質の厳しめ評価（影響度順）

- **Findings**
- High: コマンドハンドラがワークフローを内包し、テスト/再利用が困難 (`cli/internal/app/up.go`)
- High: `Dependencies` の肥大化と nil チェック運用は型安全性を失い、欠落依存がランタイムまで露見しない (`cli/internal/app/app.go`, `cli/cmd/esb/cli.go`)
- Medium: UI 入力と実行が混在し、非対話環境の実行がフラグ運用に依存 (`cli/internal/app/command_context.go`, `cli/internal/app/interaction.go`)
- Medium: グローバル設定/FS 依存がコマンド内に散在し、テストと差分導入のコストが高い (`cli/internal/app/project.go`, `cli/internal/app/env.go`)
- Medium: 出力整形の統一層がなく、将来的な JSON 出力・色付け・機械可読性に弱い (`cli/internal/app/*.go`, `cli/internal/ui`)
- Low: 依存の全初期化は現時点では許容だが、機能増加で劣化する構造 (`cli/cmd/esb/main.go`)

- **Alternatives**
- コマンドごとのコンストラクタ注入で `Dependencies` を分割し nil チェックを排除
- `UpWorkflow` / `ProjectWorkflow` を導入し、CLI は「入力収集 → DTO → 実行」に徹する
- `ui` を唯一の出力経路にし、テキスト/JSON を切り替え可能にする

---

## Pass 3: 代案比較と実行性評価

- **Option A: 低リスクの漸進改善**
- `internal/app` を `commands` と `workflows` に分割し、依存は関数引数に限定
- `Validate()` によるバリデーション集約と、プロンプト処理の独立化
- コマンドごとに遅延初期化 (必要時に Docker クライアント生成)

- **Option B: 中規模の再構成**
- `internal/workflows` + `internal/ports` を追加し、Compose/Generator を interface 化
- `Dependencies` を廃止して `NewUpCmd(upper Upper, builder Builder, ...)` へ移行
- `ui` を統一し、`fmt.Fprintln` 直接利用を段階的に排除

- **Option C: フル・ヘキサゴナル**
- `core/workflows` + `adapters/cli|container|ui` の三層構成に移行
- 大規模なファイル移動と移行期間が必要で、短期コストは高い
- CLI がプラットフォーム化する予定がある場合のみ正当化

- **Architect Verdict (Strict)**
- 指摘の大半は現行コードと整合しており、特に「ハンドラ肥大」「依存肥大」「UI混在」は妥当
- 「Python 構造の投影」は根拠薄で、実害が出るまで優先度を上げるべきではない
- 目標が CLI の長期拡張性なら Option B が最も費用対効果が高い
