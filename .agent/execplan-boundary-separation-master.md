# Boundary Separation Master Plan (CLI Split Safety)

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

This document must be maintained in accordance with `.agent/PLANS.md`.

## Purpose / Big Picture

この計画の目的は、CLI分離を進めても責務境界が崩れない構造を固定し、同じ問題が再発しない運用ルールと自動ガードを同時に導入することです。完了後は、`pkg` に入るコードが「本当に共通であるか」を機械的に判定でき、`cli` / `tools/artifactctl` / `e2e` のどこに実装すべきか迷わない状態になります。ユーザー視点では、`esb deploy` と `artifactctl deploy` の挙動は維持したまま、将来の分離作業で破綻しない変更だけがマージされるようになります。

## Progress

- [x] (2026-02-19 13:40Z) Master ExecPlan を作成し、境界ルールと再発防止の実装方針を定義した。
- [x] (2026-02-19 13:46Z) Milestone 1 の詳細計画（作業順序・成果物・完了判定）を追加し、即時実行可能な状態にした。
- [x] (2026-02-19 13:46Z) Milestone 1 を実行し、責務分類・移設対象・API縮小対象を `.agent/milestone1-boundary-baseline.md` に凍結した。
- [x] (2026-02-19 14:10Z) Milestone 2 を実装し、`ExecuteDeploy`/`DeployInput`/`prepare_images.go` を `pkg/artifactcore` から排出して `tools/artifactctl/pkg/deployops` へ移設した。
- [x] (2026-02-19 14:17Z) Milestone 3 を実装し、`composeprovision` を `pkg/composeprovision` へ分離して `cli`/`artifactctl` の import を更新した。
- [x] (2026-02-19 14:17Z) Milestone 4 を実装し、`check_tooling_boundaries.sh` に責務禁止ルールと `artifactcore` API allowlist チェックを追加した。
- [x] (2026-02-19 14:17Z) Milestone 5 のレビュー導線として `.github/pull_request_template.md` を追加し、境界影響と allowlist 更新理由の記載を必須化した。
- [ ] フルUT・フルE2E・Dockerクリーン環境で最終検証し、運用定着を確認する。

## Surprises & Discoveries

- Observation: `pkg/artifactcore` は契約ロジックだけでなく、`docker buildx/tag/push` 実行や `.dockerignore` 一時書換まで持っており、共通ライブラリ層としては責務過多になっている。
  Evidence: `pkg/artifactcore/execute.go`, `pkg/artifactcore/prepare_images.go`.

- Observation: `composeprovision` は artifact 固有ではないが `pkg/artifactcore/composeprovision` 配下にあるため、命名と責務が一致していない。
  Evidence: `pkg/artifactcore/composeprovision/composeprovision.go`, `cli/internal/infra/deploy/compose_provisioner.go`, `tools/artifactctl/cmd/artifactctl/main.go`.

- Observation: `prepare_images` を `tools/artifactctl/pkg/deployops` へ移しても、`cli`・`artifactctl`・`artifactcore` の既存UTは破綻しなかった。
  Evidence: `go -C cli test ./...`, `go -C tools/artifactctl test ./...`, `GOWORK=off go -C pkg/artifactcore test ./...` が全て pass。

- Observation: `go mod tidy` は workspace 依存前提のローカルモジュール（`pkg/artifactcore` / `pkg/composeprovision`）を remote 取得しようとして失敗する。
  Evidence: `go -C cli mod tidy`, `go -C tools/artifactctl mod tidy` が `repository not found` で失敗。`go test` は workspace 解決で pass。

## Decision Log

- Decision: この計画を単発の修正計画ではなく「マスター計画」とし、今後の境界修正サイクルの正本にする。
  Rationale: ラウンドごとに再レビューすると同種の指摘が再発しているため、ルールと自動ガードを先に固定する必要がある。
  Date/Author: 2026-02-19 / Codex

- Decision: 再発防止の中心は「運用ルール」ではなく「CIで落ちる機械ガード」に置く。
  Rationale: 目視レビューだけでは境界違反を防ぎきれないため。
  Date/Author: 2026-02-19 / Codex

