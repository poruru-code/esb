# Deploy Artifacts First: CLI 非依存実行への移行

この ExecPlan は living document です。`Progress`、`Surprises & Discoveries`、`Decision Log`、`Outcomes & Retrospective` を作業に合わせて更新し続けます。

本計画は `.agent/PLANS.md` の運用ルールに従います。実装中に方針変更があった場合は、必ずこの文書を更新してからコードを更新します。

## Purpose / Big Picture

この変更が完了すると、`esb` CLI が存在しない環境でも、`esb deploy` が生成した成果物だけでシステムを起動・更新できるようになります。CLI は「唯一の実行主体」ではなく、「成果物生成と適用を簡単にする補助ツール」になります。利用者は、(1) CLI を使う標準運用、(2) 成果物を手動作成して運用する非 CLI 運用、の両方を選べます。

目で確認できる完成条件は、`esb` バイナリを `PATH` から外した状態で、成果物ディレクトリと `docker compose` だけで Gateway が起動し、`/health` が 200 を返し、関数 invoke が成功することです。

## Progress

- [x] (2026-02-17 13:10Z) 現行の `deploy` 実装、`staging` 配置、`runtime-config` 同期経路、`bundle manifest` 利用箇所を調査した。
- [x] (2026-02-17 13:30Z) 責務分離境界と UX 仕様の初版を本 ExecPlan に整理した。
- [x] (2026-02-17 15:40Z) CLI 実行スコープ制約（repo 外では `version/help` のみ許可）を計画へ反映した。
- [x] (2026-02-17 16:20Z) 推奨順序に焦点を当てた優先実施フェーズ（Phase 1-5）と PR 分割方針を追加した。
- [x] (2026-02-17 16:40Z) Phase 1 を実装し、CLI 表示名を `esb` 固定にした（`CLI_CMD` によるコマンド名変更を無効化）。
- [x] (2026-02-17 17:05Z) Phase 2 を実装し、repo 外では `version/help` 以外を終了コード 2 で fail-fast するガードを導入した。
- [x] (2026-02-17 18:00Z) Phase 3 の初期実装として `services/agent` に StackIdentity Resolver を導入し、namespace/CNI/container 名を compose 文脈（`PROJECT_NAME`/`ENV`/`CONTAINERS_NETWORK`）から解決する経路を追加した。
- [x] (2026-02-17 18:00Z) agent compose 環境へ `PROJECT_NAME` を注入し、docker/containerd/firecracker の各 compose で stack identity 解決入力を揃えた。
- [x] (2026-02-17 18:00Z) runtime ラベルキーと image naming の prefix 解決を StackIdentity ベースへ切り替え、`ESB_TAG` への後方互換 fallback を追加した。
- [x] (2026-02-17 18:00Z) `services/agent` コードから `meta` import を除去し、`services/agent/go.mod` の `meta` require/replace 依存を削除した（CLI/agent 分離に向けた前倒し整理）。
- [x] (2026-02-17 18:30Z) Phase 4 着手として `cli` の `meta` 依存を `cli/internal/meta` へ移設し、`cli/go.mod` の `meta` require/replace を削除した。あわせて agent build の `meta_module` context と共有 `meta/` モジュールを撤去した。
- [x] (2026-02-17 19:56Z) Phase 4 を完了し、`docs/runtime-identity-contract.md` と `services/agent/docs/*` を StackIdentity 契約へ更新した。`meta.*` 前提記述を除去し、CLI/Agent の全テスト pass を再確認した。
- [x] (2026-02-17 20:00Z) Phase 5 の先行着手として `docs/deploy-artifact-contract.md` を追加し、`cli/internal/usecase/deploy` に artifact manifest の型/検証/atomic write/read の基盤を導入した。
- [x] (2026-02-17 22:20Z) Phase 5.5-A として `runtime/*/templates` を `cli/assets/runtime-templates` へ移設し、CLI からの参照を runtime module 依存なしで解決する経路へ切り替えた。
- [x] (2026-02-17 23:40Z) Phase 5.5-B として runtime hooks/proto/bootstrap の配置を再編し、`runtime-hooks/**`、`services/contracts/proto/**`、`tools/bootstrap/**` へ統一した。`runtime` 共有モジュールは削除した。
- [x] (2026-02-17 23:50Z) Phase 5.5-C として `docs/repo-layout-contract.md` と `tools/ci/check_repo_layout.sh` を追加し、旧パス再混入を CI で検知できるようにした。
- [x] (2026-02-18 00:20Z) Artifact 契約を Manifest-First に改訂し、複数テンプレート適用の正本を `artifact.yml`（`artifacts[]` 順序付き）へ統一した。
- [x] (2026-02-18 01:10Z) あるべき論レビューを反映し、契約単一化（`artifact.yml` only）、Go 実装単一化（shell は薄いラッパのみ）、フォールバック最小化（必須入力欠落時 hard fail）を本計画の基準へ再固定した。
- [x] (2026-02-18 02:20Z) Milestone 1 を完了した。`ArtifactManifest`（YAML）型へ移行し、`esb deploy` 後に `artifact.yml` を自動生成する実装と単体テストを追加した。
- [x] (2026-02-18 02:40Z) Phase B baseline を実装した。`tools/artifactctl`（`validate-id` / `merge` / `apply`）と `tools/artifact/merge_runtime_config.sh` ラッパを追加し、manifest/merge/apply の Go 正本を集約した。
- [x] (2026-02-18 02:50Z) CLI アダプタを追加した。`esb artifact apply` を追加し、`tools/artifactctl/pkg/engine` を直接呼ぶ薄いアダプタへ接続した。
- [x] (2026-02-18 03:00Z) E2E runner の artifact driver 初期対応を実装した。`deploy_driver=artifact` で `artifactctl apply` + provisioner 実行へ分岐し、`run_tests.py` で `artifactctl` ローカルビルドを条件化した。
- [x] (2026-02-18 04:20Z) `esb deploy` の Apply フェーズで `artifact.yml` を先行生成して `tools/artifactctl` Engine (`Apply`) を実行する経路を追加した。Apply 前に manifest を準備し、最後に strict で再出力する構成へ更新した。
- [x] (2026-02-18 04:55Z) Apply フェーズのフォールバックを削減した。`artifact path` 未指定時は hard fail とし、`runtime-config` 同期は `TemplatePath` 再解決ではなく Generate で確定した `stagingDir` を直接入力に使うよう変更した。
- [x] (2026-02-18 05:20Z) `esb deploy` の実行順序を「全テンプレート Generate（BuildOnly）→ strict `artifact.yml` 出力 → `workflow.Apply` で一度だけ Apply」に変更した。Apply で再ビルドしない責務分離へ更新した。
- [x] (2026-02-18 05:25Z) Milestone 2 を完了した。`deploy` は Generate と Apply を内部で明確に分離し、`esb deploy` は互換挙動を維持したまま「Generate 集約 → strict manifest → Apply 一回」の順序で実行されるようにした。
- [x] (2026-02-18 06:10Z) Milestone 3 を実装した。`esb artifact generate` を追加し、`esb deploy` の generate 経路を再利用する CLI UX を整備した。`docs/artifact-operations.md` を追加して CLI あり/なしの運用手順を明文化した。
- [x] (2026-02-18 06:20Z) Milestone 4 の先行実装として `e2e-docker-artifact` matrix を追加し、artifact driver を `artifact generate -> artifactctl apply -> provisioner` の二段経路で実行するよう更新した。
- [x] (2026-02-18 07:05Z) `uv run e2e/run_tests.py --parallel --verbose` を実行し、`e2e-docker` / `e2e-docker-artifact` / `e2e-containerd` の全 matrix entry が PASS することを確認した。Milestone 4 を完了とした。
- [x] (2026-02-18 08:10Z) Phase F の cleanup を実施した。`artifact_descriptor` 系を削除し、`artifact.yml` 経路へ一本化した。併せて `cli/internal/command/branding.go` と専用テストを削除し、CLI 名称固定ロジックを `app.go` へ集約した。
- [x] (2026-02-18 08:40Z) 並列 E2E 時の Java runtime jar 競合を解消した。staging への jar 配置を hard link から copy に変更し、`runtime-hooks/java/*` 更新は一時ディレクトリ経由の atomic `mv` に変更した。`uv run e2e/run_tests.py --parallel --verbose` の再実行で全 matrix PASS を確認した。
- [x] Milestone 4: CLI 非依存 E2E を追加し、回帰を防止する。

