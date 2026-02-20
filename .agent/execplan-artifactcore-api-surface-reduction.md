# Artifactcore API Surface Reduction

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

`pkg/artifactcore` の公開APIが増え続けると、利用側変更のたびに allowlist と互換検討が必要になり、メンテナンスコストが上がる。  
今回の目的は、契約コアに不要な公開API（runtime image 観測ヘルパ）を `artifactcore` から分離し、公開面を縮小して境界を明確化すること。

完了後は以下を満たす。

- `artifactcore` 公開API件数を削減。
- runtime image 観測ヘルパは `artifactcore` 外の共有パッケージへ移管。
- `cli` / `pkg/deployops` の呼び出しは新共有パッケージを利用。
- 境界チェック、関連UTが pass。

## Progress

- [x] (2026-02-20 08:36Z) ExecPlan を作成。
- [x] (2026-02-20 08:41Z) `artifactcore` から縮小対象APIを特定し、移管先を `pkg/runtimeimage` に確定。
- [x] (2026-02-20 08:43Z) `pkg/runtimeimage` 実装と既存呼び出し切り替え（`cli` / `pkg/deployops`）。
- [x] (2026-02-20 08:44Z) `artifactcore` の不要export削除と allowlist 更新。
- [x] (2026-02-20 08:45Z) 境界チェック・関連UT実行・セルフレビュー完了（GO）。
- [x] (2026-02-20 08:48Z) 厳密化追補として `ErrRuntimeBaseDockerfileMissing` / `FileSHA256` を `artifactcore` から削除し、CLI側へ移設。

## Surprises & Discoveries

- Observation: `runtime image` 推論ロジックは `cli` と `pkg/deployops` の双方から参照されるが、契約コア (`artifactcore`) には本質的に不要だった。
  Evidence: 呼び出しは `cli/internal/usecase/deploy/runtime_observation.go` と `pkg/deployops/runtime_probe.go` のみ。

- Observation: `HasRuntimeStackRequirements` は export である必要がなく、`pkg/deployops` 側ローカル関数で十分だった。
  Evidence: `artifactcore` 外の利用は `pkg/deployops/execute.go` のみ。

- Observation: `ErrRuntimeBaseDockerfileMissing` は参照ゼロの未使用exportで、`artifactcore` APIを不要に汚染していた。
  Evidence: `rg` 結果で定義とallowlist以外の参照なし。

## Decision Log

- Decision: runtime image 観測ヘルパは `artifactcore` ではなく独立共有パッケージへ移す。
  Rationale: これらは manifest 契約そのものではなく、観測/推論ロジックでありコア契約責務ではないため。
  Date/Author: 2026-02-20 / Codex

- Decision: 新規共有は `pkg/runtimeimage` モジュールとして作成する。
  Rationale: `cli` と `pkg/deployops` の両方から共通利用しつつ、`artifactcore` の公開面を増やさないため。
  Date/Author: 2026-02-20 / Codex

- Decision: `FileSHA256` は `artifactcore` から削除し、producer側（CLI command）ローカル実装へ戻す。
  Rationale: template hash算出は manifest生成工程の都合であり、契約コア責務ではないため。
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

目的は達成。`artifactcore` の公開APIから以下5件を削除した。

- `HasRuntimeStackRequirements`
- `InferRuntimeModeFromImageRefs`
- `InferRuntimeModeFromServiceImages`
- `ParseRuntimeImageTag`
- `PreferredRuntimeServiceImage`

追加で以下2件も削除した。

- `ErrRuntimeBaseDockerfileMissing`（未使用export）
- `FileSHA256`（producer都合ヘルパ）

上記は `pkg/runtimeimage` へ移管し、`cli` / `pkg/deployops` 呼び出しを切り替えた。  
`check_tooling_boundaries` と関連UTはすべて pass。

結果として、契約コア (`artifactcore`) と観測推論ロジックの責務分離を一段進め、公開API面積を削減できた。

## Context and Orientation

現状、以下の公開ヘルパが `artifactcore` に存在し、`cli` と `pkg/deployops` が参照している。

- `InferRuntimeModeFromServiceImages`
- `InferRuntimeModeFromImageRefs`
- `ParseRuntimeImageTag`
- `PreferredRuntimeServiceImage`
- `HasRuntimeStackRequirements`

前4件は runtime image 文字列から mode/tag を推定する観測ロジック。  
最後の1件は `runtime_stack` 要件判定の薄いヘルパで、コア外からの依存は限定的。

## Plan of Work

1. runtime image 観測ヘルパを新規共有パッケージへ移設。
2. `cli/internal/usecase/deploy/runtime_observation.go` と `pkg/deployops/runtime_probe.go` を新パッケージ参照へ変更。
3. `artifactcore` から該当exportを削除。
4. `HasRuntimeStackRequirements` は呼び出し側ローカル判定に置換し export を削除。
5. `tools/ci/artifactcore_exports_allowlist.txt` を縮小後APIに更新。
6. 境界チェックと関連UTを実行し、結果を記録。

## Concrete Steps

作業ディレクトリ: `/home/akira/esb`

実装後に実行する検証コマンド:

    go -C cli test ./internal/usecase/deploy ./internal/infra/deploy
    go -C tools/artifactctl test ./...
    GOWORK=off go -C pkg/artifactcore test ./...
    GOWORK=off go -C pkg/deployops test ./...
    GOWORK=off go -C pkg/runtimeimage test ./...
    ./tools/ci/check_tooling_boundaries.sh

## Validation and Acceptance

- `artifactcore` 公開APIから観測ヘルパが除去されていること。
- `cli` / `pkg/deployops` が新共有パッケージで同等挙動を維持すること。
- 境界チェックと関連UTが pass すること。

## Idempotence and Recovery

移行は関数置換中心で再実行可能。失敗時は import と呼び出しを戻せばビルド状態へ復帰できる。  
allowlist 更新を伴うため、最終的に `check_tooling_boundaries` pass を必ず確認する。

## Artifacts and Notes

完了時に以下を記録する。

- 削減した `artifactcore` export の一覧
- テスト結果要約
- 追加した共有パッケージと責務説明

Revision note (2026-02-20 08:45Z): 実装・検証完了に伴い Progress/Discoveries/Decision/Outcome を更新。API削減実績と検証結果を反映。
Revision note (2026-02-20 08:48Z): 厳密化要件に合わせて追加の不要export 2件を削除し、責務分離を補強。
