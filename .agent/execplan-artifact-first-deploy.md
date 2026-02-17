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
- [x] (2026-02-17 20:00Z) Phase 5 の先行着手として `docs/deploy-artifact-contract.md` を追加し、`cli/internal/usecase/deploy` に descriptor の型/検証/atomic write/read の基盤を導入した。
- [ ] Milestone 1: 成果物契約（Artifact Contract）をコードとドキュメントで定義し、`deploy` 出力に descriptor を追加する。
- [ ] Milestone 2: `deploy` を Generate フェーズと Apply フェーズに内部分離し、`esb deploy` 互換を維持する。
- [ ] Milestone 3: CLI の明示 UX（`artifact generate` / `artifact apply`）と手動運用 UX を整備する。
- [ ] Milestone 4: CLI 非依存 E2E を追加し、回帰を防止する。

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

## Decision Log

- Decision: 成果物の正本は「descriptor + runtime-config 一式」にする。`staging` は内部キャッシュとして残し、外部契約から切り離す。
  Rationale: 現在の `staging` 解決は `templatePath` と repo 依存が強く、非 CLI 環境で再現しにくい。
  Date/Author: 2026-02-17 / Codex

- Decision: `esb deploy` は後方互換を維持し、内部的には Generate と Apply を順に呼ぶ合成コマンドにする。
  Rationale: 既存利用者の CLI 契約を壊さず、責務分離だけを先に進められる。
  Date/Author: 2026-02-17 / Codex

- Decision: 非 CLI 運用の最小契約は「descriptor + runtime-config + compose 環境変数」で成立させ、証明書生成は deploy 責務に含めない。
  Rationale: 証明書は `setup:certs` 系のプラットフォーム初期化責務であり、テンプレート依存の deploy 成果物と性質が異なる。
  Date/Author: 2026-02-17 / Codex

- Decision: runtime のうち「実行時フック（Python sitecustomize / Java agent / Java wrapper）」は ESB 本体責務に固定し、「関数 Dockerfile テンプレート」は CLI 責務として分離可能にする。
  Rationale: 前者はシステム挙動そのもの（ログ/トレース/SDK パッチ）を規定し、後者は成果物生成器の実装詳細であるため、リポジトリ分割時の変更頻度・責務が異なる。
  Date/Author: 2026-02-17 / Codex

- Decision: 将来の repo 分離は 2-way split（`esb-core` と `esb-cli`）を前提にし、runtime templates は `esb-cli` 側または runtime-pack 配布物へ移す。
  Rationale: 現在 `runtime/templates_embed.go` が CLI バイナリにテンプレートを embed しており、依存方向を CLI -> runtime-templates に保つのが自然である。
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

- Decision: マイグレーションは単一リリースで完了させず、3 段階で出す。Phase A は artifact contract + descriptor、Phase B は deploy 分離 + new CLI UX、Phase C は runner の deploy_driver 分岐と artifact profile CI。
  Rationale: 変更範囲が広く、まとめて投入すると回帰時の切り戻しコストが高いため。
  Date/Author: 2026-02-17 / Codex

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

## Outcomes & Retrospective

現時点では Phase 1-4（CLI 名称固定、repo scope guard、service stack identity、meta 共有モジュール撤去）を実装済みで、分離の前提条件は揃った。次の主要リスクは artifact-first 本体導入時に `staging` 互換を保ちながら責務境界を切り替える移行設計である。

## Context and Orientation

ここで言う「成果物」は、テンプレート解析やビルド結果を、後段の実行環境が直接消費できる形で保存したディレクトリを指します。現行コードでは成果物が二系統に分かれています。1つ目はテンプレート近傍の出力（例: `<template_dir>/.esb/<env>/config`）で、2つ目は `staging` のマージ結果（`<repo_root>/.esb/staging/<project>/<env>/config`）です。実行系コンテナへ同期されるのは後者です。

主要ファイルの読み方は次の通りです。`cli/internal/infra/build/go_builder.go` が生成とビルドを実行し、`cli/internal/infra/build/merge_config_entry.go` が `staging` へマージします。`cli/internal/usecase/deploy/deploy_run.go` は deploy の全体順序を管理し、`cli/internal/usecase/deploy/runtime_config.go` が実行中コンテナまたは volume に設定を同期します。`cli/internal/infra/templategen/bundle_manifest.go` は image 集約用 manifest を出力します。ランタイム側は `services/gateway/config.py` と `services/provisioner/src/main.py` が `/app/runtime-config` を読むだけで、CLI を要求しません。