## Surprises & Discoveries

- Observation: 実行時に同期される設定はテンプレート出力ディレクトリではなく `staging` のマージ結果である。
  Evidence: `cli/internal/usecase/deploy/runtime_config.go` で `staging.ConfigDir(templatePath, composeProject, env)` を起点に同期している。

- Observation: すでに `tools/dind-bundler` は「manifest を唯一入力」とする運用を採用しており、artifact-first 化の先行事例になっている。
  Evidence: `tools/dind-bundler/README.md` の「マニフェスト駆動」節、`tools/dind-bundler/build.sh` の manifest 統合フロー。

- Observation: ランタイムサービス（Gateway/Provisioner/Agent）は `esb` バイナリを参照していない。必要なのは `/app/runtime-config` と証明書マウントである。
  Evidence: `services/gateway/config.py` と `services/provisioner/src/main.py` の入力パス定義、`docker-compose.*.yml` の volume 定義。

- Observation: 現行 E2E runner は `e2e/run_tests.py` で `ensure_local_esb_cli()` を先に実行するため、CLI 非依存経路の検証ができない。
  Evidence: `e2e/run_tests.py` の `if not args.unit_only: ensure_local_esb_cli()` と `e2e/runner/deploy.py` の `build_esb_cmd(["deploy", ...])`。

- Observation: 共有 `meta` モジュールは実質的に `cli` と `services/agent` のみが利用し、`go.mod replace => ../meta` で結合されている。
  Evidence: `cli/go.mod` と `services/agent/go.mod` の replace 定義、および `rg "github.com/poruru/edge-serverless-box/meta"` の参照結果。

- Observation: 複数テンプレートが別ディレクトリに分散するため、`.esb` path 推論と手動列挙（`ARTIFACT_ROOTS`）は運用ミスを誘発する。
  Evidence: `deploy_inputs_resolve.go` の multi-template 出力分岐、テンプレート毎に異なる base dir へ `.esb` が作成される現挙動。

- Observation: 並列 deploy で Java runtime jar が破損するケースがあり、原因は `runtime-hooks/java/*` を in-place 更新しつつ function staging 側に hard link していたことだった。
  Evidence: `e2e-containerd` Java invoke 失敗時の JVM エラー（`Error opening zip file or JAR manifest missing`）と、`stage.go` の `linkOrCopyFile` + `stage_java_runtime.go` の直接 `cp` 更新の組み合わせを再現調査で確認。

## Decision Log

- Decision: 成果物の正本は「artifact manifest + runtime-config 一式」にする。`staging` は内部キャッシュとして残し、外部契約から切り離す。
  Rationale: 現在の `staging` 解決は `templatePath` と repo 依存が強く、非 CLI 環境で再現しにくい。
  Date/Author: 2026-02-17 / Codex

- Decision: 複数テンプレート適用の正本は `artifact.yml` とし、配列順を deploy 順/merge 順の唯一の真実にする。`.esb` 探索や `ARTIFACT_ROOTS` 手動列挙は廃止する。
  Rationale: path 推論依存を除去し、手動運用でも適用対象・順序・再現性を 1 ファイルで固定するため。
  Date/Author: 2026-02-18 / Codex

