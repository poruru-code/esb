# Contract Boundary Deep Audit Master Plan

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

今回の目的は、契約変更後の責務境界が「実装として本当に成立しているか」を、`pkg/artifactcore`、`tools/artifactctl`、`cli` の3領域をまたいで再検証し、ズレを解消することです。

完了後は、適用系の責務が単一方針で説明・実装・テストの全てで一致し、利用者がどのコマンド経路を使っても同じ契約ルール（互換性判定、入力検証、適用挙動）で動作する状態にします。

## Progress

- [x] (2026-02-20 08:00Z) 完了済み旧プラン (`.agent/execplan-artifact-contract-realignment-master.md` および milestone1-6) を削除。
- [x] (2026-02-20 08:00Z) 本マスタープランを新規作成。
- [x] (2026-02-20 08:15Z) Milestone 1 詳細計画を作成（`.agent/milestone1-boundary-deep-audit.md`）。
- [x] (2026-02-20 08:26Z) Milestone 1 を実施（3領域の呼び出し経路・責務・契約差分を実測で棚卸し）。
- [x] (2026-02-20 08:26Z) Milestone 1 セルフレビューを実施し、NO-GO判定（High指摘あり）を確定。
- [x] (2026-02-20 08:35Z) Milestone 2 詳細計画を作成（`.agent/milestone2-boundary-remediation.md`）。
- [x] (2026-02-20 09:11Z) Milestone 2 実装（境界修正、重複排除、インターフェース整理）。
- [x] (2026-02-20 09:11Z) Milestone 2 レビュー（設計整合性の再評価）。
- [x] (2026-02-20 08:14Z) Milestone 3 詳細計画を作成（検証とクリーンアップ）。
- [x] (2026-02-20 08:32Z) Milestone 3 実施（UT/E2E、境界チェック、残コード確認）。
- [x] (2026-02-20 08:32Z) Final Review（GO 判定、残リスクの明確化）。

## Surprises & Discoveries

- Observation: 直近レビューで `esb artifact apply` と `artifactctl deploy` に適用経路の責務差分が確認された。
  Evidence: `cli/internal/command/artifact.go` は `artifactcore.ExecuteApply` 直呼び、`tools/artifactctl/pkg/deployops/execute.go` は runtime probe + image prepare を含む。

- Observation: runtime 観測の優先サービス選択ロジックが `cli` と `tools/artifactctl` に重複している。
  Evidence: `cli/internal/usecase/deploy/runtime_observation.go` と `tools/artifactctl/pkg/deployops/runtime_probe.go` の独立実装。

## Decision Log

- Decision: まず再設計ではなく、深掘り調査を先行する。
  Rationale: 境界問題は「契約」「実装」「運用導線」の3層で発生するため、先に現行挙動の実測差分を固定しないと修正が再ドリフトする。
  Date/Author: 2026-02-20 / Codex

- Decision: 完了済み旧プランは削除し、本件専用マスタープランへ一本化する。
  Rationale: 履歴混在を防ぎ、現在の論点（責務分離再監査）だけを追跡可能にする。
  Date/Author: 2026-02-20 / Codex

- Decision: Milestone 1 は NO-GO とし、Milestone 2 で High 指摘を先に解消する。
  Rationale: 契約文言と実装経路の不一致が残っている状態で次工程へ進むと、境界逸脱を固定化するため。
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

現時点では Milestone 2（境界修正）まで完了しています。

到達目標は次の通りです。

- `pkg/artifactcore` は「契約と共通適用ロジック」のみを保持する。
- `tools/artifactctl` は「適用オーケストレーション」の正本経路となる。
- `cli` は「生成と複合導線」に集中し、適用経路は契約上の単一方針に一致する。

Milestone 1 の結論:

- NO-GO（High 指摘あり）
- High 指摘:
  - `esb artifact apply` と `artifactctl deploy` の責務不一致（契約記述と実装差分）
- Medium 指摘:
  - runtime 観測ロジック重複
  - `runtime_stack` 判定ロジック重複

Milestone 2 の結論:

- GO（High/Medium 指摘を解消）
- `pkg/deployops` を shared orchestrator owner として導入
- `esb artifact apply` / `artifactctl deploy` の apply 経路を統一

Milestone 3 の結論:

- GO（検証系を完走）
- `composeprovision` の `NoDeps` 経路に prebuild を追加し、`secret root_ca: not found` を解消
- `check_tooling_boundaries` pass
- Go UT（`cli` / `tools/artifactctl` / `pkg/*` / `services/agent`）pass
- フル E2E（`e2e-docker` 53件、`e2e-containerd` 45件）pass