この計画で定義する責務境界は次の3つです。Artifact Producer は成果物を作る責務、Artifact Applier は成果物を実行環境へ反映する責務、Runtime Consumer は反映済みファイルを読むだけの責務です。`esb` CLI は Producer/Applier のフロントエンドに限定し、Runtime Consumer から完全に分離します。

runtime 分離の観点では、`runtime/*` を次の2系統に分けて扱います。A系統はシステム必須フックで、Python の `sitecustomize.py`、Java の `lambda-java-agent.jar` と `lambda-java-wrapper.jar` のように、実行時のログ/トレース/SDK 挙動を成立させる資産です。B系統は生成器都合の資産で、`runtime/java/templates/dockerfile.tmpl` や `runtime/python/templates/dockerfile.tmpl` のように CLI が Dockerfile を生成するためだけに必要な資産です。将来の repo 分離では A系統を core 側契約として残し、B系統を CLI 側へ寄せることを基本方針にします。

## Milestones

### Milestone 1: Artifact Contract の明文化と descriptor 生成

このマイルストーンでは、成果物を人間とツールの両方が読める契約として固定します。新規ドキュメント `docs/deploy-artifact-contract.md` を追加し、必須ファイル、相対パス規約、チェックサム、バージョン互換ポリシーを定義します。並行して `cli/internal/infra/templategen` か `cli/internal/usecase/deploy` に descriptor writer を追加し、`esb deploy` 実行時に成果物ディレクトリへ `artifact.json` を出力します。

完了時点で、利用者は descriptor を読むだけで「どの runtime-config を、どの project/env/mode に適用すべきか」を判別できます。検証は unit test と descriptor 実ファイル検査で行います。

このマイルストーン内で runtime 分離メタデータも導入します。descriptor に `runtime_hooks` と `template_renderer` の識別情報（name/api_version）を必須で持たせ、digest は任意の証跡情報として保持します。既定判定は api_version 互換で行い、digest 一致チェックは strict モード時のみ有効にします。

### Milestone 2: Generate/Apply の内部責務分離

このマイルストーンでは `cli/internal/usecase/deploy/deploy_run.go` を再構成し、Generate フェーズと Apply フェーズを別メソッドへ分離します。Generate は「テンプレート解析、生成、image build、runtime-config 最終化、descriptor 出力」まで、Apply は「image prewarm、runtime-config 同期、provisioner 実行」だけを担当します。

完了時点で `esb deploy` の外部挙動は維持しつつ、内部ではフェーズを個別に呼び出せます。これにより `artifact apply` を後から追加してもロジック重複が発生しません。検証は既存 deploy テスト群と新規フェーズ単体テストで行います。

### Milestone 3: UX の明示化（CLI UX と手動 UX）

このマイルストーンでは利用者の操作系を仕様化して実装します。CLI UX として `esb artifact generate` と `esb artifact apply` を追加し、`esb deploy` はその合成動作として残します。手動 UX として、descriptor と runtime-config を手書きまたは外部生成して `docker compose` で動かす手順を `docs/artifact-operations.md` に記載します。

完了時点で「CLI がある場合の最短経路」と「CLI がない場合の手動経路」が同じ成果物契約を共有します。CLI は便利機能であり、唯一の実行経路ではない状態になります。

### Milestone 4: CLI 非依存 E2E の追加

このマイルストーンでは、成果物生成後に `esb` を使わないで起動と invoke を行う E2E を追加します。`e2e/scenarios/standard` へ新規シナリオを追加し、`PATH` から `esb` を除外した状態で `docker compose` と HTTP invoke だけで検証します。加えて runner 側に deploy driver の分岐（`cli` / `artifact`）を追加し、matrix で経路を選択可能にします。変更対象は `e2e/runner/models.py`（`Scenario.extra.deploy_driver`）、`e2e/runner/config.py`（matrix 取り込み）、`e2e/runner/deploy.py`（driver 実行分岐）、`e2e/run_tests.py`（`ensure_local_esb_cli` 条件化）です。

完了時点で、将来の変更が「成果物だけで動作する」保証を壊したら CI で検知できます。さらに `esb deploy` の従来経路も並行で検証されるため、移行中の互換性低下を早期検知できます。

## Focused Rollout Order (Priority)

このセクションは「実装着手順」を固定するための優先順です。既存 Milestone はスコープ整理として維持し、着手順はこの順序を優先します。

### Phase 1: CLI 名称固定（`esb` 固定）

