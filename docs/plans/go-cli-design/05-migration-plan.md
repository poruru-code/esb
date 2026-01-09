# 移行計画

## Phase 1: Skeleton + Core Commands (4-7 days)

- [ ] `cmd/esb/` 基本構造
- [ ] Kong CLI セットアップ
- [ ] `internal/state/` (状態検出)
- [ ] `esb --help`, `esb status` 動作確認
- [ ] `init`, `build`, `up`, `down`, `prune`
- [ ] `internal/config/`, `internal/compose/`
- [ ] 既存 Python E2E tests で検証

## Phase 2: Generator/Provisioner (2-3 days)

- [ ] `internal/generator/` (SAM parser)
- [x] `internal/provisioner/` (DynamoDB/S3)

## Phase 3: Polish (1-2 days)

- [ ] `logs`, `stop` (watch は廃止)
- [ ] `esb env`, `esb project`
- [ ] ドキュメント更新

## Future: Node/Firecracker

> Firecracker モードの仕様確定次第着手

- [ ] Go CLI 移行中は `esb node` を無効化（移植後に復帰）
- [ ] `esb node` サブコマンド群
- [ ] `internal/node/` (SSH, WireGuard)
- [ ] pyinfra 相当の Go 実装