- Decision: `esb deploy` は後方互換を維持し、内部的には Generate と Apply を順に呼ぶ合成コマンドにする。
  Rationale: 既存利用者の CLI 契約を壊さず、責務分離だけを先に進められる。
  Date/Author: 2026-02-17 / Codex

- Decision: 非 CLI 運用の最小契約は「artifact manifest + runtime-config + compose 環境変数」で成立させ、証明書生成は deploy 責務に含めない。
  Rationale: 証明書は `setup:certs` 系のプラットフォーム初期化責務であり、テンプレート依存の deploy 成果物と性質が異なる。
  Date/Author: 2026-02-17 / Codex

- Decision: runtime のうち「実行時フック（Python sitecustomize / Java agent / Java wrapper）」は ESB 本体責務に固定し、「関数 Dockerfile テンプレート」は CLI 責務として分離可能にする。
  Rationale: 前者はシステム挙動そのもの（ログ/トレース/SDK パッチ）を規定し、後者は成果物生成器の実装詳細であるため、リポジトリ分割時の変更頻度・責務が異なる。
  Date/Author: 2026-02-17 / Codex

- Decision: 将来の repo 分離は 2-way split（`esb-core` と `esb-cli`）を前提にし、runtime templates は `esb-cli` 側または runtime-pack 配布物へ移す。
  Rationale: 現在 `runtime/templates_embed.go` が CLI バイナリにテンプレートを embed しており、依存方向を CLI -> runtime-templates に保つのが自然である。
  Date/Author: 2026-02-17 / Codex

- Decision: ディレクトリ責務分離の canonical path は `cli/assets/runtime-templates`、`runtime-hooks`、`services/contracts/proto`、`tools/bootstrap` に固定し、旧 `runtime`/`proto`/`bootstrap` ルートは再導入禁止とする。
  Rationale: repo 分離前に path 契約を固定しないと、後続実装で旧経路が混入して責務境界が曖昧になるため。静的ガード（`check_repo_layout.sh`）で fail-fast する。
  Date/Author: 2026-02-17 / Codex

- Decision: 互換判定の主軸は version（major/minor）にし、digest は既定では監査・再現性用途に限定する。digest 一致必須は strict モード時のみ有効化する。
  Rationale: 常時 digest を強制すると、運用柔軟性とメンテナンスコストが上がるため。通常運用は version 互換で判定し、CI/リリース検証だけを strict にする。
  Date/Author: 2026-02-17 / Codex

- Decision: E2E は「成果物生成経路」と「成果物消費経路」を分離して検証する。前者は CLI 許可、後者は CLI 禁止（PATH から `esb` 除外）で実施する。
  Rationale: artifact-first 要件は「生成できること」と「生成済み成果物だけで動くこと」の両方を保証しないと回帰検知できないため。
  Date/Author: 2026-02-17 / Codex

- Decision: `compose.env` は非機密のみを同梱し、機密情報（`JWT_SECRET_KEY`、`X_API_KEY`、`AUTH_USER`、`AUTH_PASS`、`RUSTFS_ACCESS_KEY`、`RUSTFS_SECRET_KEY`）は成果物外で注入する。
  Rationale: 成果物配布に機密を同梱すると漏えいリスクと運用事故が増えるため。artifact は再配布可能な非機密パッケージとして扱う。
  Date/Author: 2026-02-17 / Codex

- Decision: `compose.env` は allowlist 方式で生成し、secret キーの混入を生成時に hard fail で拒否する。`artifact apply` は `--secret-env` で外部機密を受け取り、ログにはキー名のみ出して値は一切出力しない。
  Rationale: 「非機密のみ同梱」の方針を運用依存にすると将来の変更で破られやすいため、生成時と適用時に機械的なガードを持たせる。
  Date/Author: 2026-02-17 / Codex

- Decision: マイグレーションは単一リリースで完了させず、3 段階で出す。Phase A は artifact contract + manifest、Phase B は deploy 分離 + new CLI UX、Phase C は runner の deploy_driver 分岐と artifact profile CI。
  Rationale: 変更範囲が広く、まとめて投入すると回帰時の切り戻しコストが高いため。
  Date/Author: 2026-02-17 / Codex

- Decision: `esb deploy` の `artifact.yml` 既定出力先は `<repo_root>/.esb/artifacts/<project>/<env>/artifact.yml` とする。entry の `artifact_root` は manifest からの relative path を既定出力にする。
  Rationale: 複数テンプレート/外部テンプレート混在時でも 1 つの正本 manifest へ収束させ、移送時の可搬性を高めるため。
  Date/Author: 2026-02-18 / Codex

- Decision: CLI の実行スコープは「EBS リポジトリ配下の CWD」に限定する。例外として `version` と `help` はどこからでも実行可能にする。
  Rationale: deploy/artifact 系は repo 資産（compose, templates, defaults）への依存が強いため、実行境界を明示しないと誤実行とサポートコストが増える。一方で `version/help` は自己完結情報なので repo 依存を持たせない。
  Date/Author: 2026-02-17 / Codex

- Decision: CLI コマンド名は `esb` 固定とし、`CLI_CMD` などの外部設定によるコマンド名変更を廃止する。
  Rationale: CLI を独立配布する方針では、コマンド名の可変性は互換テスト・ドキュメント・サポートコストを増やす。コマンド面は固定化し、ブランド差分は runtime/config 資産側に閉じ込める。
  Date/Author: 2026-02-17 / Codex

- Decision: `meta/meta.go` の共有モジュールは廃止し、必要定数は各モジュールへ内包する（CLI 側、Agent 側）。
  Rationale: 共有メタ定数は repo 分離時の密結合点になり、`replace ../meta` 依存が独立リリースの障害になるため。モジュールごとに責務境界内で自己完結させる。
  Date/Author: 2026-02-17 / Codex

