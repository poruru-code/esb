<!--
Where: docs/reports/e2e_cli_coordination_review.md
What: Review of e2e runner coordination with the ESB CLI.
Why: Identify waste, coupling, and missing CLI features to improve stability and speed.
-->
# E2E Runner and CLI Coordination Review

対象: `e2e/run_tests.py` と `e2e/runner/*`（CLI 連携の観点）  
目的: Runner 側の無駄/密結合、CLI 側の不足機能を多面的に整理する

---

## Summary

Runner は CLI と密に結合しており、状態ファイル/コンテナ環境に依存するパスが複数あります。特に `ports.json` とコンテナ env 取得に強く依存しているため、CLI の内部変更に弱く、障害時の復旧も難しくなっています。CLI 側に「機械可読な出力」「環境同期」「クリーンアップの一元化」の API を用意することで、Runner の複雑さを削減できます。

**主要なギャップ**:
- `ports.json` の直接読み込みとコンテナ env 依存（Runner 側の実装が CLI 内部に引きずられる）
- `generator.yml` と matrix の同期ができず環境定義がズレる
- Runner の隔離設定が `ESB_HOME` に留まり、CLI のグローバル設定が汚染されうる

---

## Findings & Improvement Ideas

### 1) Runner が CLI の内部実装に依存している
- **状況**: Runner が `ports.json` を直接読み込む（`e2e/runner/env.py` の `load_ports`）ため、CLI の保存先や形式変更に弱い。
- **改善案（CLI）**:
  - `esb ports --format json` もしくは `esb info --json` を追加し、ポート情報を CLI の公開 API として取得可能にする。
  - `esb up --json` でポート/環境状態/認証情報のセットを返せると Runner がファイルアクセス不要になる。

### 2) 認証情報の取得がコンテナ依存
- **状況**: Runner は `esb env var gateway --format json` を使い、コンテナ環境変数から認証情報を取得（`e2e/runner/env.py` の `apply_gateway_env_from_container`）。
- **問題点**: コンテナ未起動/CLI が Docker へアクセスできない場合に失敗する。CLI が一度生成した認証情報を再利用できない。
- **改善案（CLI）**:
  - `esb credentials show --format json` など、CLI が生成した認証情報をファイル/設定から返すコマンドを追加。
  - `esb up --json` に「生成済み認証情報」を含める。

### 3) クリーンアップが Docker 直叩きで重複・分岐
- **状況**: Runner が `docker rm` / `docker network rm` / `docker volume rm` を直接実行（`e2e/runner/executor.py` の `thorough_cleanup`）。
- **問題点**: CLI のラベル/命名規約変更と乖離しやすい。Runner 側の保守コストが高い。
- **改善案（CLI）**:
  - `esb down --volumes --prune` のように、「環境単位で徹底削除」できるモードを CLI に用意。
  - `esb cleanup --hard` の新設も検討（Runner の docker 直叩きを排除）。

### 3.1) `stop` の期待値が CLI の実装に依存
- **状況**: Runner は `stop -> build -> up` の順で実行するが、`stop` は常に成功する前提（`e2e/runner/executor.py`）。
- **問題点**: CLI 側が「未起動の stop をエラー扱い」にすると E2E が落ちる。
- **改善案（CLI）**:
  - `esb stop`/`esb down` を冪等にする（存在しない環境でも成功扱い）。
  - あるいは `--ignore-missing` / `--no-error` を追加し Runner が明示可能にする。

### 4) `project add` が既存 `generator.yml` を更新できない
- **状況**: Runner は matrix から `esb project add` を呼び出し generator.yml を生成するが、既存ファイルがある場合は更新されない（`e2e/runner/executor.py` の `warmup_environment`）。
- **問題点**: matrix 変更が generator.yml に反映されず、環境定義のズレが残る。
- **改善案（CLI）**:
  - `esb project add --env ... --force` で既存 generator.yml の環境リストを更新可能にする。
  - もしくは `esb project sync --env ...` を新設し、環境定義だけを同期できるようにする。

### 5) CLI の設定ストレージが Runner の隔離と一致しない
- **状況**: Runner は `ESB_HOME` を設定して `ports.json` などを分離しているが、CLI のグローバル設定は `ESB_CONFIG_HOME/ESB_CONFIG_PATH` に依存する（`cli/internal/config/global.go`）。Runner はこれを設定していない。
- **問題点**: E2E がユーザーの実環境 `~/.esb/config.yaml` を汚染する可能性がある。
- **改善案（CLI）**:
  - `ESB_HOME` を基点に `config.yaml` を解決する挙動をサポート。
  - もしくは CLI に `--config-home`/`--config-path` フラグを追加し、Runner が完全隔離できるようにする。

### 5.1) CLI バージョンの取り違えリスク
- **状況**: Runner は `config/defaults.env` の `CLI_CMD` を PATH から呼び出す（`e2e/runner/utils.py`）。
- **問題点**: リポジトリ内の CLI と別バージョンが実行される可能性がある。
- **改善案（Runner/CLI）**:
  - Runner 側で `CLI_CMD` に実行パス（`./cli/bin/esb` 等）を指定できるようにする。
  - CLI 側で `esb info --json` にバージョンを含め、Runner が自己検証できるようにする。

### 6) Runner の起動手順に冗長性がある
- **状況**: `stop -> build -> up` を常に実施しているケースがある（`e2e/runner/executor.py`）。
- **改善案（CLI）**:
  - `esb up --build` が十分なら、Runner は `up --build` の 1 コマンドに寄せられる。
  - `esb up --ensure`（不足時のみ build）や `esb up --reuse` の追加で Runner の分岐削減が可能。

### 6.1) `project add` がエンティティ同期に使えない
- **状況**: Runner の `warmup_environment` は `project add` を毎回実行するが、既存 `generator.yml` を更新できない。
- **問題点**: matrix 変更の反映漏れが起きる。
- **改善案（CLI）**:
  - `project add --force-env` もしくは `project sync --env` を用意し、環境定義のみ更新できるようにする。

### 7) Firecracker の準備確認が未実装
- **状況**: `ensure_firecracker_node_up` が未実装で、Runner 側でのチェックが機能していない（`e2e/runner/env.py`）。
- **改善案（CLI）**:
  - `esb runtime status` や `esb node status` を用意し、Runner が統一チェックできるようにする。

---

## Recommended Actions (Priority)

1. **CLI に機械可読な出力を追加**: `esb info --json` / `esb up --json` / `esb ports --json`  
2. **CLI で認証情報取得 API を用意**: `esb credentials show --format json`  
3. **CLI で環境同期を可能に**: `esb project add --force-env` / `esb project sync`  
4. **Runner の完全隔離**: `ESB_CONFIG_HOME`/`ESB_CONFIG_PATH` の指定 or CLI 側で `ESB_HOME` を統合  
5. **クリーンアップの一元化**: `esb cleanup` or `down --purge` の追加

---

## Runner 側の即時改善（CLI 変更なしで可能）

- `ESB_CONFIG_HOME` を Runner から明示し、グローバル設定汚染を抑制。
- `stop` が失敗する場合は `check=False` に切り替え、Runner 側で冪等に扱う。
- `project add` 実行前に `generator.yml` の内容を確認し、不要な再登録を避ける。

---

## Notes

このレポートは CLI 側の API 境界を明確にすることで、Runner が内部構造（ports.json, Docker 直接操作）に依存しない構成へ移行することを主眼にしています。Runner の最適化は CLI の拡張とセットで進めるのが最短です。