目的は CLI 外部契約の先行固定です。`CLI_CMD` によるコマンド名変更を無効化し、usage/help/error の表示名を `esb` で統一します。`version/help` の出力契約もこのフェーズで確定します。

変更対象は `cli/internal/command/branding.go`、`cli/internal/command/app.go`、関連テスト（`branding_test.go` / `app_test.go`）と CLI ドキュメントです。ここでは deploy/artifact の機能追加は行わず、名称契約の固定だけに集中します。

完了条件は次の 2 点です。第一に `CLI_CMD=acme` を設定しても CLI 表示名は常に `esb` であること。第二に既存 deploy コマンドの実行機能に回帰がないこと。

### Phase 2: CLI 実行スコープ制約（repo 外制御）

目的は実行境界の固定です。`version/help` だけを repo 外許可し、それ以外のコマンドを repo 外で fail-fast にします。判定は app entry の早期判定で一元化します。

変更対象は `cli/internal/command/app.go` と unit test（RepoScope 系）です。終了コード・エラーメッセージの契約をこのフェーズで固定し、後続フェーズでは変更しません。

完了条件は次の 2 点です。第一に repo 外で `esb version` / `esb --help` が成功すること。第二に repo 外で `deploy` / `artifact` が終了コード 2 で失敗すること。

### Phase 3: サービス側 Runtime Identity 導入（meta 依存を置換）

目的は `meta` 廃止の前提整備です。サービス（特に Agent）が実行時にブランドを解決できる `StackIdentity Resolver` を導入し、固定定数依存を注入型へ置換します。

変更対象は `services/agent/internal/...`（identity resolver, runtime, cni, image naming）と compose 環境注入（`PROJECT_NAME`, `ENV`）です。ここでは `meta` ディレクトリ削除はまだ行わず、二重運用期間を許容して安全に移行します。

完了条件は次の 2 点です。第一に `PROJECT_NAME`/`ENV` または `CONTAINERS_NETWORK` から `brand` が導出されること。第二に導出結果が container 名・label・namespace・CNI 名に一貫反映されること。

### Phase 4: `meta` 共有モジュール撤去

目的はモジュール分離の障害となる共有依存の除去です。`cli` と `services/agent` の `meta` import を完全撤去し、`go.mod` の `replace ../meta` を削除します。必要定数は module-local へ移設します。

変更対象は `cli/go.mod`、`services/agent/go.mod`、`meta/` 参照コード全体、関連テストです。`meta/` ディレクトリは参照ゼロ確認後に削除します。

完了条件は次の 3 点です。第一に `rg "github.com/poruru/edge-serverless-box/meta"` が `cli` / `services/agent` で 0 件であること。第二に CLI/Agent テストが pass すること。第三に stack identity 契約ドキュメントが更新されていること。

### Phase 5: Artifact-first 本体（既存 Milestone 1-4）

目的は本来の artifact-first 実装（descriptor、generate/apply 分離、artifact UX、CLI 非依存 E2E）です。ここから既存 Milestone 1-4 を順次実装します。

前提条件は Phase 1-4 が完了していることです。これにより CLI 契約・実行境界・サービス identity が固定された状態で artifact-first に集中できます。

### Phase 6: CLI Legacy クリーンアップ（最終）

目的は暫定互換コードの除去です。Phase 1-5 完了後に、CLI 名称固定のためだけに残した shim/補助ファイルを整理し、不要コードを削除します。

変更対象は `cli/internal/command/branding.go`、`cli/internal/command/branding_test.go` を第一候補とし、同等責務を `app.go` 側へ内包できる場合は削除します。併せて「`CLI_CMD` が CLI コマンド名を変える」という誤読を招く記述を docs/tests から除去します。

完了条件は次の 3 点です。第一に CLI 名称契約（常に `esb`）がテストで維持されること。第二に `branding.go` 系が不要なら削除されていること。第三に cleanup 後も `go test ./cli/internal/command ./cli/internal/app -count=1` が pass すること。

## PR Slicing Policy

上記フェーズは 1 フェーズ 1 PR を基本にします。PR は「契約固定系（Phase 1/2）」と「依存分離系（Phase 3/4）」と「機能追加系（Phase 5）」を混在させません。各 PR は単独で revert 可能に保ちます。

推奨 PR 順は次です。`PR-1: cli-name-fixed`、`PR-2: cli-repo-scope-guard`、`PR-3: agent-runtime-identity-resolver`、`PR-4: remove-meta-module`、`PR-5+: artifact-first milestones`、`PR-final: cli-legacy-cleanup`。

