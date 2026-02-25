# CONFIG_DIR 即時廃止と runtime-config named volume 固定化

This ExecPlan is a living document. The sections `Progress`, `Surprises & Discoveries`, `Decision Log`, and `Outcomes & Retrospective` must be kept up to date as work proceeds.

本計画は `/home/akira/esb3/.agent/PLANS.md` に従って維持します。テンプレート正本は `/home/akira/esb-branding-tool/tools/branding/templates/` にあるため、実装は `esb3` と `esb-branding-tool` を跨いで行います。

## Purpose / Big Picture

`CONFIG_DIR` を公開仕様・実装仕様から即時削除し、`docker compose up -d` 後の runtime-config を常に `esb-runtime-config` named volume に固定します。これにより、利用者は runtime-config のパス指定を一切せずに `compose up` と `esb deploy`（このリポジトリでは `artifactctl deploy` を含む deploy/apply 経路）を実行できる状態にします。完了後は `docker inspect` で `/app/runtime-config` の mount が named volume であること、deploy 後に `functions.yml/routing.yml/resources.yml` が更新されることを確認します。

## Progress

- [x] (2026-02-24 19:32Z) 影響範囲調査を実施し、`CONFIG_DIR` が compose・E2E runner・docs・artifactctl CLI に跨っていることを確認。
- [x] (2026-02-25 04:42Z) `esb-branding-tool` の docker/containerd compose template から `CONFIG_DIR` 参照を撤去し、runtime-config を named volume 固定化。
- [x] (2026-02-25 04:42Z) `esb3` の生成済み compose を再生成/反映し、runtime-config 関連の `${CONFIG_DIR:-...}` を完全除去。
- [x] (2026-02-25 05:19Z) deploy/apply 経路を `CONFIG_DIR` 非依存へ変更（staging -> runtime-config volume 同期を内部実装）。
- [x] (2026-02-25 04:58Z) E2E runner から `CONFIG_DIR` 必須ロジックと matrix の `config_dir` 契約を削除。
- [x] (2026-02-25 04:58Z) docs/README/サンプル/テストから `CONFIG_DIR` 記載を削除し、breaking change を `docs/release-notes.md` に明示。
- [x] (2026-02-25 05:37Z) Go/Python テストを実行し、回帰と DoD を確認（UT/E2E 通過）。

## Surprises & Discoveries

- Observation: `docker-compose.*.yml` は `esb-branding-tool` の template から生成されるが、`esb3` リポジトリ内には template が存在しない。
  Evidence: `docker-compose.docker.yml` ヘッダに `Source: tools/branding/templates/...` とあり、実ファイルは `/home/akira/esb-branding-tool/tools/branding/templates/` に存在。

- Observation: `go test ./...` を `esb3` ルートで直接実行すると `go.work` の都合で失敗する。
  Evidence: `pattern ./...: directory prefix . does not contain modules listed in go.work`。`go.work` に列挙された各 module で個別に `go test ./...` を実行して検証した。

- Observation: runtime-config mount が named volume の場合、`Mounts[].Source` は `/var/lib/docker/volumes/.../_data` になり、直接書き込みは権限エラーになる。
  Evidence: E2E deploy で `mkdir /var/lib/docker/volumes: permission denied` が発生。同期処理を volume attach 方式（`docker run -v <volume>:/runtime-config`）へ変更して解消。

- Observation: `~/.local/bin/artifactctl` の古い binary を参照すると `--out` 必須の旧仕様で E2E が失敗する。
  Evidence: E2E log に `Error: missing flags: --out=STRING`。最新 binary を再ビルド/配置して解消。

## Decision Log

- Decision: 互換レイヤ・警告運用は実装しない。`CONFIG_DIR` は即時撤去する。
  Rationale: ユーザーから「段階移行なし・後方互換なし」の明示指示があったため。
  Date/Author: 2026-02-24 / Codex

- Decision: compose template の正本を `esb-branding-tool` で修正し、`esb3` へ再生成反映する。
  Rationale: 生成済みファイルのみを直接編集すると再生成で戻るため、正本から直す必要がある。
  Date/Author: 2026-02-24 / Codex

- Decision: deployops の runtime-config 解決結果を「ディレクトリ文字列」ではなく `RuntimeConfigTarget{BindPath|VolumeName}` で扱う。
  Rationale: bind/volume で同期方式を分岐でき、named volume の権限問題を避けられるため。
  Date/Author: 2026-02-25 / Codex

