# 移行計画

## 現在のステータス (2026/01/10)

- Phase 1 のコアコマンドと `internal/*` の実装は既に Go 側に移行済み (`cmd/esb`, state/compose/config/provisioner/generator コンポーネントを含む)。
- `esb build`, `up`（`--build`含む）, `down`, `reset`, `prune`, `status`, `info`, `logs`, `stop` は Go CLI で動作し、`tools-go` に対して `go test ./...` + `uv run python e2e/run_tests.py --reset --parallel`（docker/containerd）を実行済み。
- `internal/provisioner` による DynamoDB/S3 連携や `state` の検出も Go 側で完了。E2E 操作の確認（`logs`, `stop`, `up`/`down`）も行い、`go run ./cmd/esb --env e2e-docker e2e-containerd ...` でも利用できることを確かめた。

## Phase 1: Skeleton + Core Commands (完了)

- [x] `cmd/esb/` 基本構造
- [x] Kong CLI セットアップ (Kong で `tools-go/cmd/esb` を構成)
- [x] `internal/state/` (状態検出)
- [x] `esb --help`, `esb status` 動作確認
- [x] `init`, `build`, `up`, `down`, `prune`
- [x] `internal/config/`, `internal/compose/`
- [x] 既存 Python E2E tests での検証（docker/containerd 両方通過）

## Phase 2: Generator/Provisioner (進行中/完了)

- [x] `internal/generator/` (SAM parser + gobuilder)  — 各種ビルダ/パーザ周りにテストあり
- [x] `internal/provisioner/` (DynamoDB/S3 等) — Go CLI から呼び出し済み

## Phase 3: Polish (作業中)

- [x] `logs`, `stop` (watch は廃止) — 設計どおりの `docker compose logs/stop` を実装・確認
- [ ] `esb env`, `esb project` — コマンドは実装済みだがドキュメント・UX の完成を要する（global config や generator.yml の更新フローの整備も含む）
- [ ] ドキュメント更新 — README 等に Go CLI 前提の説明を追加し、Python CLI への言及を縮小する

## Future: Node/Firecracker

> Firecracker モードの仕様確定次第着手

- [x] Go CLI 移行中は `esb node` を無効化（`tools-go/cmd/esb` で `node` を拒否）
- [ ] `esb node` サブコマンド群（Go で再実装）
- [ ] `internal/node/` (SSH, WireGuard) — 現状未実装
- [ ] pyinfra 相当の Go 実装（Compute Node のセットアップ/構成管理）
