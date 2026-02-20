# Milestone 1 Detail Plan: Boundary Deep Audit

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md` and the parent plan `.agent/execplan-contract-boundary-deep-audit-master.md`.

## Purpose / Big Picture

このマイルストーンの目的は、`pkg/artifactcore`・`tools/artifactctl`・`cli` の責務境界を「主張」ではなく「コード実測」で固定することです。

完了時点で、以下が明確になります。

- どの経路が契約上の正本挙動を担うか
- どの関数/引数が重複または越境しているか
- Milestone 2 で必ず修正すべき対象（優先度付き）

## Progress

- [x] (2026-02-20 08:15Z) Milestone 1 詳細計画を作成。
- [x] (2026-02-20 08:22Z) 呼び出し経路マップを作成（`cli -> artifactcore`, `artifactctl -> artifactcore`）。
- [x] (2026-02-20 08:23Z) runtime 互換性判定導線の整合性監査を実施。
- [x] (2026-02-20 08:24Z) API公開面と責務所有者マップを作成。
- [x] (2026-02-20 08:25Z) 指摘一覧（High/Medium/Low）を確定。
- [x] (2026-02-20 08:26Z) セルフレビューを実施し、Milestone 1 は NO-GO 判定で確定。

## Surprises & Discoveries

- Observation: ドキュメント上は `esb artifact apply` が `artifactctl deploy` と同等責務だが、実装経路は一致していない可能性が高い。
  Evidence: `cli/internal/command/artifact.go` は core 直呼びで、`tools/artifactctl/pkg/deployops/execute.go` 側の preflight 層を通らない。

- Observation: runtime 観測ロジックは共有化を進めたが、優先サービス選択などに still-duplicate が残る可能性がある。
  Evidence: `cli/internal/usecase/deploy/runtime_observation.go` と `tools/artifactctl/pkg/deployops/runtime_probe.go`。

- Observation: 境界チェックは pass しているが、責務差分は検知できない。
  Evidence: `./tools/ci/check_tooling_boundaries.sh` は pass。スクリプトは依存方向/API面を検査し、適用経路の意味差分までは検知しない。

## Decision Log

- Decision: Milestone 1 ではコード変更を最小化し、まず観測結果の固定を優先する。
  Rationale: 境界修正を先に始めると、評価基準が揺れて再調査が発生しやすいため。
  Date/Author: 2026-02-20 / Codex

- Decision: 評価は「契約適合性」と「保守性（将来ドリフト耐性）」を分けて記録する。
  Rationale: 契約適合していても重複実装が残る場合は Medium リスクとして残す必要があるため。
  Date/Author: 2026-02-20 / Codex

- Decision: Milestone 1 は NO-GO とし、Milestone 2 で境界修正を必須化する。
  Rationale: High 指摘（適用経路責務の不一致）が残っており、そのまま次工程へ進むと契約と実装の乖離を固定化するため。
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 1 の監査は完了し、判定は NO-GO。

確定した責務マップ（要点）は次の通りです。

- `artifactctl deploy` 経路:
  - `tools/artifactctl/pkg/deployops/execute.go` が runtime probe と image prepare を含む apply orchestration を実行し、最後に `artifactcore.ExecuteApply` を呼ぶ。
- `esb deploy` の apply phase:
  - `cli/internal/usecase/deploy/deploy_runtime_provision.go` で runtime observation を取得して `artifactcore.ExecuteApply` を実行し、provision へ進む。
- `esb artifact apply`:
  - `cli/internal/command/artifact.go` で `artifactcore.ExecuteApply` を直接呼ぶだけで、runtime probe/image prepare を実行しない。

確定した指摘は次の通りです。

- High:
  - 契約文言（`esb artifact apply` は `artifactctl deploy` と同等実装）と実装が不一致。
    - 契約: `docs/deploy-artifact-contract.md` の `esb artifact apply` 責務記述。
    - 実装: `cli/internal/command/artifact.go`, `tools/artifactctl/pkg/deployops/execute.go`。
- Medium:
  - runtime 観測の優先サービス選択ロジックが `cli` / `artifactctl` に重複。
    - `cli/internal/usecase/deploy/runtime_observation.go`
    - `tools/artifactctl/pkg/deployops/runtime_probe.go`
  - `runtime_stack` 有効判定ロジックが重複。
    - `tools/artifactctl/pkg/deployops/execute.go:hasRuntimeStack`
    - `pkg/artifactcore/runtime_compat_validation.go:hasRuntimeStackRequirements`
- Low:
  - `pkg/artifactcore` の公開API面は用途が拡張されているため、公開面積管理ポリシーの明文化が必要。

Milestone 2 での必須修正入力は次の2点です。

- `esb artifact apply` の適用経路を契約上の単一路線へ統合（実装統合または契約文言修正のいずれかを明示選択）。
- runtime 観測/判定補助ロジックの重複を一箇所へ集約。

## Context and Orientation

監査対象の責務定義:

- `pkg/artifactcore`
  - manifest schema、payload整合性検証、runtime stack compatibility判定、runtime-config merge の共通コア。
- `tools/artifactctl`
  - artifact-first deploy の command adapter + apply orchestration。
- `cli`
  - producer/composite 導線と UX。

監査基準:

- 契約適合: `docs/deploy-artifact-contract.md` の記述と実装挙動が一致しているか。
- 境界適合: core と adapter/orchestration の役割が混線していないか。
- 重複適合: 同一責務ロジックが複数箇所で独立実装されていないか。

## Plan of Work

1. 経路監査

`cli` と `artifactctl` から `artifactcore` への呼び出しを列挙し、

- apply path
- runtime observation path
- manifest read/write/id sync path

の3系統で責務をマッピングします。

2. 契約照合

`docs/deploy-artifact-contract.md` と `docs/artifact-operations.md` の要件をチェックリスト化し、実装への対応有無を1つずつ確認します。

3. API面監査

`pkg/artifactcore` の公開APIを棚卸しし、

- adapter層だけが使う補助関数
- truly shared core contract

を分離して、公開面積の是正候補を作成します。

4. 指摘確定

指摘を High/Medium/Low に分類し、各指摘に対して

- どのファイルを修正すべきか
- 何を削る/寄せるべきか

を明示します。

## Concrete Steps

作業ディレクトリ: `/home/akira/esb`

1. 呼び出し元抽出

- `rg -n "ExecuteApply|ReadArtifactManifest|WriteArtifactManifest|RuntimeObservation|runtime_stack" cli tools/artifactctl pkg/artifactcore -S`

2. 契約照合ポイント抽出

- `rg -n "正本|artifactctl deploy|esb artifact apply|runtime-base|runtime stack|責務" docs/deploy-artifact-contract.md docs/artifact-operations.md -S`

3. 公開API面抽出

- `./tools/ci/check_tooling_boundaries.sh`
- `tools/ci/artifactcore_exports_allowlist.txt` と差分を確認

4. 監査結果をこのファイルに追記し、GO/NO-GO判定を記録

## Validation and Acceptance

Milestone 1 は以下を満たしたら完了です。

- 3領域の責務マップがファイル単位で作成済み。 (done)
- 契約との差分指摘が優先度付きで確定。 (done)
- Milestone 2 実装対象が曖昧さなく定義済み。 (done)
- セルフレビューで GO/NO-GO 判定が記録済み。 (done / NO-GO)

## Idempotence and Recovery

この監査は再実行可能です。再実行時は、

- 呼び出し抽出結果
- 契約照合結果
- 指摘優先度

の3点を上書き更新し、古い結論を残さない運用とします。

## Artifacts and Notes

Milestone 1 の成果物:

- `.agent/milestone1-boundary-deep-audit.md`（本書）
- 責務マップ（本書内に記載）
- 指摘一覧（High/Medium/Low）

## Interfaces and Dependencies

Milestone 1 ではインターフェース実装変更は行わず、境界評価のみを行います。

Milestone 2 へ渡す設計制約:

- `core <- cli` 逆依存禁止
- apply correctness は `pkg/artifactcore` に集約
- adapter層は runtime 観測・I/O・オーケストレーションに限定

Revision note (2026-02-20 08:15Z): Initial detailed plan created for Milestone 1 boundary deep audit.
Revision note (2026-02-20 08:26Z): Completed boundary audit and self-review with NO-GO verdict due to high-severity contract/implementation drift.
