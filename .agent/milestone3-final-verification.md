# Milestone 3 Detail Plan: Final Verification and Cleanup

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md` and the parent plan `.agent/execplan-contract-boundary-deep-audit-master.md`.

## Purpose / Big Picture

Milestone 2 で実装した境界修正を最終確定し、契約・実装・テスト証跡を一致させます。

完了時点で以下を満たします。

- 境界ガード (`check_tooling_boundaries`) が pass。
- 主要 UT が pass。
- フル E2E が pass（実行不可時は理由と不足証跡を明示）。
- ドキュメントが実装構成（`pkg/deployops` + `pkg/artifactcore`）に一致。
- 残コード/残参照の取りこぼしがない。

## Progress

- [x] (2026-02-20 08:14Z) Milestone 3 詳細計画を作成。
- [x] (2026-02-20 08:32Z) 残参照・残コードを棚卸しし、`tools/artifactctl/pkg/deployops` 旧パス参照の再流入がないことを確認。
- [x] (2026-02-20 08:32Z) 境界チェック実行（`./tools/ci/check_tooling_boundaries.sh` pass）。
- [x] (2026-02-20 08:32Z) 主要 Go UT 実行（`cli` / `tools/artifactctl` / `pkg/artifactcore` / `pkg/deployops` / `pkg/composeprovision` / `pkg/yamlshape` / `services/agent` pass）。
- [x] (2026-02-20 08:32Z) フル E2E 実行（`uv run e2e/run_tests.py --parallel --verbose` pass）。
- [x] (2026-02-20 08:32Z) セルフレビュー実施（計画逸脱なし、GO 判定）。

## Surprises & Discoveries

- Observation: `artifactctl provision` の `--no-deps` 経路で、`docker compose run --no-deps` が BuildKit secret を欠落させ、`secret root_ca: not found` で失敗するケースが再現した。
  Evidence: `e2e-containerd` の deploy で `failed to solve: secret root_ca: not found` を確認し、同一コマンドを手動再現して一致。

- Observation: 同じ compose 定義でも `docker compose build python-base` は成功し、`run --no-deps provisioner` のみ失敗する。
  Evidence: 手動実行で `build python-base` は pass、`run --no-deps provisioner` は fail、`run provisioner`（depsあり）は pass。

## Decision Log

- Decision: フル E2E は契約境界修正の最終ゲートとして必須実行する。
  Rationale: apply 経路の共通化は統合動作への影響が大きく、UT のみでは不足するため。
  Date/Author: 2026-02-20 / Codex

- Decision: `--no-deps` の既定は維持しつつ、`composeprovision.Execute` で `NoDeps=true` の場合は `build provisioner` を先行実行する。
  Rationale: 既存 UX（依存サービスを起動しない）を壊さず、BuildKit secret 欠落による遅延失敗を防ぐため。
  Date/Author: 2026-02-20 / Codex

## Outcomes & Retrospective

Milestone 3 は GO。契約・実装・検証の整合は最終的に以下で成立した。

- `composeprovision` に `NoDeps` prebuild を追加し、`root_ca` secret 欠落の再現不具合を解消。
- 関連UT（`pkg/composeprovision` / `cli/internal/infra/deploy`）の期待値を更新し、回帰なしを確認。
- Go UT を対象全モジュールで pass。
- 境界チェックを pass。
- フル E2E（docker + containerd）をクリーン状態から再実行して pass。

判定: GO（High/Medium 未解消なし）。

## Context and Orientation

対象:

- `pkg/deployops`
- `pkg/artifactcore`
- `cli/internal/command`
- `cli/internal/usecase/deploy`
- `tools/artifactctl/cmd/artifactctl`
- `docs/deploy-artifact-contract.md`
- `docs/artifact-operations.md`

## Plan of Work

1. 残参照・残コード棚卸し
   - `tools/artifactctl/pkg/deployops` 旧パス参照残りの確認
   - docs/.agent の旧責務記述残りを確認

2. 検証実行
   - `./tools/ci/check_tooling_boundaries.sh`
   - Go UT（上記対象）
   - フル E2E（`uv run e2e/run_tests.py --parallel --verbose`）

3. セルフレビュー
   - 契約・実装・テスト証跡が一致しているかを GO/NO-GO 判定
   - master plan に進捗反映

## Validation and Acceptance

受け入れ条件:

- 境界チェック pass
- 主要 UT pass
- フル E2E pass
- ドキュメント整合が取れている
- High/Medium の未解消指摘がない

## Idempotence and Recovery

検証は再実行可能。失敗時は失敗点をこのファイルへ追記し、修正→再検証を繰り返す。

## Artifacts and Notes

Milestone 3 完了時に残す証跡:

- チェック/テスト実行ログ要約
- GO/NO-GO 判定
- master plan 進捗更新
