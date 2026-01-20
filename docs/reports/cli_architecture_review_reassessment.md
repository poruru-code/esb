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

- **P2 (低)**: `PortPublisher` と `DiscoverAndPersistPorts` の連携は helpers に移行したものの、明示的な状態管理（例: `ports.StateStore`）が未整備で副作用の履歴/リカバリが取りづらい。
- **P2 (低)**: Docker クライアントの初期化はワイヤで遅延させたものの、`esb info` など軽量コマンドでも `compose.NewDockerClient` が呼ばれるため一層の遅延化/必要性判定を検討。
- **P3 (低)**: Python由来の `generator` 周りの設計は現状維持だが、長期的には `manifest`/`generator` の責務再分割とテスト資産の補強が望ましい。

## 残課題の対応プラン

**P2 (低): PortPublisher / DiscoverAndPersistPorts**
- 短期: `PortPublisher` の永続化を `ports.StateStore` 相当のインターフェースに抽象化し、`ports.json` への書き込み/読み戻しを明示化する。
  - 作業: `cli/internal/ports` に `StateStore` を追加（`Load(ctx) (state.Ports, error)`, `Save(ctx, ports) error`, `Remove(ctx) error`）。
  - 作業: `cli/internal/helpers/ports.go` を `state_store.go` に分離し、`StateStore` 実装に寄せる。
  - 作業: `cli/internal/helpers/port_publisher.go` は `StateStore` を依存に取り、書き込み/削除を `StateStore` 経由に変更。
  - 受け入れ条件: `PortPublisher` が `os.WriteFile`/`os.Remove` を直接呼ばない。
- 中期: `PortPublisher` の戻り値に「永続化した値」と「検出値」を分けるか、履歴を保存する構造体を導入。
  - 作業: `ports.PortPublishResult` を導入（`Published`, `Detected`, `Changed` など）。
  - 作業: `workflows/up.go` で `PortPublishResult` を受け取ってUI出力を更新。
  - 受け入れ条件: 冪等実行時の差分がテストで検出可能。
- 検証:
  - `helpers/state_store_test.go` で `Load/Save/Remove` を単体検証。
  - `helpers/port_publisher_test.go` で `StateStore` 経由の永続化を検証。

**P2 (低): Docker クライアント初期化**
- 短期: `wire.BuildDependencies` に「軽量コマンド判定」を追加し、`info/version/completion` では Docker client を生成しない分岐を導入。
  - 作業: `BuildDependencies(args []string)` のシグネチャ変更 or `BuildDependencies` に `commandName` を渡す入口を追加。
  - 作業: `commands.commandName` 相当の判定関数を `wire` 側に持たせる（循環を避けるため独立実装）。
  - 受け入れ条件: `esb info`/`esb completion` 実行時に `compose.NewDockerClient` が呼ばれない。
- 中期: Docker依存を遅延生成に変更し、`Up/Logs/Down/Stop/Prune` のみで初期化する。
  - 作業: `wire` から `DockerClientFactory func() (compose.DockerClient, error)` を `Dependencies` に注入。
  - 作業: `helpers.NewDowner`/`NewLogger` などの生成をファクトリ呼び出しに置換。
  - 受け入れ条件: Docker非依存コマンドの起動時間が短縮（プロファイルで確認）。
- 検証:
  - `wire` のUTで `NewDockerClient` 呼び出し回数を検証。
  - `commands` のUTで Docker非依存コマンドが正常に動作することを確認。

**P3 (低): generator の構造**
- 短期: `generator` と `manifest` の責務をドキュメント化し、変更が必要な境界を整理（設計メモ）。
  - 作業: `docs/reports/generator_architecture_review.md` を追加し、責務/依存/入出力を図示。
  - 受け入れ条件: 主要な拡張ポイント（parser/renderer/builder）が文書化される。
- 中期: `renderer`/`parser`/`builder` の依存方向を整理し、最小単位のUTを追加。
  - 作業: `parser` を `manifest` に依存させ、`renderer` は `manifest` のみ参照する構成へ寄せる。
  - 作業: `renderer` で出力差分のスナップショットテストを導入。
  - 受け入れ条件: 既存テンプレートの出力が保持され、差分がUTで検知可能。
- 検証:
  - `cli/internal/generator` と `cli/internal/manifest` にテストケースを拡充。
  - `e2e` のテンプレート生成ケースで回帰がないことを確認。
