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

## 残課題の対応状況（P2/P3）

- **P3 (低)**: `generator`/`manifest`/`renderer` 周りの構造整理とテストカバレッジの強化。
  - ドキュメント: `cli/docs/sam-parsing-architecture.md` に P3 で掲げた責務・入出力の境界を統合。`parser` → `manifest` → `renderer` という依存方向を明文化し、現行 Go 実装の責務を明示。
  - テスト: `TestGenerateFilesRendersRoutingEvents` を追加 (`cli/internal/generator/generate_test.go`) し、parser から manifest を経て renderer までのイベント出力が `routing.yml` へ反映されることを確認。`cd cli && go test ./internal/generator` で通過済み。
  - 他にも既存の renderer snapshot test や `GenerateFilesIntegrationOutputs` が事実上の差分検知を担保しており、中期決議としていた「差分を検知するテスト」は実装済み。

現時点で上記以外に Stage 7 以降に残る P2/P3 項目はなく、レビュー対象で指摘された構造／テストの懸念にも対処済みです。