- Decision: `pkg/artifactcore` から `ExecuteDeploy`/`DeployInput` と image prepare 実行責務を排出し、`ExecuteApply` 中心の契約層へ縮小する。
  Rationale: `ExecuteDeploy` と `prepare_images.go` はユースケース実行/運用オーケストレーションであり、共通契約層の責務ではないため。
  Date/Author: 2026-02-19 / Codex

- Decision: `pkg/artifactcore/composeprovision` は artifact 契約層から分離し、artifact 非依存の共有運用層へ再配置する。
  Rationale: `cli` と `artifactctl` の共通利用実体はあるが、`artifactcore` 配下だと責務境界を誤誘導するため。
  Date/Author: 2026-02-19 / Codex

- Decision: Milestone 2 の移設先を `tools/artifactctl/pkg/deployops` とし、deploy 順序制御（prepare + apply）を artifactctl 側責務へ固定する。
  Rationale: 現行機能を維持しつつ `pkg/artifactcore` から実行オーケストレーション責務を排出するため。
  Date/Author: 2026-02-19 / Codex

- Decision: `composeprovision` の移設先を `pkg/composeprovision` とし、`artifactcore` 依存を完全に外す。
  Rationale: `cli` と `artifactctl` が共有利用するが artifact 契約とは独立した運用責務であるため。
  Date/Author: 2026-02-19 / Codex

- Decision: 境界再発防止として `tools/ci/check_tooling_boundaries.sh` に pure-core 禁止ルール（`os/exec` と registry env 依存禁止）と `artifactcore` export allowlist を追加する。
  Rationale: import 方向だけでは責務過多を検知できないため。
  Date/Author: 2026-02-19 / Codex

- Decision: PR レビュー導線として `.github/pull_request_template.md` を新設し、境界影響・shared placement 理由・allowlist 更新理由の記載を必須化する。
  Rationale: CI ガードに加えて、レビュー時点で設計根拠を明示させるため。
  Date/Author: 2026-02-19 / Codex

## Outcomes & Retrospective

Milestone 1〜5 の実装項目（コード/CI/レビュー導線）を完了。`pkg/artifactcore` から deploy 実行責務を排出し、`composeprovision` を独立共有層へ移し、境界違反を CI と PR テンプレートの両方で検知できる体制へ更新した。残課題はフル E2E と Docker クリーン再現を含む最終検証のみ。

## Context and Orientation

このリポジトリは、現在 `cli`、`tools/artifactctl`、`e2e`、`pkg` にロジックが分散しています。分離方針では「ESB本体に必要な最小ロジックのみ残し、運用オプションの複雑性を隔離する」ことが必須です。現状はその途中段階で、`pkg/artifactcore` が「共通契約」と「実行オーケストレーション」を同時に持っているため、境界が曖昧です。

主要な対象は次のとおりです。`pkg/artifactcore` は artifact 契約（manifest, validate, merge, apply）を扱う共通層です。`tools/artifactctl` は非CLI経路の実行アダプタです。`cli` はUXと生成系フローを担い、`e2e` は外部バイナリとして `artifactctl` を利用します。境界是正では、これらの依存方向と責務を明文化し、違反を自動検知できる状態にします。

## Boundary Rules (Hard Contract)

本計画では、以下を強制ルールとします。例外は認めません。

`pkg/yamlshape` は最下層の純粋整形ライブラリです。YAML shape の変換・キー生成のみを扱い、プロセス実行、環境変数分岐、ネットワーク、Docker操作を禁止します。

`pkg/artifactcore` は artifact 契約ロジック層です。manifest 検証、ID算出、runtime-config merge/apply、strict検証を担当します。ここには CLI UX、compose 実行、Docker build/push 実行、`CONTAINER_REGISTRY` のような運用環境分岐を置きません。`artifactcore` から `cli/internal`, `tools/artifactctl`, `e2e`, `services` への逆参照は禁止です。

`tools/artifactctl` は実行アダプタ層です。`deploy` や `provision` の順序制御、Docker/Compose 実行、エラーヒント導線を持ちます。判定ロジックの正本を再実装してはいけません。判定は `pkg/artifactcore` を呼び出します。

