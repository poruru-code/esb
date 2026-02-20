<!--
Where: docs/cli-separation-checklist.md
What: Execution checklist for extracting cli/ to a separate repository.
Why: Make split operation repeatable with explicit verification gates.
-->
# CLI 分離チェックリスト

## 目的
`cli/` を別リポジトリへ分離する際に、手順と検証を固定化して事故を防ぐための実行チェックリストです。  
このドキュメントは「分離前後で何を確認すべきか」を定義します。

## 事前条件
- `quality-gates` の以下ジョブがグリーンであること
  - `go-boundary-guard`
  - `cli-absent-rehearsal`
- `tools/ci/check_tooling_boundaries.sh` が成功すること
- `tools/ci/check_repo_layout.sh` が成功すること
- `cli/go.mod` / `tools/artifactctl/go.mod` に `pkg/* v0.0.0` が残っていないこと

## 分離前チェック（この repo）
```bash
./tools/ci/check_tooling_boundaries.sh
./tools/ci/check_repo_layout.sh
GOWORK="$(pwd)/go.work.cli" go -C cli test ./... -run '^$'
go -C tools/artifactctl test ./... -run '^$'
```

期待結果:
- すべて exit code 0
- `boundary checks passed`
- `layout-check OK`

## 分離実行（高レベル）
1. `cli/` を新リポジトリへ移行する
2. 旧リポジトリ（この repo）では `cli/` と `go.work.cli` を除去する
3. 新リポジトリ側で `pkg/*` 依存バージョンを固定する（replace 前提にしない）

## 分離後チェック（この repo, CLI なし）
```bash
./tools/ci/check_tooling_boundaries.sh
CLI_ABSENT_MODE=1 ./tools/ci/check_repo_layout.sh
GOWORK=off go -C tools/artifactctl test ./... -run '^$'
GOWORK=off go -C pkg/artifactcore test ./... -run '^$'
GOWORK=off go -C pkg/composeprovision test ./... -run '^$'
GOWORK=off go -C pkg/deployops test ./... -run '^$'
GOWORK=off go -C pkg/runtimeimage test ./... -run '^$'
GOWORK=off go -C pkg/yamlshape test ./... -run '^$'
```

期待結果:
- すべて exit code 0
- `cli-absent-rehearsal` CI ジョブと同等の検証がローカルで再現できる

## 運用確認（この repo）
```bash
go build -o /tmp/artifactctl-local ./tools/artifactctl/cmd/artifactctl
ARTIFACTCTL_BIN=/tmp/artifactctl-local uv run e2e/run_tests.py --parallel --verbose --cleanup
```

期待結果:
- E2E が全マトリクスで成功
- `artifactctl` 適用フローが CLI 非依存で成立

## ロールバック方針
- 分離後チェックのいずれかが失敗した場合は、`cli/` 削除コミットを revert し、失敗した検証項目を修正して再実施する
- 境界ガードを緩める対応は禁止（恒久修正のみ）

---

## Implementation references
- `.github/workflows/quality-gates.yml`
- `tools/ci/check_tooling_boundaries.sh`
- `tools/ci/check_repo_layout.sh`
- `go.work`