## Plan of Work

まず、成果物契約をドキュメント先行で固定します。新規 `docs/deploy-artifact-contract.md` に required/optional ファイルを定義します。必須は `artifact.json` と `runtime-config/functions.yml` と `runtime-config/routing.yml` です。`resources.yml` と `image-import.json` と `bundle/manifest.json` は条件付き必須にします。descriptor には、schema version、生成時刻、template 入力の識別情報、project/env/mode、runtime-config 相対パス、image prewarm 方針、compose 適用ヒント、runtime/renderer の api_version を含めます。digest は任意の provenance 情報として扱い、既定では fail 条件にしません。パスは絶対パス禁止にして、成果物ディレクトリごと移動しても再利用できるようにします。

同時に runtime 資産を二分します。`runtime/python/extensions/*` と `runtime/java/extensions/*` は「runtime hooks 契約」として扱い、core 側から参照される前提に固定します。`runtime/*/templates/*.tmpl` は「renderer 契約」として CLI 側へ寄せ、`runtime/templates_embed.go` の責務を CLI パッケージ内へ移す準備を行います。移行期間は互換のため現行パスを残しつつ、内部 import 経路だけを `cli/internal/...` 側に寄せます。

次に、`deploy` usecase を分割します。現在の `Workflow.Run` は一連で実行されるため、`runGeneratePhase` と `runApplyPhase` に分けます。`runGeneratePhase` は `prepareBuildPhase` から `emitPostBuildSummary` までを担当し、最後に descriptor を出力します。`runApplyPhase` は descriptor か明示入力を受け取り、`runImagePrewarm`、`syncRuntimeConfig`、`runProvisioner` を実施します。`syncRuntimeConfig` は `templatePath` ベースの staging 解決依存を減らし、descriptor が指定する runtime-config パスを優先するよう更新します。

その後、CLI 面を拡張します。`cli/internal/command/app.go` に `artifact` サブコマンドを追加し、`generate` と `apply` を定義します。`deploy` 既存フラグは維持し、内部で同じ usecase を呼ぶだけにします。表示 UX は「出力先」「descriptor path」「適用対象 compose project」「build/apply どちらを実行したか」を必ず明示します。

最後に、運用ドキュメントと E2E を揃えます。`docs/environment-variables.md`、`docs/spec.md`、`cli/docs/architecture.md` を更新して責務境界を一致させます。E2E は次の 2 経路を回帰テスト化します。A 経路は `deploy_driver=cli`（従来互換確認）、B 経路は `deploy_driver=artifact`（CLI 非依存確認）です。runner は `deploy_driver=artifact` のとき `ensure_local_esb_cli` をスキップし、deploy フェーズで `artifact apply` または手動 compose 経路を実行します。`e2e/environments/test_matrix.yaml` に `e2e-docker-artifact` を追加し、`deploy_driver: artifact` を明示します。

## UX Specification

標準 UX は次の3操作です。`esb artifact generate` は成果物を作るだけで、稼働中コンテナの状態を変更しません。`esb artifact apply` は既存成果物を対象に prewarm/sync/provisioner を実行します。`esb deploy` は generate と apply を連続実行するショートカットとして扱います。

手動 UX は CLI を前提にしません。運用者は `artifact.json` と `runtime-config` ファイル群を用意し、`CONFIG_DIR` と `PROJECT_NAME` と `CERT_DIR` を指定して `docker compose` を起動します。provisioner が必要な場合は `docker compose --profile deploy run --rm provisioner` を実行します。この経路ではテンプレート解析や Go 実装に依存しません。

`compose.env` は非機密値のみを保持します。機密情報は `secrets.env`（ローカル管理・配布対象外）または CI secrets から注入し、artifact には同梱しません。descriptor には `required_secret_env` を持たせ、`artifact apply --secret-env <path>` を標準経路にします。未設定の必須機密がある場合はキー名だけを列挙して即時エラーにし、値はログに出しません。

`compose.env` 生成は allowlist ベースで実装します。allowed は `PROJECT_NAME`、`CONFIG_DIR`、`CERT_DIR`、`MODE` と、明示的に「non-secret」と分類されたキーのみです。denylist（`*_KEY`、`*_SECRET`、`*_TOKEN`、`AUTH_PASS` など）に一致するキーが入った場合は generate/apply とも hard fail にします。

