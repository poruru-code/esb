# Buf ベース Proto 生成への移行

この ExecPlan は living document です。作業中は `Progress`、`Surprises & Discoveries`、`Decision Log`、`Outcomes & Retrospective` を必ず更新します。

本ドキュメントはリポジトリルートの `.agent/PLANS.md` に従って維持します。

## Purpose / Big Picture

この変更が完了すると、`services/contracts/proto/agent.proto` の生成手順が `tools/gen_proto.py` + Docker イメージ依存から、`buf` による宣言的な生成に置き換わります。これにより、生成系の責務を `services/contracts` に寄せ、`latest` イメージ依存を排除し、再現性とレビュー容易性を上げます。利用者は `buf generate`（またはそれを呼ぶ単一タスク）で Go/Python 生成物を更新でき、CI で proto 契約の破壊的変更チェックを機械的に確認できます。

## Progress

- [x] (2026-02-19 17:01Z) 現行の proto 生成導線を調査し、`tools/gen_proto.py`・関連 docs・生成先ディレクトリの依存を確認した。
- [x] (2026-02-19 17:01Z) `buf` 公式ドキュメントで v2 設定、`buf generate`、`buf breaking`、Managed Mode の前提を確認した。
- [x] (2026-02-19 17:55Z) `services/contracts` 配下に `buf.yaml` / `buf.gen.yaml` を追加し、`mise run gen-proto` で既存配置へ Go/Python 生成物が出力されることを確認した。
- [x] (2026-02-19 17:55Z) Python import 補正は `services/contracts/scripts/fix_python_grpc_imports.py` に移設し、`services/gateway/pb/__init__.py` まで含めて安定化した。
- [x] (2026-02-19 17:56Z) `tools/gen_proto.py` を削除し、`.mise.toml` の `gen-proto` タスクと関連 docs を `buf` 基準へ統一した。
- [x] (2026-02-19 17:57Z) CI に proto generation + breaking のゲート（`quality-gates.yml` 内 `proto-contract`）を追加した。
- [x] (2026-02-19 17:31Z) クリーン状態からフルE2Eを再実行し、dead code / dangling reference / 追加差分なしを確認した。

## Surprises & Discoveries

- Observation: 現在の生成導線は Go 側で `rvolosatovs/protoc:latest` を使っており、バージョン固定が弱い。
  Evidence: `tools/gen_proto.py` の `gen_go_docker()` で `rvolosatovs/protoc:latest` を直接指定している。

- Observation: Python gRPC 生成物は現在のままでは import が壊れるため、手動置換を入れている。
  Evidence: `tools/gen_proto.py` の `fix_python_imports()` が `import agent_pb2` を `from . import agent_pb2` に置換している。

- Observation: proto 生成の CI ゲート（生成差分検出、破壊的変更検出）は現時点で存在しない。
  Evidence: `.github/workflows/*` に `buf generate` / `buf breaking` 呼び出しがない。

- Observation: クリーンE2Eの初回失敗はコード不具合ではなく、`setup:certs` が Root CA 生成時に sudo 対話失敗し、leaf cert (`server.crt`/`server.key`) 未生成だったことが原因だった。
  Evidence: `esb-e2e-*-agent` / `esb-e2e-*-gateway` ログに `open /app/config/ssl/server.crt: no such file or directory` / `FileNotFoundError`。

## Decision Log

- Decision: `buf` 設定ファイルは `services/contracts` 直下に置く。
  Rationale: proto 契約の所有境界が `services/contracts` なので、設定も同じ境界に置くのが責務分離として自然。`tools` 直下に新しい横断スクリプトを増やさない方針にも一致する。
  Date/Author: 2026-02-19 / Codex

- Decision: 移行は「一発置換」ではなく、最初に出力同等性を確認する段階を設ける。
  Rationale: Python import 形式の差異で Gateway 実行時エラーになるリスクがあるため、生成差分を観測してから削除系へ進む。
  Date/Author: 2026-02-19 / Codex

- Decision: 後方互換目的で `tools/gen_proto.py` を残さない。最終状態では削除する。
  Rationale: 本リポジトリ全体の方針（不要オプション・残コードの削減）と一致。入口を増やさず単一路線にする。
  Date/Author: 2026-02-19 / Codex

- Decision: `buf breaking` は `services/contracts/proto` を比較対象にし、基準ブランチ側に `buf.yaml` が未導入でも比較可能な形を採用する。
  Rationale: develop 側未移行期間でも CI で breaking 検知を有効に保つため。
  Date/Author: 2026-02-19 / Codex