本マスタープランは完了。

## Context and Orientation

評価対象は以下の3領域です。

- `pkg/artifactcore`: manifest schema、入力検証、runtime stack compatibility 判定、設定適用 merge の共通コア。
- `tools/artifactctl`: artifact-first deploy の command adapter と deploy orchestration。
- `cli`: template 生成と複合 deploy 導線、および artifact apply アダプタ。

本計画での「責務分離」は次の意味で使います。

- Core responsibility: コマンドUIやDocker実行詳細に依存しない、契約そのものの判定・適用。
- Adapter responsibility: 引数解釈、I/O、実行環境からの観測値収集、core呼び出し。
- Orchestration responsibility: build/apply/provision の順序制御。

## Plan of Work

Milestone 1 では、責務境界をコードベースで実測し、呼び出し経路を固定します。調査結果は「どこが契約の正本か」「どこが重複か」「どこが越境か」をファイル単位で列挙し、優先度をつけます。

Milestone 2 では、Milestone 1 の指摘を実装で解消します。特に適用経路の二重実装、runtime 観測の重複、公開API過多の是正方針を決定し、不要分岐・不要引数・不要公開関数を削減します。

Milestone 3 では、UT/E2Eとドキュメントを同期し、境界ガード（`tools/ci/check_tooling_boundaries.sh`）を含む最終検証を行います。必要に応じてE2E artifactを再生成し、契約差分がないことを確認します。

## Concrete Steps

作業ディレクトリは `/home/akira/esb` を前提とします。

1. Milestone 1 詳細計画を作成
   - `.agent/milestone1-boundary-deep-audit.md` を作成し、調査観点・対象ファイル・判定基準を固定。

2. 呼び出し経路の実測
   - `rg` とコード読解で `cli -> artifactcore`, `artifactctl -> artifactcore` の呼び出しを棚卸し。
   - runtime observation と apply path の重複実装を抽出。

3. 改修設計レビュー
   - 指摘を High/Medium/Low に分類。
   - 修正前に GO/NO-GO 判定を記録。

4. 実装と検証
   - 合意した設計に沿って境界整理を実装。
   - `./tools/ci/check_tooling_boundaries.sh`
   - 影響UT（`pkg/artifactcore`, `tools/artifactctl`, `cli`）
   - 必要時フル E2E

## Validation and Acceptance

受け入れ条件は次の通りです。

- 高優先度の境界違反（責務不一致、契約挙動差）がゼロ。
- `pkg/artifactcore` への依存方向が維持され、`core <- cli` 逆依存がない。
- runtime compatibility 判定経路が契約説明と実装で一致する。
- `tools/ci/check_tooling_boundaries.sh` が pass。
- 変更範囲に応じた UT/E2E が pass。

## Idempotence and Recovery

この計画は段階的に再実行可能です。各 milestone の完了時点で、

- 進捗更新
- 判定ログ更新
- 失敗時のロールバック方針（直近コミット単位）

を必ず記録し、次回再開時に単独で再現可能な状態を保ちます。

## Artifacts and Notes

この計画で作成予定の主要成果物は以下です。

- `.agent/milestone1-boundary-deep-audit.md`
- `.agent/milestone2-boundary-remediation.md`
- `.agent/milestone3-final-verification.md`

実装後は、該当PRに以下の証跡を必須添付します。

- 境界チェック実行結果
- 主要UT実行結果
- 必要時E2E実行結果
- 責務境界の最終マップ（簡潔な文章）

## Interfaces and Dependencies

最終状態で守るべき依存規約は次の通りです。

- `pkg/artifactcore` は `cli` / `tools/artifactctl` に依存しない。
- `tools/artifactctl` と `cli` は `pkg/artifactcore` を共有利用する。
- `tools/artifactctl` は applyオーケストレーションの正本経路を提供する。
- `cli` は producer/composite 導線を担当し、適用責務は契約上の単一方針に一致させる。
- 将来の境界逸脱を防ぐため、公開API面は `tools/ci/artifactcore_exports_allowlist.txt` で管理する。

Revision note (2026-02-20 08:00Z): Completed-plan cleanup and new deep-audit master plan initialization.
Revision note (2026-02-20 08:26Z): Milestone 1 audit completed with NO-GO verdict; High/Medium findings recorded for Milestone 2 remediation planning.
Revision note (2026-02-20 09:11Z): Milestone 2 remediation completed with GO verdict; shared apply orchestrator moved to `pkg/deployops`.
Revision note (2026-02-20 08:32Z): Milestone 3 verification completed with GO verdict; NoDeps prebuild added to stabilize provision path and full UT/E2E passed.