UX 上のエラーポリシーは次で統一します。descriptor 不整合のうち必須ファイル欠落と schema major 非互換は hard fail にします。digest/checksum 不一致は既定では warning とし、`strict` モード時のみ hard fail にします。稼働 stack 未検出は warning で継続し、provisioner 実行失敗は hard fail にします。

`strict` モードの UX は明示フラグで提供します。`esb artifact apply --strict` を追加し、CI では strict を標準、ローカル運用では非 strict を標準にします。

CLI 実行スコープの UX は次で固定します。`esb version`、`esb help`、`esb --help`、`esb <subcommand> --help` は CWD に依存せず実行可能です。それ以外（`deploy`、`artifact generate`、`artifact apply` など）は CWD から repo root 解決できない場合に即時エラーにします。エラーメッセージは「EBS repository root not found from current directory. Run this command inside the EBS repository.」を基準文言にし、終了コードは 2 を使用します。

CLI 実行スコープ制約の実装ポリシーを固定します。fail-closed を採用し、`version/help` 以外はデフォルトで repo 必須にします。つまり新規サブコマンドを追加した場合、明示的に「repo 不要」と定義しない限り repo 外実行は不可です。これにより将来コマンド追加時のセキュリティ/運用漏れを防ぎます。

CLI 実行スコープ制約の判定順は次で固定します。第一に「引数なし」「version」「help 系フラグ/サブコマンド」を判定します。第二に exempt でなければ `ResolveRepoRoot("")` を 1 回だけ実行します。第三に成功時のみコマンド本体へ進みます。第四に失敗時は統一エラーと終了コード 2 を返します。`deploy` 側で二重に repo 解決しても挙動差が出ないよう、最終的には app entry での早期判定を正本にします。

help 判定対象は次で固定します。`esb --help`、`esb -h`、`esb help`、`esb <subcommand> --help`、`esb <subcommand> -h`。ここでの目的は「repo 外でもヘルプ表示まで到達すること」であり、`<subcommand>` 自体が未知の場合は既存 parser エラー挙動を維持します。

CLI 名称の扱いは次で固定します。ユーザー向けコマンド名、usage 表示、エラーヒントは常に `esb` を使います。`CLI_CMD` はコマンド名切替用途では使用しません。`esb-branding-tool` の書き換え対象から CLI コマンド名を外し、下流差分は runtime 名称空間（例: image prefix / env prefix / data dir）側だけで吸収します。

`meta` 廃止後の定数配置は次を基準にします。CLI が参照する値（`OutputDir`、`HomeDir`、`ImagePrefix`、`LabelPrefix`、`RootCA*` など）は `cli/internal/...` 配下へ集約し、Agent が参照する runtime 値（`RuntimeNamespace`、`RuntimeCNIName`、`RuntimeLabel*` など）は `services/agent/internal/...` 配下へ集約します。互いのモジュールを import しないことをルール化します。

サービス側ブランド解決の UX/契約は次で固定します。サービスは起動時に `StackIdentity` を解決し、以降のラベル生成・namespace 決定・イメージ名生成に使用します。解決順序は 1) `ESB_BRAND_SLUG` 明示値、2) `PROJECT_NAME` と `ENV` からの導出（`<brand>-<env>` 形式）、3) `CONTAINERS_NETWORK` から `-external` を除去して導出、4) fallback `esb`（warning ログ）です。`PROJECT_NAME` と `ENV` は compose からサービスへ必ず注入します。

## Concrete Steps

作業者は常に `/home/akira/esb` で作業します。

1. 契約ドキュメント追加と descriptor 型定義。

    cd /home/akira/esb
    touch docs/deploy-artifact-contract.md
    go test ./cli/internal/usecase/deploy ./cli/internal/infra/templategen -count=1

期待される結果は、descriptor の encode/decode テストが追加され、既存 deploy テストが落ちないことです。

2. deploy フェーズ分離。

    cd /home/akira/esb
    go test ./cli/internal/usecase/deploy -count=1

期待される結果は、`Workflow.Run` が Generate/Apply を順に呼ぶ薄い orchestrator になり、既存の `esb deploy` と同じ外部挙動を保つことです。

3. CLI コマンド追加。

    cd /home/akira/esb/cli
    go test ./internal/command ./internal/app -count=1

期待される結果は、`esb artifact generate --help` と `esb artifact apply --help` が表示され、`--strict` の説明が `artifact apply` に表示されることです。

4. 非 CLI 実行確認（手動経路）。

    cd /home/akira/esb
    set -a; source <artifact-root>/compose.env; source <secret-env-path>; set +a
    PATH="/usr/bin:/bin" docker compose -f docker-compose.docker.yml up -d
    curl -k https://localhost:443/health