## Outcomes & Retrospective

- 生成導線は `buf` + `mise run gen-proto` の 1 入口へ統一。`tools/gen_proto.py` と旧依存参照は削除できた。
- 再現性: `mise run gen-proto` 実行後の生成差分は空（Go/Python 双方）。
- 品質ゲート: repo layout / tooling boundary チェックは通過。`proto-contract` ジョブを `quality-gates.yml` に追加。
- 検証結果:
  - `go -C services/agent test ./...` PASS
  - `uv run pytest services/gateway/tests -q` PASS (149 passed)
  - `uv run e2e/run_tests.py --parallel --verbose` PASS (`e2e-containerd` 45 passed, `e2e-docker` 53 passed)
- 運用面の注意: 証明書生成は初回 `setup:certs` で sudo 対話が必要になる場合がある。今回のように Root CA だけ存在する中途状態では、再実行で leaf cert を生成できる。

## Context and Orientation

現在の正本は `services/contracts/proto/agent.proto` です。生成物は Go が `services/agent/pkg/api/v1/agent.pb.go` と `services/agent/pkg/api/v1/agent_grpc.pb.go`、Python が `services/gateway/pb/agent_pb2.py` と `services/gateway/pb/agent_pb2_grpc.py` に置かれます。

生成入口は `tools/gen_proto.py` で、Python は `grpc_tools.protoc`、Go は Docker コンテナ内 `protoc` を使っています。Python 生成物はそのままだと package import が合わないため、同スクリプト内で import 文字列を置換しています。関連ドキュメントは `services/agent/docs/proto-generation.md`、`services/agent/docs/README.md`、`services/agent/docs/grpc-api.md`、`services/contracts/README.md`、`docs/repo-layout-contract.md` に分散しています。

今回移行する `buf` は Protocol Buffers の lint / 生成 / 破壊的変更検出を行う CLI です。`buf generate` は `buf.gen.yaml` の定義に従って生成物を出力し、`buf breaking` は基準ブランチとの差分で互換性違反を検出します。

## Plan of Work

最初に `services/contracts` 配下へ `buf.yaml` と `buf.gen.yaml` を追加し、`agent.proto` を入力に Go/Python 両方の生成物を現行と同じ場所へ出す構成を作ります。この段階では `tools/gen_proto.py` はまだ削除せず、`buf generate` 結果と現行成果物を比較して差分の意味を確定します。

次に Python import の互換性を確定します。`buf` プラグイン設定だけで `services/gateway/pb/agent_pb2_grpc.py` が `from . import agent_pb2` になるなら post-process を不要にします。設定だけで満たせない場合は、`services/contracts` 側の最小 post-process（生成物 1 箇所の deterministic 置換）を導入し、`tools` 直下の汎用スクリプト増殖を避けます。

出力同等性が取れたら `tools/gen_proto.py` を削除し、`.mise.toml` に `gen-proto` タスクを追加して実行入口を一本化します。ドキュメントはすべて `buf` 手順へ更新し、`tools/gen_proto.py` 参照を排除します。最後に CI へ `buf` の generation check と breaking check を追加して、契約変更の逸脱を PR 時に検出します。仕上げとして dead code / dangling reference（到達不能コード、未参照スクリプト、削除済み導線への参照）を掃除し、クリーンワークツリーで E2E フルを通して完了とします。

## Concrete Steps

作業ディレクトリは常に `/home/akira/esb` を前提にします。

1. Baseline 確認
   - 実行:
       git switch develop
       git pull --ff-only origin develop
       uv run python tools/gen_proto.py
       git status --short
   - 期待: 生成後に差分が出ない、または既知差分だけであること。

2. `buf` 設定追加と生成 POC
   - 追加予定ファイル:
     - `services/contracts/buf.yaml`
     - `services/contracts/buf.gen.yaml`
   - 実行:
       buf generate services/contracts
       git diff -- services/agent/pkg/api/v1 services/gateway/pb
   - 期待: 生成先が既存と同一で、差分の理由を説明できること。

3. 生成入口の切替
   - 編集予定ファイル:
     - `.mise.toml`（`gen-proto` タスク追加）
     - `tools/gen_proto.py`（削除）
   - 実行:
       mise run gen-proto
       git diff -- services/agent/pkg/api/v1 services/gateway/pb
   - 期待: `tools/gen_proto.py` なしで同じ成果物が再生成できること。