- Decision: module-local 定数の重複は許容し、代わりに「共有契約値だけ」をドキュメントで固定して drift を管理する。
  Rationale: 再共有パッケージを作ると同じ結合問題が再発するため。重複は意図的に受け入れ、契約値（label key、runtime namespace、CA path 等）を明示契約としてテストで守る。
  Date/Author: 2026-02-17 / Codex

- Decision: サービス側のブランド識別は実行時に解決する。`meta` の固定定数ではなく、compose stack 文脈（`PROJECT_NAME`、`ENV`、`CONTAINERS_NETWORK`）から `brand slug` を導出する resolver を導入する。
  Rationale: CLI 名称を `esb` 固定にしても、下流環境では stack 名・namespace 名はブランド差分を持ちうるため。サービスは「実行文脈」から識別する方が repo 分離後も安定する。
  Date/Author: 2026-02-17 / Codex

- Decision: Artifact Apply の実行ロジックは Go 実装を唯一正本とし、shell はラッパに限定する。二重実装は禁止する。
  Rationale: ロジックを shell/Go に分散すると将来の仕様差分・バグ修正漏れを招くため。
  Date/Author: 2026-02-18 / Codex

- Decision: `artifacts[].id` の算出は可搬性優先とし、絶対パスや実行環境依存値を入力に使わない。
  Rationale: 成果物の移送先で ID 再計算が不一致になる仕様は契約として成立しないため。
  Date/Author: 2026-02-18 / Codex

- Decision: フォールバックは phase 前提の暫定措置を除き原則禁止とする。必須入力欠落は warning ではなく hard fail とする。
  Rationale: 過剰フォールバックは不整合を隠蔽し、障害の検知遅延を引き起こすため。
  Date/Author: 2026-02-18 / Codex

## Outcomes & Retrospective

現時点では Phase 1-4（CLI 名称固定、repo scope guard、service stack identity、meta 共有モジュール撤去）を実装済みで、分離の前提条件は揃った。次の主要リスクは artifact-first 本体導入時に `staging` 互換を保ちながら責務境界を切り替える移行設計である。

## Context and Orientation

ここで言う「成果物」は、テンプレート解析やビルド結果を、後段の実行環境が直接消費できる形で保存したディレクトリを指します。現行コードでは成果物が二系統に分かれています。1つ目はテンプレート近傍の出力（例: `<template_dir>/.esb/<env>/config`）で、2つ目は `staging` のマージ結果（`<repo_root>/.esb/staging/<project>/<env>/config`）です。実行系コンテナへ同期されるのは後者です。

主要ファイルの読み方は次の通りです。`cli/internal/infra/build/go_builder.go` が生成とビルドを実行し、`cli/internal/infra/build/merge_config_entry.go` が `staging` へマージします。`cli/internal/usecase/deploy/deploy_run.go` は deploy の全体順序を管理し、`cli/internal/usecase/deploy/runtime_config.go` が実行中コンテナまたは volume に設定を同期します。`cli/internal/infra/templategen/bundle_manifest.go` は image 集約用 manifest を出力します。ランタイム側は `services/gateway/config.py` と `services/provisioner/src/main.py` が `/app/runtime-config` を読むだけで、CLI を要求しません。

この計画で定義する責務境界は次の3つです。Artifact Producer は成果物を作る責務、Artifact Applier は成果物を実行環境へ反映する責務、Runtime Consumer は反映済みファイルを読むだけの責務です。`esb` CLI は Producer/Applier のフロントエンドに限定し、Runtime Consumer から完全に分離します。

runtime 分離の観点では、`runtime/*` を次の2系統に分けて扱います。A系統はシステム必須フックで、Python の `sitecustomize.py`、Java の `lambda-java-agent.jar` と `lambda-java-wrapper.jar` のように、実行時のログ/トレース/SDK 挙動を成立させる資産です。B系統は生成器都合の資産で、`runtime/java/templates/dockerfile.tmpl` や `runtime/python/templates/dockerfile.tmpl` のように CLI が Dockerfile を生成するためだけに必要な資産です。将来の repo 分離では A系統を core 側契約として残し、B系統を CLI 側へ寄せることを基本方針にします。

## Milestones

### Milestone 1: Artifact Contract の明文化と Manifest 生成

このマイルストーンでは、成果物を人間とツールの両方が読める契約として固定します。`docs/deploy-artifact-contract.md` に必須ファイル、相対パス規約、バージョン互換ポリシーを定義します。並行して `esb deploy` / `esb artifact generate` 実行時に、`artifact.yml` を出力し、複数テンプレート情報は `artifacts[]` に記録します。

完了時点で、利用者は `artifact.yml` の `source_template` で生成元テンプレートを追跡でき、複数テンプレート適用では `artifact.yml` だけで対象と順序を判別できます。検証は unit test と artifact manifest 実ファイル検査で行います。

このマイルストーン内で runtime 分離メタデータも導入します。`artifact.yml` の各 entry に `runtime_hooks` と `template_renderer` の識別情報（name/api_version）を推奨項目として持たせ、digest は任意の証跡情報として保持します。既定判定は api_version 互換で行い、digest 一致チェックは strict モード時のみ有効にします。

### Milestone 2: Generate/Apply の内部責務分離

このマイルストーンでは `cli/internal/usecase/deploy/deploy_run.go` を再構成し、Generate フェーズと Apply フェーズを別メソッドへ分離します。Generate は「テンプレート解析、生成、image build、runtime-config 最終化、artifact manifest 出力」まで、Apply は「artifact manifest を入力に image prewarm、runtime-config 同期、provisioner 実行」だけを担当します。

完了時点で `esb deploy` の外部挙動は維持しつつ、内部ではフェーズを個別に呼び出せます。これにより `artifact apply` を後から追加してもロジック重複が発生しません。検証は既存 deploy テスト群と新規フェーズ単体テストで行います。