期待される結果は、`curl` が HTTP 200 を返し、`esb` バイナリがなくても起動が可能なことです。実際の invoke 検証は E2E シナリオで行います。

5. E2E deploy_driver 追加。

    cd /home/akira/esb
    uv run pytest -q e2e/runner/tests
    uv run python e2e/run_tests.py --profile e2e-docker-artifact --build-only --verbose

期待される結果は、`e2e-docker-artifact` が `ensure_local_esb_cli` に依存せず deploy/test フェーズを進められることです。

6. 機密同梱防止テスト追加。

    cd /home/akira/esb
    go test ./cli/internal/usecase/deploy -run SecretEnv -count=1
    uv run pytest -q e2e/scenarios/standard/test_artifact_runtime.py -k secret_guard

期待される結果は、`compose.env` に secret 系キーが混入したとき generate が失敗し、`--secret-env` 未指定または required 未充足時に apply が失敗することです。

7. CLI 実行スコープ制約テスト追加。

    cd /tmp
    esb version
    esb --help
    esb deploy --template /tmp/any.yaml --env dev --mode docker

    cd /home/akira/esb/cli
    go test ./internal/command -run RepoScope -count=1

期待される結果は、`version/help` は成功し、`deploy` は「repo root not found」エラーで終了コード 2 を返すことです。

8. CLI 実行スコープの単体テスト行列追加（app entry）。

    cd /home/akira/esb/cli
    go test ./internal/command -run RepoScope -count=1

期待される結果は次を満たすことです。`Run(["version"])` は repo resolver を呼ばず 0 終了、`Run(["--help"])` は repo resolver を呼ばず 0 終了、`Run(["deploy", ...])` は repo resolver 失敗時に終了コード 2、`Run(["artifact","apply",...])` も同様、`Run(["deploy","--help"])` は repo resolver を呼ばず help 表示まで到達すること。

9. CLI 名称固定化テスト追加。

    cd /home/akira/esb/cli
    CLI_CMD=acme go test ./internal/command -run "Branding|RepoScope|Version" -count=1

期待される結果は、`CLI_CMD=acme` を与えても usage/help/version のコマンド表示は `esb` のままであること、`acme` という別名コマンドを前提にした分岐が存在しないことです。

10. `meta` 共有モジュール撤去計画テスト追加。

    cd /home/akira/esb
    rg -n "github.com/poruru/edge-serverless-box/meta" cli services/agent

    cd /home/akira/esb/cli
    go test ./... -count=1

    cd /home/akira/esb/services/agent
    go test ./... -count=1

期待される結果は、`cli` / `services/agent` から `meta` import が消え、`go.mod` の `replace ../meta` も不要になることです。さらに双方のテストが pass し、ラベル名・namespace・画像名生成などの契約が維持されることです。

11. サービス側ブランド resolver テスト追加。

    cd /home/akira/esb/services/agent
    go test ./... -run "StackIdentity|BrandResolver|ImageNaming|RuntimeLabel" -count=1

期待される結果は、`PROJECT_NAME=acme-dev` `ENV=dev` から `brand=acme` が導出されること、`CONTAINERS_NETWORK=acme-dev-external` でも同様に導出できること、導出値が container 名・label key・namespace・CNI 名へ一貫適用されることです。

## Validation and Acceptance

受け入れ条件は次の9点です。

第一に、`esb deploy` の既存ユースケースが回帰しないことです。`deploy` 既存テストと E2E smoke を通します。第二に、`artifact generate` で出た成果物を `artifact apply` で反映できることです。第三に、同じ成果物を使って CLI なし（`esb` 実行なし）でも compose 起動と provisioner 実行ができることです。第四に、descriptor を手動作成した最小成果物でも Gateway 起動と config 読み込みが成立することです。第五に、artifact には機密値が含まれず、必須機密未設定時に apply が失敗することです。第六に、CLI は repo 外 CWD で `deploy/artifact` を拒否し、`version/help` のみ実行可能であることです。第七に、CLI コマンド名は常に `esb` で、`CLI_CMD` による名称変更ができないことです。第八に、`meta` 共有モジュールが撤去され、CLI/Agent が module-local 定数で自己完結していることです。第九に、サービスは compose stack 文脈からブランドを解決し、ブランド変更時も runtime 識別子が一貫することです。