`cli` はユーザー対話と生成経路のアダプタ層です。`tools/artifactctl` へ直接依存しません。artifact 適用時も `pkg/artifactcore` 契約を利用し、判定ロジックの二重実装を禁止します。

`e2e` は利用者視点の実行層です。`artifactctl` バイナリを外部コマンドとして扱います。repo 内の `tools/artifactctl` を暗黙ビルドする前提を持ちません。

## Plan of Work

最初に、境界契約のベースラインを固定します。`pkg/artifactcore` に存在する責務を「契約ロジック」と「実行ロジック」に分類し、現状の過不足を文書化します。この時点で「どの責務をどこに置くか」を曖昧にせず、配置ルールに落とします。

次に、`pkg/artifactcore` の縮小を行います。`ExecuteDeploy` のようなユースケース実行エントリを廃止し、`artifactcore` は `apply` 系契約APIに限定します。`docker buildx/tag/push` を伴う image prepare は `tools/artifactctl` 側の実行層へ移します。これにより `pkg` には共通契約のみが残ります。

続いて、`composeprovision` の再配置を行います。artifact 固有ではない compose run 引数構築は、`artifactcore` 配下から切り離し、必要なら独立共通パッケージへ移します。`cli` と `artifactctl` はその共通パッケージを参照し、artifact 契約層とは分離します。

その後に再発防止ガードを実装します。`tools/ci/check_tooling_boundaries.sh` を拡張し、import 方向だけでなく「禁止API利用」を検知します。具体的には、`pkg/artifactcore` と `pkg/yamlshape` で `os/exec`、Docker CLI直実行、運用環境変数依存を禁止し、違反時はCIを失敗させます。加えて `pkg/artifactcore` の公開APIを allowlist 管理し、意図しない export 増加を検知します。

最後に運用導線を整備します。PRテンプレートに「境界影響」記入を必須化し、`pkg` 変更時は境界レビュー項目を通さないとマージできないようにします。これにより、設計ドキュメントだけでなくレビュー運用にも再発防止を組み込みます。

## Milestones

### Milestone 1: Boundary Baseline Freeze

このマイルストーンでは、現行責務の棚卸しを確定し、どこまでを是正対象とするかを凍結します。対象ファイルを明示した差分リストを作り、移動・削除・維持の判定を記録します。完了条件は、次マイルストーンで迷いなく機械的に実装できる粒度まで責務分類が終わっていることです。

#### Milestone 1 Detailed Plan (Execution Ready)

Milestone 1 は「調査」ではなく「次工程の入力契約を固定する工程」として実施します。ここで曖昧さを残すと Milestone 2 以降で再び判断ぶれが発生するため、成果物をファイルとして固定します。

最初に責務インベントリを作ります。対象は `pkg/artifactcore`、`pkg/yamlshape`、`cli/internal/**`、`tools/artifactctl/**`、`e2e/**` です。各ファイルを「契約ロジック」「実行オーケストレーション」「I/Oアダプタ」「表示/UX」に分類し、分類根拠を行番号付きで記録します。

次に依存方向インベントリを作ります。`import` と呼び出し関係を抽出し、境界ルール違反の候補を列挙します。特に `pkg -> upper layer` 逆依存、`pkg` 内の `os/exec` や Docker 実行、`cli` と `artifactctl` の重複判定を重点確認します。

続いて公開APIインベントリを作ります。`pkg/artifactcore` と `pkg/yamlshape` の export 一覧を確定し、各APIを「維持」「縮小候補」「移設候補」に振り分けます。ここで Milestone 2 の削除対象を確定します。

その後、移設候補マップを作ります。各責務について「from」「to」「理由」「リスク」「受け入れテスト」を 1 対 1 で定義し、Milestone 2/3 で機械的に実装できるようにします。

最後に凍結判定を行います。凍結判定は以下の 4 条件を満たしたときのみ合格です。1) 責務分類に未判定ファイルがない。2) 移設候補に owner が付与されている。3) 依存方向違反候補に対して是正方針がある。4) 次マイルストーンの対象ファイルと受け入れテストが確定している。