### Milestone 3: UX の明示化（CLI UX と非CLI UX）

このマイルストーンでは利用者の操作系を仕様化して実装します。CLI UX として `esb artifact generate` と `esb artifact apply` を追加し、`esb deploy` はその合成動作として残します。`artifact apply` は `--artifact <artifact.yml>` のみを受け付けます。非CLI UX として、artifact manifest と runtime-config を入力に `tools/artifactctl` + `docker compose` で動かす手順を `docs/artifact-operations.md` に記載します。

完了時点で「CLI がある場合の最短経路」と「CLI がない場合のオペレータ実行経路」が同じ成果物契約を共有します。CLI は便利機能であり、唯一の実行経路ではない状態になります。

### Milestone 4: CLI 非依存 E2E の追加

このマイルストーンでは、成果物生成後に `esb` を使わないで起動と invoke を行う E2E を追加します。`e2e/scenarios/standard` へ新規シナリオを追加し、`PATH` から `esb` を除外した状態で `docker compose` と HTTP invoke だけで検証します。加えて runner 側に deploy driver の分岐（`cli` / `artifact`）を追加し、matrix で経路を選択可能にします。変更対象は `e2e/runner/models.py`（`Scenario.extra.deploy_driver`）、`e2e/runner/config.py`（matrix 取り込み）、`e2e/runner/deploy.py`（driver 実行分岐）、`e2e/run_tests.py`（`ensure_local_esb_cli` 条件化）です。

完了時点で、将来の変更が「成果物だけで動作する」保証を壊したら CI で検知できます。さらに `esb deploy` の従来経路も並行で検証されるため、移行中の互換性低下を早期検知できます。

## Focused Rollout Order (Architecture Reset)

このセクションは「あるべき論」に合わせた再固定版です。既存フェーズの履歴は保持しつつ、未完了作業の順序は以下を唯一の優先順とします。

### Phase A: Contract Freeze（契約凍結）

`artifact.yml` 単一正本、`artifacts[]` 単一順序、パス規約、ID 規約、secret 規約を文書と型に同時固定します。
ここでの原則は「仕様に曖昧さを残したまま実装を進めない」です。

完了条件:
- `artifact.yml` 以外は apply 入力として受理しない
- path 規約が 1 通りに固定される
- `runtime_meta` の required/optional が plan/contract/型で一致する
- `canonical_template_ref` 正規化アルゴリズムが実装仕様として固定される

### Phase B: Artifact Engine（Go 実装一本化）

apply/merge/validation の本体ロジックは Go 側に一本化します。shell は「引数を受けて Go 実装を呼ぶだけ」の薄いラッパに限定し、ロジック二重実装は禁止します。

完了条件:
- `tools/artifact/merge_runtime_config.sh` にロジックを持たない
- Go の単一実装が CLI あり/なし両経路で再利用される
- `tools/artifactctl merge/apply/validate-id` が判定・実行の唯一正本になる

### Phase C: CLI Adapter / Non-CLI Adapter 分離

CLI ありは `esb artifact generate/apply`、CLI なしは同じ Engine を呼ぶ経路（`tools/artifactctl` など）を整備します。
どちらの経路も入力は `artifact.yml` のみです。

完了条件:
- CLI 有無で結果差分が出ない
- `deploy` は generate+apply の合成に限定される

### Phase D: Runtime Hardening（フォールバック最小化）

次 phase で必要なものを除き、フォールバックを撤去します。必須入力不足は warning ではなく hard fail に切り替えます。

完了条件:
- サービス側ブランド解決の暗黙 fallback を禁止
- 必須 env 欠落時に即失敗する

### Phase E: E2E Matrix Gate

`deploy_driver=cli` と `deploy_driver=artifact` の両経路を CI で必須ゲート化します。さらに full E2E（`--parallel --verbose`）を受け入れ条件へ組み込みます。

完了条件:
- `docker compose up` で service 起動が通る
- `uv run e2e/run_tests.py --parallel --verbose` がフル完走する

### Phase F: Cleanup & Deletion

旧 descriptor、旧 apply 経路、冗長 fallback、未使用コードを削除し、再混入を CI ガードで防止します。

完了条件:
- 未使用コード/冗長コードの残存がない
- 旧契約 (`artifact.json` 系) の参照が 0 件

## PR Slicing Policy

1 フェーズ 1 PR を継続します。base は `develop` 固定とし、`main` への直接マージは行いません。
推奨順序は `PR-A contract-freeze` -> `PR-B artifact-engine` -> `PR-C adapters` -> `PR-D runtime-hardening` -> `PR-E e2e-gate` -> `PR-F cleanup` です。

## Plan of Work

1. Contract Freeze を先に完了させます。`docs/deploy-artifact-contract.md` と本 ExecPlan の規約差分を 0 にし、`artifact.yml` のみを正本に固定します。
2. Artifact Engine を Go 実装で作ります。manifest 検証、runtime-config merge、secret 検証、apply 実行を単一 usecase に集約します。
3. CLI Adapter を薄くします。`esb artifact apply` は Engine 呼び出しだけを担当し、ロジックを持ちません。
4. Non-CLI Adapter を追加します。`tools/artifactctl apply --artifact <artifact.yml>` を用意し、CLI 非依存運用でも同一 Engine を呼びます。
5. Runtime Hardening を実施します。ブランド解決・設定解決の過剰フォールバックを除去し、必須入力欠落は hard fail に統一します。
6. E2E Matrix Gate を追加します。`deploy_driver=cli` と `deploy_driver=artifact` を同時に維持し、最終的に full E2E を必須化します。
7. Cleanup で旧実装を削除します。`artifact.json` 系、使われない helper、冗長分岐を除去し、CI で再混入を禁止します。

