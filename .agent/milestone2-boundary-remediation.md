# Milestone 2 Detail Plan: Boundary Remediation

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md` and the parent plan `.agent/execplan-contract-boundary-deep-audit-master.md`.

## Purpose / Big Picture

Milestone 1 で確定した High/Medium 指摘を解消し、契約と実装を一致させます。

このマイルストーン完了後は、`esb artifact apply` と `artifactctl deploy` が同じ適用責務モデルで動作し、runtime 互換性判定と適用オーケストレーションのドリフト余地を減らします。

## Progress

- [x] (2026-02-20 08:35Z) Milestone 2 詳細計画を作成。
- [x] (2026-02-20 08:55Z) 修正方針を確定（shared apply orchestrator の owner を `pkg/deployops` に固定）。
- [x] (2026-02-20 09:02Z) High 指摘解消（`esb artifact apply` を `deployops.Execute` 経路へ統一）。
- [x] (2026-02-20 09:06Z) Medium 指摘解消（runtime 観測/判定ロジック重複の集約）。
- [x] (2026-02-20 09:09Z) ドキュメント同期（契約書・運用ガイド更新）。
- [x] (2026-02-20 09:11Z) セルフレビュー（計画逸脱チェック、GO判定）。

## Surprises & Discoveries

- Observation: High 指摘は「実装修正」か「契約文言修正」どちらでも解消できるが、後者のみだとドリフトリスクを残す。
  Evidence: `cli/internal/command/artifact.go` と `tools/artifactctl/pkg/deployops/execute.go` が責務的に分岐している。

- Observation: このリポジトリの `go.work` は「主要モジュール + replace」運用を前提にしており、`pkg/*` を全面 `use` へ切り替えると依存解決が崩れる。
  Evidence: `go list -m all` / `go test` で `pkg/artifactcore@v0.0.0` の remote fetch が発生。

## Decision Log

- Decision: High 指摘は契約文言の後退ではなく、実装の整合化で解消する。
  Rationale: 契約を実装へ合わせて弱めるより、単一路線の適用経路へ統合した方が保守コストと将来ドリフトを抑えられるため。
  Date/Author: 2026-02-20 / Codex

- Decision: `cli` から `tools/artifactctl` への直接依存は導入しない。
  Rationale: 将来のリポジトリ分離時に依存境界を再び壊すため。共有ロジックは `pkg/*` に集約する。
  Date/Author: 2026-02-20 / Codex

- Decision: deploy orchestration は `tools/artifactctl/pkg/deployops` から `pkg/deployops` へ移送し、`cli`/`artifactctl` の共通 adapter 入口にする。
  Rationale: apply 経路を1つに固定し、契約ドリフト余地をなくすため。
  Date/Author: 2026-02-20 / Codex

- Decision: `go.work` は既存方針（主要 module `use` + `pkg/*` replace）を維持し、`pkg/deployops` 単体検証は `GOWORK=off` + module local replace で担保する。
  Rationale: 開発運用・CIの互換を崩さずに新モジュールを導入するため。
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 2 の修正は完了。

実装結果:

- shared apply orchestration を `pkg/deployops` へ移送。
- `esb artifact apply` を `deployops.Execute` 呼び出しへ変更し、`artifactctl deploy` と apply 経路を統一。
- runtime 観測の優先サービス選択を `artifactcore.PreferredRuntimeServiceImage` へ集約。
- runtime_stack 要求判定を `artifactcore.HasRuntimeStackRequirements` へ集約。
- 境界チェック allowlist を更新（新規公開 API を明示管理）。
- 運用ドキュメントを実装構成（`pkg/deployops` + `pkg/artifactcore`）へ同期。

Milestone 2 の成功条件:

- High 指摘が消え、契約記述と実装経路が一致している。
- Medium 指摘の重複ロジックが単一 owner へ集約されている。
- 境界ガードと関連UTが通過する。

検証結果:

- `go -C cli test ./internal/command ./internal/usecase/deploy`: pass
- `go -C tools/artifactctl test ./...`: pass
- `GOWORK=off go -C pkg/artifactcore test ./...`: pass
- `GOWORK=off go -C pkg/deployops test ./...`: pass
- `./tools/ci/check_tooling_boundaries.sh`: pass

セルフレビュー判定: GO（Milestone 2 完了）。

## Context and Orientation

対象領域:

- `cli/internal/command/artifact.go`
- `cli/internal/usecase/deploy/runtime_observation.go`
- `pkg/deployops/execute.go`
- `pkg/deployops/runtime_probe.go`
- `pkg/artifactcore/*`
- `docs/deploy-artifact-contract.md`
- `docs/artifact-operations.md`

解消対象:

- High: `esb artifact apply` と `artifactctl deploy` の責務不一致
- Medium: runtime 観測ロジック重複
- Medium: runtime_stack 要求有無判定ロジック重複

## Plan of Work

1. 単一適用モデルの owner を固定

`apply correctness` と `runtime compatibility` の最終判定は `pkg/artifactcore` に維持しつつ、
`artifactctl deploy` と `esb artifact apply` が共通オーケストレーションを使うように整理します。

実装方針:

- `tools/artifactctl/pkg/deployops` の deploy orchestration を `pkg` 配下の共有パッケージへ移送する。
- `cli` と `artifactctl` はその共有 orchestrator を呼ぶ adapter になる。
- `cli -> tools/artifactctl` の直接依存は作らない。

2. High 指摘を実装で解消

- `esb artifact apply` を「core直呼びのみ」から、共有 orchestrator 経由へ変更。
- `artifactctl deploy` は同 orchestrator の adapter 呼び出しに揃える。
- これにより契約文言（同等実装）と実装を一致させる。

3. Medium 指摘を解消

- runtime 観測で共通化可能なロジック（優先サービス選択、image tag抽出、mode推定）を単一 owner へ集約。
- `runtime_stack` 有効判定の重複 (`hasRuntimeStack` 系) を単一関数に集約。

4. ドキュメント同期

- 実装後の ownership map を `docs/artifact-operations.md` と `docs/deploy-artifact-contract.md` に反映。
- 「どこが正本実装か」を一文で明示し、二重解釈を排除。

## Concrete Steps

作業ディレクトリ: `/home/akira/esb`

1. 共有 orchestrator 抽出

- `pkg/deployops/execute.go` と `prepare_images.go` の責務を shared module として維持。
- `tools/artifactctl` は command adapter のみに整理。

2. CLI apply 経路統一

- `cli/internal/command/artifact.go` の `runArtifactApply` を共有 orchestrator 呼び出しへ置換。

3. 重複ロジック削減

- `cli/internal/usecase/deploy/runtime_observation.go`
- `pkg/deployops/runtime_probe.go`
- `pkg/artifactcore/runtime_compat_validation.go`

4. 検証

- `./tools/ci/check_tooling_boundaries.sh`
- `go test ./pkg/artifactcore/...`
- `go test ./tools/artifactctl/...`
- `go test ./cli/internal/command ./cli/internal/usecase/deploy`

5. セルフレビュー

- 指摘が消えているかを High/Medium/Low で再評価。
- GO/NO-GO 判定を本書と master に反映。

## Validation and Acceptance

受け入れ条件:

- `esb artifact apply` と `artifactctl deploy` の責務差分が解消。
- runtime 観測/判定ロジックの重複が解消。
- 契約書と運用ガイドが実装に一致。
- 境界チェックと関連UTが pass。

## Idempotence and Recovery

この改修は段階的に実施します。

- 共有 orchestrator 抽出
- 呼び出し側差し替え
- 重複削減

の順でコミットし、各段階でテストを通過させます。失敗時は直近段階までロールバック可能です。

## Artifacts and Notes

Milestone 2 完了時の成果物:

- 共有 orchestrator 実装（`pkg/*`）
- `cli` / `artifactctl` adapter 更新
- 契約ドキュメント更新
- 検証ログ（boundary check + 主要UT）

## Interfaces and Dependencies

最終的な依存ルール:

- `cli` と `tools/artifactctl` は `pkg` 共有実装に依存する。
- `cli` は `tools/artifactctl` に依存しない。
- `pkg/artifactcore` は `cli` / `tools/artifactctl` に依存しない。

Revision note (2026-02-20 08:35Z): Milestone 2 remediation plan created from Milestone 1 NO-GO findings.
Revision note (2026-02-20 09:11Z): Milestone 2 implementation completed with GO verdict after boundary checks and related UT pass.