- Decision: named volume 同期はホストパス書き込みではなく `docker run --rm -v <volume>:/runtime-config -v <staging>:/src:ro alpine sh -c 'cp -a'` を採用する。
  Rationale: Docker 管理ディレクトリへの直接アクセス権限に依存せず、docker mode/containerd mode の両方で同一手順にできるため。
  Date/Author: 2026-02-25 / Codex

- Decision: E2E の既定経路でも新 `artifactctl` が使われるよう `~/.local/bin/artifactctl` を最新 binary で更新する。
  Rationale: `ARTIFACTCTL_BIN` 指定なしの標準運用で再現性を担保するため。
  Date/Author: 2026-02-25 / Codex

## Outcomes & Retrospective

目的どおり、`CONFIG_DIR` は compose・deploy/apply・E2E runner・docs・tests から即時撤去され、runtime-config は named volume 固定へ移行できた。利用者の操作は `docker compose up -d` と `artifactctl deploy --artifact ...`（CLI 経路では `esb deploy`）に一本化され、runtime-config path 指定は不要になった。

初回 E2E では 2 つの環境依存問題（古い artifactctl binary と named volume 直接書き込み権限）が露呈したが、いずれも実装修正と実行バイナリ更新で解消した。最終的に docker/containerd の matrix E2E が標準経路（`ARTIFACTCTL_BIN` 未指定）で全通し、`docker inspect` でも `/app/runtime-config` が volume mount であることを確認した。未完了項目は現時点でなし。

## Context and Orientation

この変更は以下 4 つの層を同時に更新する必要があります。1) compose 定義（docker/containerd）で runtime-config mount を固定する層、2) deploy/apply で artifact を runtime-config に反映する層、3) E2E runner の環境組み立て層、4) 利用者向けドキュメント層です。

runtime-config は gateway/provisioner が読む `functions.yml` `routing.yml` `resources.yml` を保持するディレクトリです。現行では `CONFIG_DIR` が build 時追加コンテキストと runtime mount 指定の両方に使われ、E2E でも `config_dir` を必須入力として扱っています。今回の作業で runtime mount は常に `esb-runtime-config` named volume に固定し、deploy は内部 staging からこの runtime target へ同期します。

主な編集対象:
- `/home/akira/esb-branding-tool/tools/branding/templates/docker-compose.docker.yml.tmpl`
- `/home/akira/esb-branding-tool/tools/branding/templates/docker-compose.containerd.yml.tmpl`
- `/home/akira/esb3/docker-compose.docker.yml`
- `/home/akira/esb3/docker-compose.containerd.yml`
- `/home/akira/esb3/tools/artifactctl/cmd/artifactctl/main.go`
- `/home/akira/esb3/pkg/deployops/*`（deploy input/execute/新規 runtime-config 同期ユーティリティ）
- `/home/akira/esb3/e2e/runner/{config.py,context.py,deploy.py,lifecycle.py,constants.py}`
- `/home/akira/esb3/e2e/runner/tests/*` と `e2e/environments/test_matrix.yaml`
- `/home/akira/esb3/docs/*`, `/home/akira/esb3/README.md`（必要箇所）, リリースノート相当ファイル

## Plan of Work

最初に template 正本を更新します。docker/containerd の compose template で、`/app/runtime-config` と permission-helper の `/data/runtime-config` mount を `esb-runtime-config`（template 側では `{{SLUG}}-runtime-config`）へ固定し、build `additional_contexts.config` を固定パス `services/gateway/seed-config` に変更します。次に branding generator で `esb3` の生成済み compose を再生成し、`CONFIG_DIR` 展開が残っていないことを確認します。

続いて deploy/apply 経路を変更します。`artifactctl deploy` の公開引数から `--out` を撤去し、内部で staging ディレクトリを作って `artifactcore` のマージ結果を出力します。その後、実行中 compose コンテナ（gateway/provisioner）の mount 情報から `/app/runtime-config` のホスト側同期先を解決し、staging から runtime-config volume へ同期します。これにより利用者は config path を指定しません。

E2E runner では matrix `config_dir` と `ENV_CONFIG_DIR` 契約を削除します。deploy フェーズは `artifactctl deploy --artifact ...` のみを呼び出し、`CONFIG_DIR` 依存チェックを撤去します。関連 unit test を更新し、`CONFIG_DIR` 前提の assertion をすべて除去します。

最後に docs/README/運用手順/契約文書を更新します。`CONFIG_DIR` 記載を削除し、breaking change として「`CONFIG_DIR` 廃止・旧運用非サポート」を明記します。

## Concrete Steps

作業ディレクトリは `/home/akira/esb3` を基準にします。