## UX Specification

標準 UX は `esb artifact generate`、`esb artifact apply`、`esb deploy`（generate+apply の合成）で固定します。`esb` は生成・適用の補助であり、実行環境の必須依存にはしません。

CLI なし UX は「同じ Artifact Engine を別アダプタで呼ぶ」方式で提供します。手順は `tools/artifactctl apply --artifact <artifact.yml>` を基本とし、`.esb` 探索や `ARTIFACT_ROOTS` 手動列挙は許可しません。
ここでの「手動」はオペレータが実行する運用を指し、ロジックを shell へ再実装する意味ではありません。

フェーズ別の明文化:
- Phase 1/2（テンプレート解析・生成）は CLI ありが標準
- Phase 3 以降（build/apply/provision/up）は CLI あり/なし両対応
- どちらの経路でも入力契約は `artifact.yml` 単一

セキュリティ UX:
- `compose.env` には非機密のみを許可
- `required_secret_env` 未充足は apply 前に hard fail
- secret 値はログ出力禁止（キー名のみ）

エラーポリシー:
- 必須ファイル欠落、schema major 不一致、path 規約違反、required secret 欠落は hard fail
- digest/checksum は既定 warning、`--strict` で hard fail

CLI スコープ:
- repo 外で許可するのは `version/help` のみ
- それ以外は終了コード 2 で失敗
- fail-closed を採用し、新規サブコマンドは明示 exempt されない限り repo 必須

サービス側ブランド解決:
- `ESB_BRAND_SLUG` または `PROJECT_NAME` / `ENV` は必須入力として扱う
- 必須入力が解決できない場合は hard fail（運用時 fallback は許可しない）

## Concrete Steps

作業者は常に `/home/akira/esb` で作業します。

1. `contract-freeze` PR
   `artifact.yml` 単一正本、パス規約、ID 規約、secret 規約を文書と型で固定します。

2. `artifact-engine` PR
   `cli/internal/usecase/deploy` に apply エンジンを実装し、manifest 検証・merge・secret 検証を集約します。

3. `adapter-cli` PR
   `esb artifact apply` をエンジン呼び出しの薄いアダプタへ整理します。

4. `adapter-non-cli` PR
   `tools/artifactctl apply --artifact <artifact.yml>` を追加し、CLI 非依存運用経路を確立します。

5. `runtime-hardening` PR
   サービス側の過剰フォールバックを撤去し、必須 env 欠落を hard fail に統一します。

6. `e2e-gate` PR
   `deploy_driver=cli` / `deploy_driver=artifact` を matrix に常設し、両経路を回帰テストへ昇格します。

7. `cleanup` PR
   `artifact.json` 系、未使用 helper、冗長分岐を削除し、再混入検知を CI に追加します。

## Validation and Acceptance

受け入れ条件は次の 8 条件を同時に満たすことです。

1. `artifact.yml` 単一正本で apply が成立し、`artifact.json` 系入力を受理しない。
2. `docker compose up` で service が正常起動する（docker/containerd/firecracker）。
3. `deploy_driver=artifact` で `esb` 非依存起動・invoke が成功する。
4. `uv run e2e/run_tests.py --parallel --verbose` がフル完走する。
5. `compose.env` に secret 値が混入せず、`required_secret_env` 未充足は hard fail する。
6. `artifacts[].id` が形式・一意性・再計算一致を満たす。
7. CLI/サービス双方のフォールバック削減方針（必須入力欠落は hard fail）がテストで保証される。
8. 複数テンプレート（別ディレクトリ配置を含む）で `artifacts[]` 配列順どおりの merge/apply が再現される。

実行コマンド（最低限）:

    cd /home/akira/esb/cli
    go test ./... -count=1

    cd /home/akira/esb/services/agent
    go test ./... -count=1

    cd /home/akira/esb
    uv run pytest -q e2e/runner/tests

    cd /home/akira/esb
    uv run e2e/run_tests.py --parallel --verbose

    cd /home/akira/esb
    rg -n "artifact\\.json|ArtifactDescriptor|ReadArtifactDescriptor|WriteArtifactDescriptor" cli/internal/usecase/deploy && exit 1 || true

    cd /home/akira/esb
    rg -n "JWT_SECRET_KEY=|X_API_KEY=|AUTH_PASS=|RUSTFS_SECRET_KEY=" <artifact-manifest-dir>/compose.env && exit 1 || true

    cd /home/akira/esb
    yq -e '.artifacts[].id | select(test("^[a-z0-9-]+-[0-9a-f]{8}$") | not)' <artifact-manifest-dir>/artifact.yml && exit 1 || true
    test "$(yq -r '.artifacts[].id' <artifact-manifest-dir>/artifact.yml | wc -l | tr -d ' ')" = "$(yq -r '.artifacts[].id' <artifact-manifest-dir>/artifact.yml | sort -u | wc -l | tr -d ' ')" || exit 1

    cd /home/akira/esb
    tools/artifactctl validate-id --artifact <artifact-manifest-dir>/artifact.yml

    cd /home/akira/esb
    uv run pytest -q e2e/scenarios/standard/test_artifact_runtime.py -k multi_template

artifact profile の matrix 条件:
- `e2e/environments/test_matrix.yaml` に `deploy_driver: artifact` を含む profile が定義されていること
- 同 profile は `ensure_local_esb_cli` を経由せずに deploy/test を完走できること

## Idempotence and Recovery

`artifact generate` は同一入力に対して上書き再実行可能にします。manifest は atomic write（temp file から rename）で書き込み、途中失敗時は壊れたファイルを残しません。`artifact apply` は複数回実行しても同じ最終状態になるようにし、既存の `copyFile` 原子的更新と provisioner の冪等動作（既存リソース skip）を利用します。

