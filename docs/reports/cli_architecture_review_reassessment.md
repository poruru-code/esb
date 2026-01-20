<!--
Where: docs/reports/cli_architecture_review_reassessment.md
What: Objective reassessment of architecture_review.md.resolved against current CLI implementation.
Why: Provide strict, evidence-based judgment with alternatives in three passes.
-->
# CLI アーキテクチャレビュー再評価（未解消事項）

対象: `cli/`（Go CLI）  
参照: `architecture_review.md.resolved`, `docs/developer/cli-architecture.md`

---

## Stage 7 反映状況

Stage 7 で掲げた Workflows/Commands/Helpers 分離と周辺課題は一通り反映済みです。対応内容:

- **P0 依存安全**: `cli/internal/commands` をコンストラクタ化/`internal/wire` 経由に統合し、`Dependencies` から nil チェックを排除。
- **P1 境界分割**: `internal/helpers` に `RuntimeEnvApplier`/`PortPublisher`/`CredentialManager`など共通処理を集約。`internal/interaction` でプロンプト/TTY判定を分離し、`cli/internal/commands` は DTO + UI に集中。`legacyUI`/`writeLine` による出力統一も実施。
- **P1 設定/FS前段化**: `helpers.GlobalConfigLoader`/`ProjectConfigLoader`/`ProjectDirFinder` を導入し、コマンドは loader 経由で config/generator.yml を取得。`cli/internal/helpers/config_loader_test.go` で欠損/正規化のふるまいを担保。

現時点で Stage 7 で想定した課題は解消済なので、このセクションでは残存している**P2/P3**についてのみ記載します。

---

## 現在も残る課題（P2/P3）

- **P3 (低)**: Python由来の `generator` 周りの設計は現状維持だが、長期的には `manifest`/`generator` の責務再分割とテスト資産の補強が望ましい。

## 残課題の対応プラン

**P3 (低): generator の構造**
- 短期: `generator` と `manifest` の責務をドキュメント化し、変更が必要な境界を整理（設計メモ）。
  - 作業: `cli/docs/sam-parsing-architecture.md` に統合し、責務/依存/入出力を図示（完了）。
  - 受け入れ条件: 主要な拡張ポイント（parser/renderer/builder）が文書化される（満了）。
- 中期: `renderer`/`parser`/`builder` の依存方向を整理し、最小単位のUTを追加。
  - 作業: `parser` を `manifest` に依存させ、`renderer` は `manifest` のみ参照する構成へ寄せる。
  - 作業: `renderer` で出力差分のスナップショットテストを導入。
  - 受け入れ条件: 既存テンプレートの出力が保持され、差分がUTで検知可能。
- 検証:
  - `cli/internal/generator` と `cli/internal/manifest` にテストケースを拡充。
  - `e2e` のテンプレート生成ケースで回帰がないことを確認。