具体的には次を実行します。

    cd /home/akira/esb/cli
    go test ./... -count=1

    cd /home/akira/esb
    X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run pytest -q e2e/runner/tests

    cd /home/akira/esb
    uv run python e2e/run_tests.py --profile e2e-docker --test-target e2e/scenarios/standard/test_artifact_runtime.py --verbose

    cd /home/akira/esb
    uv run python e2e/run_tests.py --profile e2e-docker-artifact --test-target e2e/scenarios/standard/test_artifact_runtime.py --verbose

    cd /home/akira/esb
    rg -n "JWT_SECRET_KEY=|X_API_KEY=|AUTH_PASS=|RUSTFS_SECRET_KEY=" <artifact-root>/compose.env && exit 1 || true

    cd /tmp
    esb version
    esb --help
    esb deploy --template /tmp/any.yaml --env dev --mode docker || test $? -eq 2

    cd /home/akira/esb/cli
    CLI_CMD=acme go test ./internal/command -run Branding -count=1

    cd /home/akira/esb
    rg -n "github.com/poruru/edge-serverless-box/meta" cli services/agent && exit 1 || true

    cd /home/akira/esb/services/agent
    PROJECT_NAME=acme-dev ENV=dev go test ./... -run "StackIdentity|BrandResolver" -count=1

成功判定は、次の 7 条件を同時に満たすことです。第一に従来 profile（CLI deploy 経路）が pass すること。第二に artifact profile（CLI 非依存経路）が pass し、`test_artifact_runtime.py` が `esb` 実行なしで invoke 成功まで到達することです。第三に artifact の `compose.env` へ secret 系キーが出力されないことです。第四に repo 外 CWD で `version/help` は成功し、`deploy/artifact` は失敗することです。第五に `CLI_CMD` を変更しても CLI 表示名と実行コマンド名は `esb` 固定であることです。第六に `meta` import が CLI/Agent から消え、module-local 定数実装へ移行していることです。第七にサービスが compose stack 文脈からブランドを解決し、brand 変更時もラベル/namespace/CNI 名が破綻しないことです。

artifact profile の追加定義は次を満たすことを必須条件にします。`e2e/environments/test_matrix.yaml` に `esb_env: e2e-docker-artifact` を追加し、`deploy_driver: artifact`、`deploy_templates`、必要な `env_vars` を明示すること。

## Idempotence and Recovery

`artifact generate` は同一入力に対して上書き再実行可能にします。descriptor は atomic write（temp file から rename）で書き込み、途中失敗時は壊れたファイルを残しません。`artifact apply` は複数回実行しても同じ最終状態になるようにし、既存の `copyFile` 原子的更新と provisioner の冪等動作（既存リソース skip）を利用します。

失敗時の復旧は単純に再実行できる形にします。descriptor validation 失敗時は何も反映しない早期 abort、runtime 同期失敗時は同期済みファイルの差分をログへ出し、修正後に再実行できるようにします。

Apply は「同期」と「provision」を分離して記録し、`artifact-apply.state.json` に直近状態を書き出します。`sync_done=true/provision_done=false` の中断ケースを明示できるようにし、再実行時にどこからやり直したかを出力します。ロールバックは自動では行わず、状態を可視化して安全再試行を優先します。

## Artifacts and Notes

成果物の想定レイアウト（v1）は次です。

    <artifact-root>/
      artifact.json
      runtime-config/
        functions.yml
        routing.yml
        resources.yml
        image-import.json
      bundle/
        manifest.json
      compose.env
      compose.secrets.env.example

`artifact.json` の最小イメージは次です。

    {
      "schema_version": "1",
      "project": "esb-dev",
      "env": "dev",
      "mode": "docker",
      "runtime_config_dir": "runtime-config",
      "bundle_manifest": "bundle/manifest.json",
      "image_prewarm": "all"
    }

`compose.env` は CLI なし起動のために非機密のみを持ちます。必須は `PROJECT_NAME`、`CONFIG_DIR`、`CERT_DIR`、必要に応じて `MODE` です。`compose.secrets.env.example` は必須機密キーのテンプレートのみを持ち、実値は含めません。`compose.env` は allowlist 生成とし、secret 系キーが入った場合は artifact 生成を失敗させます。

## Interfaces and Dependencies

実装後に満たすべきインターフェースをここで固定します。名前と責務は変更しない前提で進めます。