失敗時の復旧は単純に再実行できる形にします。manifest validation 失敗時は何も反映しない早期 abort、runtime 同期失敗時は同期済みファイルの差分をログへ出し、修正後に再実行できるようにします。

Apply は「同期」と「provision」を分離して記録し、`artifact-apply.state.json` に直近状態を書き出します。`sync_done=true/provision_done=false` の中断ケースを明示できるようにし、再実行時にどこからやり直したかを出力します。ロールバックは自動では行わず、状態を可視化して安全再試行を優先します。

## Artifacts and Notes

成果物の想定レイアウト（v1）は次です。

    <artifact-manifest-dir>/
      artifact.yml
      compose.env
      compose.secrets.env.example

    <artifact-root-a>/
      runtime-config/
        functions.yml
        routing.yml
        resources.yml
        image-import.json
      bundle/
        manifest.json

`artifact.yml` の最小イメージは次です。

    schema_version: "1"
    project: esb-dev
    env: dev
    mode: docker
    artifacts:
      - id: template-a-2b4f1a9c
        artifact_root: ../service-a/.esb/template-a/dev
        runtime_config_dir: runtime-config
        image_prewarm: all
        source_template:
          path: /path/to/template-a.yaml

`artifacts[].id` の採番ルールは次で固定します。`id=<template_slug>-<h8>` を使用し、`template_slug` は `source_template.path` の basename（拡張子除去）を `[a-z0-9-]` へ正規化した値、`h8` は `sha256(canonical_template_ref + "\n" + canonical_parameters + "\n" + canonical_source_sha256)` の先頭 8 桁です。`canonical_template_ref` は `source_template.path` の文字列を正規化した参照値（`/` 区切り、絶対/相対の区別は保持）を使用し、ファイルシステム解決（絶対化・symlink 解決）は行いません。`canonical_source_sha256` は `source_template.sha256`（未指定時は空文字）です。`canonical_parameters` は key 昇順 `key=value` の `\n` 連結（末尾改行なし）です。連番は使用しません。`id` は追跡用途であり、適用順序は常に `artifacts[]` 配列順です。apply 時は `id` 再計算値との一致を必須検証（不一致 hard fail）とします。

`compose.env` は CLI なし起動のために非機密のみを持ちます。必須は `PROJECT_NAME`、`CONFIG_DIR`、`CERT_DIR`、必要に応じて `MODE` です。`compose.secrets.env.example` は必須機密キーのテンプレートのみを持ち、実値は含めません。`compose.env` は allowlist 生成とし、secret 系キーが入った場合は artifact 生成を失敗させます。

## Interfaces and Dependencies

実装後に満たすべきインターフェースをここで固定します。名前と責務は変更しない前提で進めます。

`cli/internal/usecase/deploy` に次を定義します。

    type ArtifactManifest struct {
        SchemaVersion string
        Project       string
        Env           string
        Mode          string
        Artifacts     []ArtifactEntry
    }

    type ArtifactEntry struct {
        ID                string
        ArtifactRoot      string
        RuntimeConfigDir  string
        BundleManifest    string
        ImagePrewarm      string
        RequiredSecretEnv []string
        SourceTemplate    ArtifactTemplate
        RuntimeMeta       *ArtifactRuntimeMeta // optional
    }

    type ArtifactTemplate struct {
        Path       string
        Sha256     string
        Parameters map[string]string
    }

    type GenerateResult struct {
        ArtifactPath  string
        ArtifactCount int
    }

    type ArtifactGenerator interface {
        Generate(req Request) (GenerateResult, error)
    }

    type ArtifactApplier interface {
        Apply(req ApplyRequest) error
    }

artifact entry には runtime 分離互換のため次を追加します。

    type ArtifactRuntimeMeta struct {
        Hooks    RuntimeHooksMeta `yaml:"runtime_hooks"`
        Renderer RendererMeta     `yaml:"template_renderer"`
    }

    type RuntimeHooksMeta struct {
        APIVersion                string `yaml:"api_version"` // compatibility contract
        PythonSitecustomizeDigest string `yaml:"python_sitecustomize_digest,omitempty"`
        JavaAgentDigest           string `yaml:"java_agent_digest,omitempty"`
        JavaWrapperDigest         string `yaml:"java_wrapper_digest,omitempty"`
    }

    type RendererMeta struct {
        Name           string `yaml:"name"`                     // e.g. "esb-cli-embedded-templates"
        APIVersion     string `yaml:"api_version"`              // compatibility contract
        TemplateDigest string `yaml:"template_digest,omitempty"` // provenance only
    }

    type ApplyRequest struct {
        ArtifactPath   string
        ComposeProject string
        Mode           string
        NoDeps         bool
        Verbose        bool
        ComposeFiles   []string
        SecretEnvPath  string // external secrets file path; required when RequiredSecretEnv is non-empty
        Strict         bool   // enable digest/checksum hard-fail validation
    }

`api_version` の運用規約を固定します。互換判定は `major.minor` で行い、`major` 不一致は hard fail、`minor` 不一致は warning（strict 時 hard fail）です。`major` は破壊的変更時のみ更新、`minor` は後方互換の拡張時に更新します。更新責任は manifest reader/writer 双方を持つ CLI 側メンテナが負います。

パス解決規約を固定します。`artifact_root` が relative の場合のみ `artifact.yml` 所在ディレクトリ基準で解決し、`runtime_config_dir` / `bundle_manifest` など entry 内の実行パスは必ず `artifact_root` 基準で解決します。実行時の cwd には依存しません。

