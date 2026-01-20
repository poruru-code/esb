<!--
Where: docs/reports/cli_optionb_migration_plan.md
What: Stage 7 follow-ups for Option B after Phase 3.
Why: Track remaining P0/P1 architecture work.
-->
# CLI Option B - Stage 7 Follow-ups

Stages 0-6 are archived in `docs/reports/cli_optionb_migration_plan_archive.md`.

## Stage 7: Architecture Follow-ups (P0/P1)
- **P0: Dependency wiring safety**
  - Introduce command-level constructors (`NewUpCmd`, `NewBuildCmd`, etc.) and remove nil-checked direct `Dependencies` access.
  - Add a `internal/wire` initializer so main only calls a single builder function.
- **P1: Boundary cleanup**
  - Split command handlers vs shared helpers (e.g., `internal/commands`, `internal/helpers`).
  - Extract prompt handling into a dedicated interaction layer; keep workflows free of interactive logic.
  - Replace remaining `fmt.Fprintln` output with `UserInterface` calls to unify output paths.
  - Centralize config/FS reads before workflow execution to reduce per-command I/O.

### Stage 7 Detailed Tasks

- **Task 7.1 (P0): Command constructors**
  - 作業: `runBuild/runUp/runDown/runLogs/runStop/runPrune` を `New*Cmd` で生成したハンドラに委譲し、必須依存をコンストラクタ引数に固定する。
  - 受け入れ条件: 各コマンドが `Dependencies` の nil チェックを行わない; コンストラクタで欠落依存が検知できる。
  - 現状: build/up/down/logs/stop/prune すべてを構造体＋`Run` に切り出し、`go test ./...` に成功。

- **Task 7.2 (P0): Wire 集約**
  - 作業: `internal/wire` を追加し、main 側の初期化を `wire.BuildCLI()` のような単一関数に集約する。
  - 受け入れ条件: `cli/cmd/esb/main.go` のワイヤリング記述が大幅に縮小; 依存追加時は `internal/wire` のみ更新。
  - 現状: `cli/internal/wire` に依存構築ロジックを移設し、`main` は `wire.BuildDependencies()` を呼ぶだけになった。

- **Task 7.3 (P1): commands/helpers 分割**
  - 作業: `internal/commands`（コマンドハンドラ）と `internal/helpers`（共通処理）の境界を整理する。
  - 受け入れ条件: コマンドエントリが `internal/commands` に集約され、ヘルパーは `internal/helpers` からのみ参照される。

- **Task 7.4 (P1): Prompt/Interaction 分離**
  - 作業: プロンプト/UI 入力を `internal/interaction` に集約し、`command_context` から分離する。
  - 受け入れ条件: 非対話分岐が `interaction` 層に集約され、コマンド側は DTO 構築に専念。

- **Task 7.5 (P1): 出力統一**
  - 作業: `fmt.Fprintln` の残存箇所を `UserInterface` 経由へ移行する。
  - 受け入れ条件: `cli/internal/commands` から `fmt.Fprintln` の直書きが消える; 出力が UI で一元化。

- **Task 7.6 (P1): Config/FS 依存の前段化**
  - 作業: 設定読み込み/FS 解決を workflow 前段へ集約し、コマンド内 I/O を削減する。
  - 受け入れ条件: `project/env` 系コマンドの I/O 依存が helpers に集約され、テストでの差し替えが容易。
