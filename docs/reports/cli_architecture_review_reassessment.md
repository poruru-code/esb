<!--
Where: docs/reports/cli_architecture_review_reassessment.md
What: Objective reassessment of architecture_review.md.resolved against current CLI implementation.
Why: Provide strict, evidence-based judgment with alternatives in three passes.
-->
# CLI アーキテクチャレビュー再評価（未解消事項）

対象: `cli/`（Go CLI）  
参照: `architecture_review.md.resolved`, `docs/developer/cli-architecture.md`

---

## 未解消事項（具体）

**P0 (高)**: `Dependencies` 分割後も nil チェック運用が残り、欠落依存が実行時まで露見; 型安全性が弱い (`cli/internal/commands/app.go`, `cli/internal/commands/deps_split.go`)
**P0 (高)**: 依存の手動ワイヤリングがエントリポイントに集中し、初期化変更のコストが高い (`cli/cmd/esb/cli.go`, `cli/cmd/esb/main.go`)
**P1 (中)**: `internal/commands` と `internal/helpers` の境界がまだ曖昧で、周辺ロジックがコマンド側に残存; 変更影響が広い (`cli/internal/commands`, `cli/internal/helpers`)
**P1 (中)**: プロンプト/対話分岐がコマンド処理内に残り、非対話実行がフラグ前提 (`cli/internal/commands/command_context.go`, `cli/internal/commands/project.go`, `cli/internal/commands/env.go`)
**P1 (中)**: 出力が `fmt` と UI helper で混在し、出力仕様や将来の JSON 化が不安定 (`cli/internal/commands/*.go`, `cli/internal/ui`, `cli/internal/ports/ui.go`)
**P1 (中)**: グローバル設定/FS 依存がコマンド内に散在し、テスト/差分導入が高コスト (`cli/internal/commands/project.go`, `cli/internal/commands/env.go`)
**P2 (低)**: `DiscoverAndPersistPorts` がグローバル関数として残存し、DI を迂回する副作用点が存在 (`cli/internal/helpers/ports.go`, `cli/internal/helpers/port_publisher.go`)
**P2 (低)**: 起動時に Docker クライアント初期化が走る構造が維持され、軽量コマンドでも初期化コストが発生し得る (`cli/cmd/esb/main.go`, `cli/internal/compose/client.go`)
**P3 (低)**: generator の構造が Python 由来である点は検証不足のまま; 実害の証拠は薄いが保守性リスクは残る (`cli/internal/generator/generate.go`, `cli/internal/generator/go_builder.go`)

---

## 対応方針（短期/中期）

**P0 (高)**
- **Dependencies の nil チェック運用**
  - 短期: コマンドごとのコンストラクタに必須依存を引数化し、`Dependencies` からの直接参照を最小化 (`NewUpCmd`, `NewBuildCmd` 等)
  - 中期: `Dependencies` を用途別パッケージに分割し、nil 許容の型を排除; 未設定は構築時にエラー化
- **エントリポイントの手動ワイヤリング集中**
  - 短期: 初期化を `internal/wire` に集約し、main からの組み立てを関数呼び出し化
  - 中期: モジュール単位の factory を導入し、コマンド単位の組み替えが可能な構成に整理

**P1 (中)**
- **commands/helpers の境界曖昧**
  - 短期: コマンドハンドラと共通処理の境界を `internal/commands`/`internal/helpers` で再整理
  - 中期: command 層と workflow 層の依存方向を固定し、コマンド層を縮小
- **プロンプト/対話分岐の混在**
  - 短期: プロンプト処理を `interaction` に集約し、`command_context` から分離
  - 中期: 入力解決を workflow 前の DTO 組み立て専用に切り出し、非対話の仕様を明文化
- **出力経路の混在**
  - 短期: `fmt.Fprintln` を `UserInterface` に置換する移行表を作成
  - 中期: JSON 出力モードに備えた UI 抽象の統合
- **グローバル設定/FS 依存の散在**
  - 短期: 設定/FS 依存を `ports` 経由に寄せる箇所を優先度順に整理
  - 中期: 設定読み込みを workflow 前段に集中し、コマンド内 I/O を削減

**P2 (低)**
- **`DiscoverAndPersistPorts` のグローバル関数**
  - 短期: `PortPublisher` の実装へ移譲し、app からの直接参照を廃止
  - 中期: 状態更新は `ports.StateStore` などに切り出し、副作用を明確化
- **Docker クライアントの早期初期化**
  - 短期: コマンド単位の遅延初期化フラグを導入
  - 中期: 依存注入経路を分割し、Docker が不要なコマンドは初期化をスキップ

**P3 (低)**
- **generator の移植構造**
  - 短期: 構造の健全性を測る設計メモを作成（責務/依存/拡張点）
  - 中期: 実害が出た部分から関数単位で再設計