4. ドキュメント更新
   - 編集予定ファイル:
     - `services/agent/docs/proto-generation.md`
     - `services/agent/docs/README.md`
     - `services/agent/docs/grpc-api.md`
     - `services/contracts/README.md`
     - `docs/repo-layout-contract.md`
   - 期待: 生成入口の説明が `buf` に統一され、削除済みファイル参照が残らないこと。

5. CI ゲート追加
   - 編集予定ファイル:
     - `.github/workflows/quality-gates.yml`（または proto 専用 workflow）
   - 実行:
       buf generate services/contracts
       git diff --exit-code -- services/agent/pkg/api/v1 services/gateway/pb
       buf breaking --against '.git#branch=develop' services/contracts
   - 期待: 生成差分漏れと破壊的変更を PR 上で検出できること。

6. 最終 cleanup とクリーン E2E
   - 実行:
       rg -n "gen_proto.py|grpc_tools.protoc|rvolosatovs/protoc" docs services tools .github .mise.toml
       git status --short
       uv run e2e/run_tests.py --parallel --verbose
       git status --short
   - 期待: 旧導線への参照が残らず、E2E フルパス後も意図しない差分が発生しないこと。

## Validation and Acceptance

受け入れ条件は次の通りです。

`services/contracts/proto/agent.proto` を編集した後、`mise run gen-proto` または `buf generate services/contracts` だけで Go/Python 生成物が更新されること。`services/gateway` のユニットテストが import エラーなしで通ること。`services/agent` の Go テストが生成 API と整合して通ること。CI で proto 生成差分漏れと breaking change を検出できること。

加えて、最終確認はクリーンワークツリー（`git status --short` が空）で開始し、E2E フル実行後も不要差分・不要生成物が残らないことを条件にします。

検証コマンド:

    mise run gen-proto
    go -C services/agent test ./...
    uv run pytest services/gateway/tests -q
    buf breaking --against '.git#branch=develop' services/contracts
    ./tools/ci/check_repo_layout.sh
    ./tools/ci/check_tooling_boundaries.sh

## Idempotence and Recovery

`buf generate` は同じ入力に対して繰り返し実行しても同じ生成物を出す想定で設計します。`tools/gen_proto.py` 削除前に `buf` 出力同等性を確認し、問題があれば削除コミットを分離してロールバック可能にします。CI 追加は既存 workflow への段階的追加で行い、失敗時は proto 生成ジョブだけを一時的に切り戻せるようコミットを分けます。

## Artifacts and Notes

実装時は以下を短い証跡として追記します。

    $ buf generate services/contracts
    $ git diff -- services/agent/pkg/api/v1 services/gateway/pb
    (差分が空、または許容差分のみ)

    $ buf breaking --against '.git#branch=develop' services/contracts
    (breaking changes detected 0)

## Interfaces and Dependencies

最終状態で必須とするインターフェースは次の通りです。

- `services/contracts/buf.yaml`: module/lint/breaking の定義を持つ。
- `services/contracts/buf.gen.yaml`: Go/Python の生成先と plugin version を pin する。
- `.mise.toml`: 開発者入口として `gen-proto` タスクを持つ。
- 生成物配置:
  - Go: `services/agent/pkg/api/v1/agent.pb.go`, `services/agent/pkg/api/v1/agent_grpc.pb.go`
  - Python: `services/gateway/pb/agent_pb2.py`, `services/gateway/pb/agent_pb2_grpc.py`

依存は `buf` CLI を採用し、plugin version は `buf.gen.yaml` 内で明示 pin します。これにより「誰がいつ実行しても同じ生成物」を担保し、`latest` タグ依存を排除します。

---

Change note (2026-02-19 / Codex): 初版作成。現行の `tools/gen_proto.py` 導線と docs 参照、CI 非整備、Python import 補正のリスクを反映し、削除方針を維持した段階移行計画として整理した。
Change note (2026-02-19 / Codex): ユーザー要望により、最終クリーン状態での E2E フルパス確認と dead code/dangling reference cleanup を明示的な完了条件へ追加した。
Change note (2026-02-19 / Codex): 実装・検証完了。`buf` 生成導線へ移行し、旧 `tools/gen_proto.py` を削除。クリーンE2Eフルパスと dead code/残参照なしを確認した。