`canonical_template_ref` 正規化規約を固定します。入力は `source_template.path` の記録文字列とし、(1) trim、(2) `\` を `/` へ置換、(3) lexical clean（`path.Clean` 相当、filesystem 非依存）、(4) 連続 `/` の畳み込み、(5) 絶対/相対の区別維持、(6) 文字大小は不変、の順に正規化します。これを `id` 再計算の唯一アルゴリズムとします。

依存ルールは次の通りです。`command` 層は manifest の内容を直接解釈しないで usecase へ渡すだけにします。`usecase` 層は compose 実行の具体実装を持たず既存 interface を再利用します。`infra` 層は manifest I/O、version 互換判定、任意の checksum/digest 検証、ファイル同期、apply 状態記録を担当し、`services/*` は変更不要を原則とします。

ツール責務は次で固定します。`tools/artifactctl` が validate-id/merge/apply の正本実装を持ち、`tools/artifact/merge_runtime_config.sh` は `artifactctl merge` を呼ぶ薄いラッパに限定します。`esb artifact apply` も同一 Go 実装を呼ぶアダプタに限定し、独自の merge/apply ロジックを持ちません。

Artifact Engine の配置方針を固定します。Go 正本は `tools/artifactctl/` 配下（`cmd/artifactctl` + `pkg/engine`）に置き、CLI の `internal/usecase/deploy` は engine のアダプタ層に縮退させます。将来 repo 分離時は core repo が engine モジュールを保有し、CLI repo はそのモジュールを参照して同一実装をリンクします。逆依存（core が CLI を import）は禁止します。

CLI 実行スコープ制約のため `command` 層に次の小さな責務を追加します。`isRepoScopeExempt(args)`（`version/help` 判定）と `validateRepoScope(repoResolver)`（repo root 解決 + 統一エラー化）です。これらは app entry 配下に閉じ、deploy usecase へは漏らしません。

CLI 名称固定化のため `command` 層から `CLI_CMD` 依存を撤去します。具体的には usage/hint 表示名を返す helper を `esb` 定数化し、既存 `branding` テストは「`CLI_CMD` が設定されても `esb` を表示する」観点へ置換します。`Deploy`/`Artifact` の機能ロジックはこの変更の影響を受けません。

`meta` 廃止後の依存ルールを追加します。`cli/go.mod` と `services/agent/go.mod` から `github.com/poruru/edge-serverless-box/meta` を削除し、`replace ../meta` を撤去します。`meta/` ディレクトリ自体は移行完了後に削除し、必要なら `docs/runtime-identity-contract.md` に契約値一覧（env prefix、label prefix、runtime namespace、CA path）を残します。

サービス側へ次の interface を追加して固定します。`services/agent/internal/identity` に `type StackIdentity struct { BrandSlug, Source string }` を置き、`ResolveStackIdentityFrom(...) (StackIdentity, error)` が compose 文脈から 1 回解決します。`PROJECT_NAME` / `ENV` など必須入力が不足する場合は error を返し、起動を hard fail させます。派生値（`RuntimeNamespace()`、`RuntimeCNIName()`、`ImagePrefix()`、`LabelPrefix()` など）はメソッドとして提供し、`main.go` で runtime へ注入します。`docker` / `containerd` 実装は `meta` ではなく注入済み identity を参照します。

## Change Note

2026-02-17: 初版作成。ユーザー要求（`esb deploy` 成果物を残し CLI 非依存で動作させる）に合わせ、責務分離境界、UX 仕様、段階的移行手順を自己完結で実装可能な粒度にした。
2026-02-17: runtime 分離方針を追記。実行時フック（monkey patch / JavaAgent / wrapper）は core 契約、`runtime/*/templates` は CLI 契約として repo 分離可能な境界を明示した。
2026-02-17: 互換判定ポリシーを更新。digest 必須運用は避け、api_version を主判定、digest は strict モード時のみ hard-fail 対象にした。
2026-02-17: 第2回レビュー反映。手動運用の必須環境変数、E2E deploy_driver の具体実装箇所、api_version 運用規約、strict フラグ仕様、apply 中断状態記録を追加した。
2026-02-17: `compose.env` を見直し、機密同梱を禁止。非機密のみ同梱し、機密は成果物外注入（`required_secret_env` で検証）へ変更した。
2026-02-17: `compose.env` の運用を強化。allowlist 生成、secret 混入 hard-fail、`artifact apply --secret-env` 標準化、secret 同梱防止テストを追加した。
2026-02-17: CLI 実行スコープを明確化。repo 外 CWD では `deploy/artifact` を禁止し、`version/help` のみ許可する方針と検証条件を追加した。
2026-02-17: CLI 実行スコープの設計詳細を追加。fail-closed 方針、判定順序、help 判定対象、app entry での責務分離、単体テスト行列を確定した。
2026-02-17: CLI 名称固定化方針を追加。`CLI_CMD` によるコマンド名変更を廃止し、`esb` 固定にする境界・テスト・受け入れ条件を確定した。
2026-02-17: `meta` 共有モジュール廃止方針を追加。module-local 定数への移行、依存削除条件、受け入れ基準を確定した。
2026-02-17: サービス側ブランド解決方針を追加。compose stack 文脈から stack identity を導出し、brand 変更時も動作継続できる設計を確定した。
2026-02-17: 推奨順序フォーカス版へ再編。`CLI固定 -> repoスコープ -> service identity -> meta撤去 -> artifact-first` の優先実施順と PR slicing を確定した。
2026-02-17: Phase 4 完了として `meta.*` 記述を agent ドキュメントから除去し、`docs/runtime-identity-contract.md` を追加。実装済み StackIdentity 契約に文書を一致させた。
2026-02-17: Phase 5 の初手として `docs/deploy-artifact-contract.md` と manifest I/O 基盤（validate/read/write/resolve）を追加し、artifact contract 実装の足場を作成した。
2026-02-18: あるべき論レビューを反映し、実装順序を `contract freeze -> engine -> adapters -> hardening -> e2e gate -> cleanup` に再編。Go 実装単一正本、フォールバック最小化、full E2E 必須化を受け入れ条件へ昇格した。
