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
| `e2e/runner/warmup.py` | deploy 前に必要な Buildx builder の事前準備 |
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
- `config_dir` を必須入力として取り込み（runner での staging パス推測は行わない）
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
- deploy を伴う実行では `artifactctl` バイナリが PATH 上に必要です（または `ARTIFACTCTL_BIN` で明示）。

## ログと診断
- 環境ごとのログ: `e2e/.parallel-<env>.log`
- Live UI は TTY + parallel + 非 verbose の場合のみ使用します。
- Plain reporter はフォールバックおよびサマリーイベントで常に使用します。

## 主要な回帰テスト
- `e2e/runner/tests/test_context.py`
- `e2e/runner/tests/test_ports.py`
- `e2e/runner/tests/test_warmup_command.py`

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
- `e2e/runner/tests/test_context.py`
- `e2e/runner/tests/test_ports.py`
- `e2e/runner/tests/test_warmup_command.py`
- `e2e/environments/test_matrix.yaml`
