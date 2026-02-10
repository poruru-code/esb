<!--
Where: e2e/runner/README.md
What: Architecture and execution contracts for the E2E runner pipeline.
Why: Keep WS1 runner responsibilities and behavior explicit for maintenance.
-->
# E2E Runner アーキテクチャ

## 対象範囲
このドキュメントは `e2e/run_tests.py` の背後にある**実行パイプライン**を説明します。
主に、オーケストレーションの挙動、モジュール境界、失敗時の契約を対象にします。

Smoke/runtime シナリオ設計は `docs/e2e-runtime-smoke.md` を参照してください。

## 正式な実行経路
`e2e/run_tests.py` が唯一のエントリーポイントです。

```text
e2e/run_tests.py
  -> e2e.runner.cli.parse_args
  -> e2e.runner.config.load_test_matrix
  -> e2e.runner.planner.build_plan
  -> e2e.runner.runner.run_parallel
```

`e2e/runner/executor.py` は現在の実行経路には含まれません。

## フェーズモデル
各環境は同じフェーズ順で実行されます。

1. `reset`
2. `compose`
3. `deploy`
4. `test`

`--test-only` 指定時は `reset/compose/deploy` をスキップします。
`--build-only` 指定時は deploy 成功後に `test` をスキップします。

## モジュール責務（WS1 分割）
| モジュール | 責務 |
| --- | --- |
| `e2e/runner/runner.py` | スイートのオーケストレーション、並列スケジューリング、フェーズ進行、結果集計 |
| `e2e/runner/context.py` | 環境ごとの runtime/deploy/pytest コンテキスト組み立てと env マージ |
| `e2e/runner/ports.py` | 並列実行時の環境別ホストポートブロックの安定割り当て |
| `e2e/runner/warmup.py` | テンプレート走査と Java フィクスチャのウォームアップ（Docker 上の `maven`） |
| `e2e/runner/lifecycle.py` | `compose up/down`、reset、gateway ヘルス待機 |
| `e2e/runner/cleanup.py` | コンテナ/ネットワーク/ボリューム/イメージの強制クリーンアップ |
| `e2e/runner/buildx.py` | deploy/build フロー向け Buildx builder の選択・作成 |
| `e2e/runner/planner.py` | matrix エントリを `Scenario` オブジェクトへ変換 |
| `e2e/runner/config.py` | matrix のパースと環境シナリオ展開 |

## Matrix と Scenario 解決
`e2e/environments/test_matrix.yaml` でスイートと対象環境を定義します。

`e2e/runner/config.py` は各 matrix エントリを scenario に解決します。
- mode 推論: `docker` / `containerd`
- `env_dir` からの env ファイル解決
- suite の target をプロジェクト相対パスへ展開
- `env_dir` / `env_file` に `firecracker` を含む場合の推論

その後 `e2e/runner/planner.py` が生の辞書を型付き `Scenario` に変換します。

## 失敗時・終了時の契約
`e2e/run_tests.py` は次の CLI 契約を維持します。

- いずれかの環境が失敗した場合、プロセスは非ゼロ（`1`）で終了します。
- 失敗環境では `e2e/.parallel-<env>.log` の末尾ログを表示します。
- `run_parallel` は環境名をキーにした `dict[str, bool]` を返します。
- `--build-only` と `--test-only` は同時指定できません。
- `--test-target` は `--profile` が必須で、指定 target のみ実行します。

## ログと診断
- 環境ごとのログ: `e2e/.parallel-<env>.log`
- Live UI は TTY + parallel + 非 verbose の場合のみ使用します。
- Plain reporter はフォールバックおよびサマリーイベントで常に使用します。

## 主要な回帰テスト
- `e2e/runner/test_runner_java_warmup.py`
- `e2e/runner/tests/test_context.py`
- `e2e/runner/tests/test_ports.py`
- `e2e/runner/tests/test_warmup_templates.py`

## よく使うコマンド
```bash
# デフォルト matrix（直列）
uv run e2e/run_tests.py

# matrix を並列実行
uv run e2e/run_tests.py --parallel

# 単一プロファイル
uv run e2e/run_tests.py --profile e2e-containerd

# プロファイルを build のみ実行
uv run e2e/run_tests.py --profile e2e-containerd --build-only --verbose
```

## Java ビルドイメージ
Java warmup/deploy で使う Java ビルドイメージは次の固定 digest のみを使用します。

`public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7`

上書き用の環境変数は提供しません。

## Proxy 検証
実proxy環境での挙動を検証する場合は tinyproxy ハーネスを使用します。

Java warmup と deploy の Java build は次の契約を常に満たします。

- 毎回一時 `settings.xml` を生成して Docker に read-only マウントする
- Maven は常に `mvn -s /tmp/m2/settings.xml ...` で実行する
- `HTTP_PROXY`/`HTTPS_PROXY`（大小文字）から `settings.xml` 内に `<proxy>` を生成する
- Java build コンテナ内の proxy env は空値に固定し、proxy ソースを `settings.xml` に一本化する
- Maven 依存取得は `-Dmaven.artifact.threads=1` で直列化し、認証付き proxy の再現性を優先する
- Maven local repository は `./.esb/cache/m2/repository` を共有キャッシュとして使用する
- `~/.m2/settings.xml` 依存、`-s` なし実行、`latest` タグを禁止する
- 契約仕様の正本は `docs/java-maven-proxy-contract.md`

契約の静的チェック:

```bash
bash tools/ci/check_java_proxy_contract.sh
```

Maven キャッシュをクリアする場合:

```bash
rm -rf .esb/cache/m2/repository
```

```bash
uv run python tools/e2e_proxy/run_with_tinyproxy.py --check-only
uv run python tools/e2e_proxy/run_with_tinyproxy.py -- \
  uv run e2e/run_tests.py --profile e2e-docker --verbose
uv run python tools/e2e_proxy/run_with_tinyproxy.py \
  --proxy-user proxyuser \
  --proxy-password proxypass \
  --check-only
```

`--proxy-user/--proxy-password` を指定した場合、tinyproxy BasicAuth も有効化されます。

---

## 実装参照
- `e2e/run_tests.py`
- `e2e/runner/runner.py`
- `e2e/runner/context.py`
- `e2e/runner/ports.py`
- `e2e/runner/warmup.py`
- `e2e/runner/lifecycle.py`
- `e2e/runner/cleanup.py`
- `e2e/runner/buildx.py`
- `e2e/runner/config.py`
- `e2e/runner/planner.py`
- `e2e/runner/test_runner_java_warmup.py`
- `e2e/runner/tests/test_context.py`
- `e2e/runner/tests/test_ports.py`
- `e2e/runner/tests/test_warmup_templates.py`
- `e2e/environments/test_matrix.yaml`