`cli/internal/usecase/deploy` に次を定義します。

    type ArtifactDescriptor struct {
        SchemaVersion    string
        Project          string
        Env              string
        Mode             string
        RuntimeConfigDir string
        BundleManifest   string
        ImagePrewarm     string
        RequiredSecretEnv []string
        Templates        []ArtifactTemplate
    }

    type ArtifactTemplate struct {
        Path       string
        Sha256     string
        Parameters map[string]string
    }

    type GenerateResult struct {
        DescriptorPath   string
        RuntimeConfigDir string
        BundleManifest   string
    }

    type ArtifactGenerator interface {
        Generate(req Request) (GenerateResult, error)
    }

    type ArtifactApplier interface {
        Apply(req ApplyRequest) error
    }

descriptor には runtime 分離互換のため次を追加します。

    type ArtifactRuntimeMeta struct {
        Hooks RuntimeHooksMeta   `json:"runtime_hooks"`
        Renderer RendererMeta    `json:"template_renderer"`
    }

    type RuntimeHooksMeta struct {
        APIVersion                string `json:"api_version"` // compatibility contract
        PythonSitecustomizeDigest string `json:"python_sitecustomize_digest,omitempty"`
        JavaAgentDigest           string `json:"java_agent_digest,omitempty"`
        JavaWrapperDigest         string `json:"java_wrapper_digest,omitempty"`
    }

    type RendererMeta struct {
        Name          string `json:"name"`        // e.g. "esb-cli-embedded-templates"
        APIVersion    string `json:"api_version"` // compatibility contract
        TemplateDigest string `json:"template_digest,omitempty"` // provenance only
    }

    type ApplyRequest struct {
        DescriptorPath string
        ComposeProject string
        Mode           string
        NoDeps         bool
        Verbose        bool
        ComposeFiles   []string
        SecretEnvPath  string // external secrets file path; required when RequiredSecretEnv is non-empty
        Strict         bool // enable digest/checksum hard-fail validation
    }

`api_version` の運用規約を固定します。互換判定は `major.minor` で行い、`major` 不一致は hard fail、`minor` 不一致は warning（strict 時 hard fail）です。`major` は破壊的変更時のみ更新、`minor` は後方互換の拡張時に更新します。更新責任は descriptor reader/writer 双方を持つ CLI 側メンテナが負います。

パス解決規約を固定します。descriptor 内の相対パスは必ず「descriptor ファイル所在ディレクトリ基準」で解決し、実行時の cwd には依存しません。これにより artifact ディレクトリ移動後も同一挙動を保証します。

依存ルールは次の通りです。`command` 層は descriptor の内容を直接解釈しないで usecase へ渡すだけにします。`usecase` 層は compose 実行の具体実装を持たず既存 interface を再利用します。`infra` 層は descriptor I/O、version 互換判定、任意の checksum/digest 検証、ファイル同期、apply 状態記録を担当し、`services/*` は変更不要を原則とします。

CLI 実行スコープ制約のため `command` 層に次の小さな責務を追加します。`isRepoScopeExempt(args)`（`version/help` 判定）と `validateRepoScope(repoResolver)`（repo root 解決 + 統一エラー化）です。これらは app entry 配下に閉じ、deploy usecase へは漏らしません。

CLI 名称固定化のため `command` 層から `CLI_CMD` 依存を撤去します。具体的には usage/hint 表示名を返す helper を `esb` 定数化し、既存 `branding` テストは「`CLI_CMD` が設定されても `esb` を表示する」観点へ置換します。`Deploy`/`Artifact` の機能ロジックはこの変更の影響を受けません。

`meta` 廃止後の依存ルールを追加します。`cli/go.mod` と `services/agent/go.mod` から `github.com/poruru/edge-serverless-box/meta` を削除し、`replace ../meta` を撤去します。`meta/` ディレクトリ自体は移行完了後に削除し、必要なら `docs/runtime-identity-contract.md` に契約値一覧（env prefix、label prefix、runtime namespace、CA path）を残します。

サービス側へ次の interface を追加して固定します。`services/agent/internal/identity` に `type StackIdentity struct { BrandSlug, Source string }` を置き、`ResolveStackIdentityFrom(...)` が compose 文脈から 1 回解決します。派生値（`RuntimeNamespace()`、`RuntimeCNIName()`、`ImagePrefix()`、`LabelPrefix()` など）はメソッドとして提供し、`main.go` で runtime へ注入します。`docker` / `containerd` 実装は `meta` ではなく注入済み identity を参照します。

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
2026-02-17: Phase 5 の初手として `docs/deploy-artifact-contract.md` と descriptor I/O 基盤（validate/read/write/resolve）を追加し、artifact contract 実装の足場を作成した。