1. template 正本を編集する。
   - `/home/akira/esb-branding-tool/tools/branding/templates/docker-compose.docker.yml.tmpl`
   - `/home/akira/esb-branding-tool/tools/branding/templates/docker-compose.containerd.yml.tmpl`

2. `esb3` へ compose を再生成する。（実施済み）

    cd /home/akira/esb-branding-tool
    uv run python tools/branding/generate.py --root /home/akira/esb3 --brand esb --force

3. `esb3` 側の deploy/apply 実装と E2E runner を修正する。

4. docs と release note を更新し、`CONFIG_DIR` 文言を削除する。

5. `CONFIG_DIR` が残っていないことを検索で確認する。

    cd /home/akira/esb3
    rg -n "CONFIG_DIR" -S

6. テストを実行する。（実施済み）

    cd /home/akira/esb3
    # go.work に列挙された各 module ごとに実行
    for m in $(awk '/^\t\.\//{gsub(/[\t()]/,""); print $1}' go.work); do (cd "$m" && go test ./...); done

    X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy \
      uv run pytest runtime-hooks/python/tests e2e/runner/tests services/common/tests services/gateway/tests services/runtime-node/tests

    X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run e2e/run_tests.py

必要に応じて対象を絞った追加テストを実行する。

## Validation and Acceptance

受け入れ確認は次の観点で行います。

- コード/compose/docs/tests に `CONFIG_DIR` 参照が存在しない。
- `docker-compose.docker.yml` と `docker-compose.containerd.yml` の runtime-config mount が named volume 固定。
- `artifactctl deploy`（deploy/apply 経路）が runtime-config path 指定なしで実行できる。
- E2E runner が matrix `config_dir` なしで実行可能。
- 主要 unit test（deployops/artifactctl/e2e runner）が通過。

環境依存の実機検証（compose up -> deploy -> inspect）は可能な範囲で実施し、未実施なら理由を明記します。

## Idempotence and Recovery

template 変更は再生成コマンドを繰り返し実行しても同じ結果になります。deploy/apply のテストは一時ディレクトリを利用するため再実行可能です。途中失敗時は `git status` で変更点を確認し、該当ステップから再実行します。

## Artifacts and Notes

実施結果:

- `CONFIG_DIR` 参照検索:

    cd /home/akira/esb3 && rg -n "\bCONFIG_DIR\b|\$\{CONFIG_DIR" -S
    # no matches

    cd /home/akira/esb-cli && rg -n "\bCONFIG_DIR\b|EnvConfigDir|HostSuffixConfigDir" -S
    # no matches

    cd /home/akira/esb-branding-tool && rg -n "\bCONFIG_DIR\b|\$\{CONFIG_DIR" -S
    # no matches

- 主要テスト:

    cd /home/akira/esb-cli && go test ./...
    # pass

    cd /home/akira/esb3 && for m in $(awk '/^\t\.\//{gsub(/[\t()]/,""); print $1}' go.work); do (cd "$m" && go test ./...); done
    # pass

    cd /home/akira/esb3 && X_API_KEY=dummy AUTH_USER=dummy AUTH_PASS=dummy uv run e2e/run_tests.py
    # [PASSED] ALL MATRIX ENTRIES PASSED!

- mount 確認（抜粋）:

    docker inspect esb-e2e-docker-gateway --format '{{range .Mounts}}{{if eq .Destination "/app/runtime-config"}}{{.Type}} {{.Name}}{{end}}{{end}}'
    # volume esb-e2e-docker_esb-runtime-config

    docker inspect esb-e2e-containerd-provisioner --format '{{range .Mounts}}{{if eq .Destination "/app/runtime-config"}}{{.Type}} {{.Name}}{{end}}{{end}}'
    # volume esb-e2e-containerd_esb-runtime-config

## Interfaces and Dependencies

最終的に満たす公開インターフェース方針:

- `docker-compose.(docker|containerd).yml` は runtime-config を `esb-runtime-config` named volume へ固定。
- `artifactctl deploy` は runtime-config 出力先を外部引数で受け取らない。
- deploy 実装は内部 staging ディレクトリを使い、compose 実行中コンテナ mount から runtime-config 同期先を解決する。
- E2E matrix は `config_dir` を持たず、runner は `CONFIG_DIR` を参照しない。

更新履歴:
- 2026-02-24: 初版作成。ユーザーの更新指示に合わせ、互換レイヤなしの即時撤去方針を明記。
- 2026-02-25: 実装完了に合わせて `Progress`/`Surprises & Discoveries`/`Decision Log`/`Outcomes & Retrospective` を更新し、検証ログと最終結果を追記。理由: 計画書が実際の実装進捗に追従していなかったため。