Milestone 1 の成果物は `.agent/milestone1-boundary-baseline.md` に固定します。このファイルは以降のPRレビューで参照する正本とし、Milestone 2 以降で方針変更した場合は必ず `Decision Log` に追記します。

### Milestone 2: Artifactcore Deflation

このマイルストーンでは、`pkg/artifactcore` から実行オーケストレーション責務を排出します。公開APIを「契約ロジック最小集合」に絞り、呼び出し側 (`tools/artifactctl`, `cli`) を新境界に合わせて修正します。完了条件は、`artifactcore` が Docker/Compose 実行を持たず、契約処理のみで成立することです。

#### Milestone 2 Detailed Plan

Milestone 2 では、`pkg/artifactcore/execute.go` から `DeployInput` と `ExecuteDeploy` を削除し、`ExecuteApply` のみを契約エントリとして残します。同時に `pkg/artifactcore/prepare_images.go` を `tools/artifactctl` 側へ移設し、`artifactcore` から `os/exec` 依存を除去します。

移設先は `tools/artifactctl/pkg/deployops` とし、`deployops.Execute` が「入力正規化 -> image prepare -> artifact apply」の順序を担います。`tools/artifactctl/cmd/artifactctl/main.go` は `artifactcore.ExecuteDeploy` 呼び出しを廃止し、`deployops.Execute` 呼び出しへ差し替えます。

テストは、`pkg/artifactcore/execute_test.go` から deploy 系ケースを削除し、`tools/artifactctl/pkg/deployops` 側で deploy 系回帰を担保します。完了条件は、artifactcore が prepare/deploy 実行責務を持たず、`go test` が `cli`・`tools/artifactctl`・`pkg/artifactcore` で通ることです。

### Milestone 3: Shared Ops Boundary Normalization

このマイルストーンでは、`composeprovision` を artifactcore から切り離し、命名と責務を一致させます。`cli` と `artifactctl` の両方が同一の薄い運用ユーティリティを使う構造にし、どちらかにしかない判定分岐を排除します。完了条件は、重複ロジックが消え、依存方向が単純化されることです。

#### Milestone 3 Detailed Plan

Milestone 3 では `pkg/artifactcore/composeprovision` を artifact 契約層から分離し、artifact 非依存の共有運用層へ移設します。第一候補は `pkg/composeprovision` で、`cli/internal/infra/deploy/compose_provisioner.go` と `tools/artifactctl/cmd/artifactctl/main.go` の import を更新します。

この段階で `pkg/artifactcore` には compose 実行関心を残しません。`composeprovision` 側 API はできる限り現状互換に維持し、呼び出し側の挙動変更を避けます。完了条件は import 経路の単純化（artifactcore 非経由）と既存 provision テストの通過です。

### Milestone 4: Automated Recurrence Guards

このマイルストーンでは、再発防止をCIに組み込みます。境界違反を検知するシェルチェックを追加し、ワークフローで常時実行します。公開API増加の自動検知も導入し、意図しない責務拡張を阻止します。完了条件は、境界違反がPR段階で必ず失敗することです。

#### Milestone 4 Detailed Plan

Milestone 4 では `tools/ci/check_tooling_boundaries.sh` を拡張し、import 方向だけでなく責務禁止ルールを検査します。対象は `pkg/artifactcore` と `pkg/yamlshape` で、`os/exec`、`exec.Command`、Docker CLI 実行、運用環境変数依存（`CONTAINER_REGISTRY` など）を禁止します。

加えて、`pkg/artifactcore` 公開 API の allowlist を導入し、export 追加時に CI で検知できるようにします。`.github/workflows/quality-gates.yml` に境界チェックを組み込み、PR で常時実行します。完了条件は、意図的違反を入れた試験変更がCIで落ちることです。

### Milestone 5: Governance and Verification

このマイルストーンでは、PRレビュー運用とドキュメントを同期し、最終的な回帰検証を行います。UT/E2Eのフル実行と Docker クリーン環境再現までを完了条件に含めます。完了時に、この計画の `Outcomes & Retrospective` を更新してクローズします。

#### Milestone 5 Detailed Plan

Milestone 5 では、レビュー運用をコードと同じ強度で固定します。PR テンプレートまたはレビュー規約に「境界影響」「新規共通化の根拠」「allowlist 更新理由」を必須項目として追加します。

最終検証は `go test`（`cli`、`tools/artifactctl`、`pkg/artifactcore`、`pkg/yamlshape`）、runner UT、フル E2E の順で実行し、Docker クリーン状態で再現確認します。完了条件は、全ゲート通過と計画文書の最終 retrospective 更新です。

## Concrete Steps

作業ディレクトリは `/home/akira/esb` を前提とします。

1. ベースライン確認と責務棚卸し。

    rg -n "ExecuteDeploy|prepareImages|composeprovision" pkg/artifactcore cli tools/artifactctl
    rg -n "github.com/.*/pkg/artifactcore" cli tools/artifactctl e2e
    ./tools/ci/check_tooling_boundaries.sh

2. マイルストーン2-4の実装（責務移動、呼び出し修正、CIガード追加）。

3. 回帰テスト実行。

    go -C cli test ./...
    go -C tools/artifactctl test ./...
    GOWORK=off go -C pkg/artifactcore test ./...
    GOWORK=off go -C pkg/yamlshape test ./...
    uv run pytest -q e2e/runner/tests

4. Dockerクリーン環境でフルE2E再実行。

    docker compose down -v --remove-orphans || true
    docker system prune -f
    uv run e2e/run_tests.py --parallel --verbose

## Validation and Acceptance

受け入れ基準は、内部構造ではなく観測可能な振る舞いで判定します。`esb deploy` と `artifactctl deploy` が既存シナリオで成功し、`e2e/run_tests.py` フル実行が通ることを必須にします。加えて、境界違反を意図的に入れた試験変更で `tools/ci/check_tooling_boundaries.sh` が失敗することを確認し、再発防止ガードが実働していることを証明します。

`pkg/artifactcore` の公開APIは計画で定めた最小集合に収束し、新規追加には明示的な allowlist 更新と設計根拠が必要な状態を受け入れ条件にします。

## Idempotence and Recovery

この計画の実装は、段階的に適用しても再実行可能な手順で構成します。各マイルストーンは小さなPRに分割し、失敗時は該当マイルストーンだけをロールフォワード修正します。境界ガードが過剰検知した場合は、ルール側を緩めるのではなく、例外根拠を `Decision Log` に記録してから最小例外を追加します。

Dockerクリーンアップは開発環境に影響するため、E2E最終検証前にのみ実施し、必要なら再セットアップ手順 (`mise run setup`) を併記して復旧可能にします。

## Artifacts and Notes

この計画の実装時には、以下を成果物として残します。責務境界ルールドキュメント、CI境界チェックの実行ログ、公開API allowlist、フルUT/E2E成功ログ要約、そして境界違反を捕捉した証跡です。証跡は本計画の `Surprises & Discoveries` と `Outcomes & Retrospective` に転記します。

## Interfaces and Dependencies

最終状態で、`pkg/artifactcore` は artifact 契約処理を提供する最小APIを持ちます。`tools/artifactctl` はそのAPIを呼ぶ実行アダプタとし、順序制御やDocker/Compose操作を担当します。`cli` は `artifactcore` 契約を利用するが `tools/artifactctl` を import しません。`e2e` は `artifactctl` バイナリ呼び出しのみを契約とします。

`pkg` 配下に新規コードを追加する条件は、「少なくとも2つの上位モジュールが共通利用し、かつ運用依存を持たない」ことです。この条件を満たさないコードは `pkg` へ置かず、呼び出し側アダプタへ配置します。

## Plan Revision Note

2026-02-19: `pkg` 肥大化と責務過多の再発を止めるため、境界ルールとCI強制を含むマスター計画として新規作成した。ラウンド修正を繰り返す前提を廃止し、恒久ルールで収束させる方針に切り替えた。
2026-02-19: Milestone 1 の詳細計画を追記し、実行結果を `.agent/milestone1-boundary-baseline.md` に固定した。以降の移設判断はこの凍結結果を正本とする。
