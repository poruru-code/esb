# Deploy-First 再設計プラン（AS-IS / TO-BE）

## 目的 / 方針
- **互換性は不要**。新フローに合わせて全体最適化・再設計。
- **`esb build` は完全削除**。
- **事前ビルドは必須ではない**（`docker compose up` で基盤がビルドされる）。
- `docker compose up` で基盤起動、`esb deploy` で関数デプロイを行う。
- 複数テンプレート（A/B）を順にデプロイし、**後勝ちで統合**。
- **削除はしない**（追加/上書きのみ）。

---

## AS-IS（現状）

### ビルド / 起動フロー
- `esb build` が SAM パース、設定生成、関数イメージ + control-plane ビルドまで一括実行。
- `docker compose up` は control-plane を起動。
- Gateway/Provisioner は build 時に設定（`functions.yml`/`routing.yml`/`resources.yml`）を baked-in。
- **CLI 現状**: `esb deploy` は存在せず、`esb build` が唯一のビルド系フロー入口。

### 設定反映
- Gateway は起動時に `functions.yml` / `routing.yml` を読み、**ホットリロードなし**。
- Provisioner は起動時に `resources.yml` を読み、**1 回のみ適用**。
- 参照パスは ` /app/config/*.yml `（ログで `not found` を確認）。

### 現状確認記録（2026-02-01）
- **Docker クリーン確認**: `docker ps -a` / `docker images` が空であることを確認。
- **キャッシュ削除確認**:
  - Docker: `docker builder prune -af` / `docker buildx prune -af` / `docker image prune -af` などでキャッシュ一掃。
  - ローカル: `~/.esb/.cache/staging/*` と `./.esb/buildx-cache/*` を削除。
  - buildx builder（`esb-buildx`）と buildkit イメージを削除。
- **起動前提**: `.env` が必須のため、`.env.example` を複製して `.env` を作成。
- **起動コマンド**: `docker compose up -d` を実行（`--build` 指定なし）。
  - **補足**: イメージが存在しないため、`--build` なしでも compose がビルドを実行。
- **起動結果（稼働サービス）**:
  - `esb-dev-gateway`, `esb-dev-agent`, `esb-dev-database`, `esb-dev-s3-storage`,
    `esb-dev-victorialogs`, `esb-infra-registry` が起動。
  - `esb-dev-provisioner` はワンショットで完了終了（期待どおり）。
- **ログ観測（要点）**:
  - Gateway: `functions.yml` / `routing.yml` が未生成のため警告（`not found`）。
  - Provisioner: `resources.yml` 不在のため「スキップ」警告 → 正常終了。
  - 主要基盤サービス（DB/S3/Logs/Registry/Agent）は起動完了ログを確認。

### E2E 現状確認（2026-02-01）
- **前提**: イメージ/キャッシュをクリーン化してから開始。
  - `docker image prune -af` / `docker builder prune -af` / `docker buildx prune -af` を実施。
  - `~/.esb/.cache/staging/*` と `./.esb/buildx-cache/*` を削除。
- **E2E 実行**: `uv run e2e/run_tests.py --parallel` を実行。
  - **注意**: 依頼コマンドは `run_test.py` だったが、実ファイルは `run_tests.py`。
  - **タイムアウト**: ランナーに timeout オプションが無いため、実行コマンド側の **長めタイムアウト（90 分）**で実施。
- **依存解決**:
  - 初回実行で `ModuleNotFoundError: rich` が発生。
  - `uv sync --all-extras` を実行し、`rich` を含む dev 依存を導入。
- **結果**:
  - **Build Phase で失敗**（`e2e-docker` / `e2e-containerd` 両方）。
  - 失敗箇所: `Building Python base image... Failed`（詳細ログ不足）。
  - ログ: `e2e/.parallel-e2e-docker.log` / `e2e/.parallel-e2e-containerd.log` を確認。
  - 既定のログでは詳細原因が出ないため、**再実行時は `--verbose` が必要**。
- **再実行（`--verbose`）結果**:
  - 失敗原因は **Registry (127.0.0.1:5010) への push が接続拒否**。
    - `failed to push ... dial tcp 127.0.0.1:5010: connect: connection refused`
  - 影響: `lambda-base` の push 失敗により build 全体が中断。
  - 補足: buildx キャッシュ警告（`index.json.lock: no such file`）は **クリーン状態起因**。
- **原因切り分け（E2E runner 実装確認）**:
  - `e2e/runner/infra.py::ensure_infra_up` は `docker compose -f docker-compose.infra.yml up -d registry` を実行するだけで、
    **Registry のヘルス待ち・ポート待ちが無い**。
  - そのため **起動直後に build の push が走ると接続拒否**になる可能性が高い（今回の失敗ログと一致）。
  - 対応案は TO-BE の E2E 改修で整理（起動待ちの追加が必要）。
- **再実行結果（deploy-first 対応後）**:
  - `e2e-docker` が **buildx cache export で失敗**。
  - 失敗ログ: `error writing manifest blob: ... rename tmp file ... .esb/buildx-cache/base/...: no such file or directory`
  - `e2e-containerd` は並列実行中に **強制終了（exit code: -15）** となった。
  - 並列実行で **両環境が同一の buildx cache ディレクトリ**を共有するため、
    **競合で失敗している可能性が高い**（TO-BE で分離が必要）。
- **再実行結果（reset 有効化後）**:
  - `docker compose down --volumes` が **CONFIG_DIR 未設定で失敗**することを確認。
  - 結果として volume が削除されず、クリーンスタートが崩れる（要修正）。
  - 修正後も `docker compose up` が **S3 (RustFS) のヘルス不一致で失敗**。
  - RustFS ログ: `panic ... manager.rs:120: expected value` を繰り返し出力。
  - `docker compose up` が `dependency failed to start: s3-storage is unhealthy` を返し、
    **E2E が deploy 前に失敗**する。
  - **前提**: RustFS は以前は正常稼働していたため、ここは「差分起因の回帰」前提で切り分ける。

---

## 差分整理（現状確認ベース）
- **起動の前提**: 現状は `docker compose up -d` だけでもビルドが走る（クリーン環境）。TO-BE でも **事前ビルドは必須にしない**。
- **CLI 入口**: 現状は `esb build` に集約。TO-BE は **`esb build` を完全削除**し、`esb deploy` を新設。
- **設定の読み取り位置**: 現状は ` /app/config/*.yml ` を参照。TO-BE は **`CONFIG_DIR` を唯一の正**とし、`/app/runtime-config` にマウント。
- **反映方式**: 現状は Gateway/Provisioner が起動時読み込みのみ。TO-BE は **Gateway ホットリロード** + **Provisioner を deploy 毎に実行**。
- **欠損時の扱い**:
  - 現状は `functions.yml` / `routing.yml` / `resources.yml` 不在で警告 → スキップ。
  - TO-BE は **/app/seed-config を優先**し、無い場合のみ空ファイル生成。ただし **警告は必ず表示**（未デプロイ状態を明示）。
  - `esb deploy` 実行時は **`resources.yml` を生成**し、空の場合は Warning を出して **no-op** 実行。

## TO-BE（目標）

### 新フロー概要
1. **基盤起動**（docker compose up）
   - `db/s3/logs/gateway/agent` + `runtime-node`（containerd のときのみ）
   - `registry` は `docker-compose.infra.yml` に含める
2. **デプロイ**（`esb deploy`）
   - SAM パース → 関数生成 → 関数イメージのみビルド
   - 集約設定に **追加/上書き** マージ
   - Provisioner を **都度実行**
   - Gateway は **ホットリロード**で即時反映
   - containerd の場合は **常に pull** して既存イメージを更新（固定タグ前提）

### 想定 CLI 手順（例）
```
# 基盤起動
# docker

docker compose up -d

# containerd
# docker compose up -d

# デプロイ（A/B）
esb deploy -t A.yml --env prod --mode docker
esb deploy -t B.yml --env prod --mode docker
```

### モード切り替え（docker / containerd）
- `docker-compose.yml` の include パスを切り替える
```
include:
  - path: docker-compose.infra.yml
  - path: docker-compose.docker.yml
  # または
  # - path: docker-compose.containerd.yml
```
※ E2E は `docker-compose.yml` の include を変更せず、`docker compose -f docker-compose.<mode>.yml`
  を直接使う（E2E ランナー側で切替）。

---

## コア設計

### 1) `esb deploy` コマンド
- **ビルド対象**: 関数イメージのみ（control-plane は対象外）
- **タグ運用**: 固定タグ（`latest` or `<BRAND>_TAG`）で上書き。ユニークタグは必須にしない。
  - E2E では **環境分離のためユニークタグを許容**（env override で切替）
- **引数仕様**: `esb build` と同一
  - `--template/-t`, `--env/-e`, `--mode/-m`, `--output/-o`, `--no-cache`, `--verbose`
- **事前チェック**:
  - **Registry Ready 確認**（`HOST_REGISTRY_ADDR` があればそれを使用、無ければ `127.0.0.1:${PORT_REGISTRY}`）
    - 例: `http://<host-registry>/v2/`
  - timeout: 60s（1s 間隔）。timeout 時は **deploy 失敗**で終了
  - Gateway / Agent が未起動の場合は **Warning** を出す（deploy は続行）
- **Registry Ready 実装位置（固定）**:
  - CLI 側: `cli/internal/workflows/deploy.go` に **`waitRegistryReady()`** を追加
  - `docker compose up` は呼ばず、**HTTP `/v2/` の 200 を待機**するのみ
- **実行内容**:
  1. SAM パース / 生成
  2. 関数イメージのみビルド（固定タグ）
  3. 集約設定にマージ
  4. Provisioner を都度実行
  5. containerd の場合は **関数イメージを必ず pull**（固定タグ更新を反映）

#### control-plane のビルド方法
- `docker compose up -d` 実行時に **イメージが無ければ自動ビルド**される
- 事前ビルドは任意（CI/配布で push しても良いが必須ではない）

### 2) 集約設定（CONFIG_DIR）
- `CONFIG_DIR` が **唯一の設定ソース**
- 期待ファイル:
  - `${CONFIG_DIR}/functions.yml`
  - `${CONFIG_DIR}/routing.yml`
  - `${CONFIG_DIR}/resources.yml`
- Gateway / Provisioner は `CONFIG_DIR` から読む（baked-in 参照は廃止）
- **永続化**:
  - `CONFIG_DIR` はホスト側のパス（`~/.<brand>/.cache/staging/...`）に配置されるため **再起動後も保持**される
  - `docker compose down --volumes` や手動削除を行うと消える

#### CONFIG_DIR の決定と共有
- `CONFIG_DIR` は **compose 起動時に必ず固定値で指定**する
- 計算規則（Go 側 staging.ConfigDir と同一）
  - `proj_key = <project>`
  - `seed = "<project>:<env>"`（env は lower）
  - `hash = sha256(seed)[:8]`
  - `stage_key = <project>-<hash>`
  - `root = $XDG_CACHE_HOME/<brand>/staging` または `~/.<brand>/.cache/staging`
  - `CONFIG_DIR = <root>/<stage_key>/<env>/config`
- `esb deploy` は **同じルールで計算**し、同一パスに書き込む
- E2E は `calculate_staging_dir()` と同一ロジックで固定化

### 2.5) 初期設定のシード（未デプロイ時の挙動）
- **目的**: 事前登録が必要なケースを許容しつつ、未デプロイ状態を**警告で明示**する。
- **基本方針**: 初期状態は以下の優先順で決定する。
  1. `${CONFIG_DIR}` に既存設定があれば **そのまま採用**（最優先）。
  2. **イメージに焼き込んだ `/app/seed-config` があれば初期設定としてコピー**。
  3. どちらも無い場合のみ **空の有効 YAML を生成**（ただし**警告は必ず表示**）。
- **実装主体（固定）**:
  - **Gateway の entrypoint** で初期化を行う（起動前の 1 回のみ）。
  - `CONFIG_DIR` が空の場合に限り、`/app/seed-config` → `CONFIG_DIR` へコピーする。
  - いずれも無い場合は空ファイルを生成し **Warning を出す**。
- **マージ基準**:
  - シード済み設定は **既存集約のベース**として扱う。
  - 以降の `esb deploy` はこのベースに **マージ（後勝ち）**する。
- **シード供給の方式（固定）**:
  - **イメージに焼き込み**（`/app/seed-config` に配置）
- **リポジトリ配置（必須）**:
  - 例: `services/gateway/seed-config/`
  - `functions.yml` / `routing.yml` / `resources.yml` を **空でも良いので必ず用意**する
  - Gateway の Dockerfile で `seed-config` を `/app/seed-config` にコピーする
    - 追加箇所: `services/gateway/Dockerfile.docker` と `services/gateway/Dockerfile.containerd`
- **コピーのルール**:
  - `functions.yml` / `routing.yml` / `resources.yml` を対象
  - 既存ファイルがある場合は **上書きしない**（初期化は一回のみ）
  - コピー後は `esb deploy` が **通常のマージ**を実行（重複は後勝ち）
  - 初期コピーは **`.deploy.lock` と同じロック**を取得して実行（deploy との競合回避）
- **警告方針**:
  - `CONFIG_DIR` も `/app/seed-config` も無い場合は、空ファイル生成後でも **Warning を必ず出す**。
  - 「未デプロイ状態である」ことを明示する警告文言にする。
  - 例: `WARNING: No runtime config found. Using empty config. Deploy required.`
- **空ファイル内容**:
  - 既存の YAML スキーマに準拠した「空の設定」を出力すること（構造は現行の生成ロジックに合わせる）
  - 例:
    - `functions.yml`: `functions: {}`（`defaults: {}` は任意）
    - `routing.yml`: `routes: []`
    - `resources.yml`: `resources: {}`
  - `esb deploy` は **既存集約にマージして出力**（空の場合も含む）。重複は後勝ち

### 3) マージルール（追加/上書きのみ）
- **マージ順序**: 既存集約 → 新規デプロイ（後勝ち）
- **対象ファイル**: `functions.yml` / `routing.yml` / `resources.yml` は **すべてマージ対象**
- **functions.yml**
  - `functions` は **関数名キーで上書き**
  - `defaults` は **既存保持 + 欠損キー補完**（上書きはしない）
  - テンプレート固有 defaults は **deploy 時に関数へ焼き込み**（function-level へ展開）
- **routing.yml**
  - キーは `(path, method)`
  - 同一キーがあれば **後勝ち**で上書き
- **resources.yml**
  - DynamoDB: `TableName` で上書き
  - S3: `BucketName` で上書き
  - Layers: `Name` で上書き
  - **将来拡張**: 新しいリソース種別は **明示的にマージキーを追加**する（未定義はスキップして Warning）

#### defaults 焼き込みの具体手順
- RenderFunctionsYml 前に **FunctionSpec.Environment** に defaults をマージ
- 既存 `FunctionSpec.Environment` が優先（テンプレート側が勝つ）

### 4) Gateway ホットリロード
- `functions.yml` / `routing.yml` を **一定間隔で再読込**
- `CONFIG_RELOAD_INTERVAL`（デフォルト: `1s`、最小: `0.5s`）
- 変更検知は mtime + size
- **原子的更新**を前提（tmp → rename）
- 読み込み中の部分ファイルは **リトライ**（短い backoff）
- エラー時は旧設定を維持し、ログのみ出力

### 5) Provisioner 都度実行
- `esb deploy` が **mode 別 compose** で `docker compose run --rm provisioner` を実行
  - docker: `docker-compose.docker.yml`
  - containerd: `docker-compose.containerd.yml`
  - project 名は `<brand>-<env>` に統一（他サービスと同一）
- manifest の参照順:
  1. `${CONFIG_DIR}/resources.yml`
  2. `RESOURCES_YML` env（任意）
- **resources が空/未生成の場合**:
  - `esb deploy` が **空の `resources.yml` を生成**し、Warning を出す
  - Provisioner は **no-op** として実行（成功扱い）
- 失敗時は `esb deploy` を **失敗扱い**で終了（再試行はユーザー判断）
- **リソース更新/削除は非対応**（create のみ保証）
- `CONFIG_DIR` が生成できない場合は **エラー終了**
- **永続化**:
  - `resources.yml` は `CONFIG_DIR` に保存されるため **設定としては永続化**
  - 実体リソース（DynamoDB/S3 など）は各サービスの **volume** に保持される（ボリューム削除で消える）

### 6) containerd の「常時 pull」設計
- 新規環境変数: `IMAGE_PULL_POLICY`（`if-not-present` / `always`）
- containerd モードの agent には `IMAGE_PULL_POLICY=always` を設定
- docker モードは `if-not-present`（従来通り）
- Agent の実装:
  - image が存在しても `Pull` を実行
  - pull 後に最新 digest を使用

### 7) Compose 変更
- `gateway` から `provisioner` 依存を削除
- `CONFIG_DIR` を `/app/runtime-config` にマウント
- baked-in 設定のコピーは廃止
- **初期設定はイメージ内 `/app/seed-config` に固定**（外部マウントは不要）
- containerd モードでは agent に `IMAGE_PULL_POLICY=always` を付与

---

## 影響範囲（削除/変更）

### 削除対象（具体）
- CLI
  - `cli/internal/commands/build.go`
  - `cli/internal/commands/build_test.go`
  - `cli/docs/build.md`
  - `cli/docs/architecture.md` の build 記述
- Generator
  - `buildControlImages` とその呼び出し
  - build-only workflow / compose build 関連
- Docs
  - README / docs から build 記述を削除

### 変更対象
- CLI: `esb deploy` 追加、`build` 関連コード削除
- Generator: 関数イメージのみビルドへ簡略化
- Gateway: runtime-config 参照 + ホットリロード
- Provisioner: runtime-config 参照 + 都度実行
- Compose: depends_on / volume マウントの更新
- E2E: 新フローに合わせた手順へ変更

---

## E2E 再設計（新フロー準拠）

### 目的
- `docker compose up -d` で基盤起動 → `esb deploy` を順に実行 → Gateway ホットリロード反映
- A/B テンプレートの統合デプロイ（後勝ち）を検証
- 並列実行時は **環境名でコンテナ分離**（既存ルール）
- E2E では `docker-compose.yml` の include を変更しない（ランナーが mode 別 compose を直接指定）

### 新フロー（E2E）
1. **基盤起動**（`--build` なし）
   - docker compose up -d (db/s3/logs/gateway/agent/runtime-node)
   - **補足**: イメージ未存在のときは compose が自動ビルドする
2. **Registry Ready 待機**（必須）
   - `docker-compose.infra.yml` の `registry` 起動後、**HTTP `/v2/` が 200 になるまで待つ**
   - 例: `http://127.0.0.1:${PORT_REGISTRY:-5010}/v2/`
   - timeout: 60s（1s 間隔）。タイムアウト時は **E2E を失敗**扱い。
3. **esb deploy を実行**
   - `esb deploy -t template-a.yml` → `esb deploy -t template-b.yml`
4. **ホットリロードを待機**（poll + timeout）
5. **E2E テスト実行**

### テスト用テンプレートとコード配置
- `e2e/fixtures/template-a.yml`
  - `/hello` GET → `HelloAFunction`
- `e2e/fixtures/template-b.yml`
  - `/hello` GET → `HelloBFunction`
- コード配置:
  - `e2e/fixtures/functions/hello_a/`（`lambda_function.py` が `{"version":"A"}` を返す）
  - `e2e/fixtures/functions/hello_b/`（`lambda_function.py` が `{"version":"B"}` を返す）

### 期待結果
- B デプロイ後に `/hello` が `{"version":"B"}` を返す

### 変更ポイント
- `e2e/runner/executor.py` の build フェーズを **deploy フェーズ**に置換
- `e2e/runner/utils.py` の `run_esb` 呼び出しは `build` → `deploy`
- `esb build` 呼び出しは完全削除
- `CONFIG_DIR` を環境別に固定（`calculate_staging_dir()` を流用）
- **buildx cache の分離**:
  - `BUILDX_CACHE_DIR` を env 別に設定し、並列実行時の競合を防止
  - `docker-compose.*.yml` は `BUILDX_CACHE_DIR` の指定があればそれを使用
- **reset の信頼性**:
  - `docker compose down` 実行時に **CONFIG_DIR を必須で渡す**
  - これにより volume を確実に削除し、クリーンスタートを保証
- ホットリロード反映待機は **poll + timeout** を実装（固定 sleep は不可）
- **Registry Ready 待機を追加**（`ensure_infra_up` 直後に `/v2/` 200 を確認）
  - 実装位置: `e2e/runner/infra.py` に **`wait_registry_ready()`** を追加し `ensure_infra_up` から呼ぶ

#### ホットリロード wait の具体条件
- `GET /hello` を 1s 間隔で最大 30 秒まで実行
- 期待値が B になった時点で成功
- timeout で失敗

---

## 実装プラン（小ステップ）

### Phase 0: 事前レビュー（設計確定）
- このドキュメントをレビューし、仕様確定
- 合意事項: マージルール / 削除方針 / pull policy / hot reload 間隔 / provisioner 実行方式
#### Phase 0 レビュー観点チェックリスト
- [ ] **新フロー合意**: `docker compose up -d` → `esb deploy` の 2 段階
- [ ] **`esb build` 完全削除**: CLI / docs / generator / tests からの削除範囲が合意済み
- [ ] **マージ方針**: 追加のみ・削除しない・重複は後勝ち
- [ ] **resources.yml マージ**: `resources.yml` も merge 対象であること
- [ ] **defaults の扱い**: defaults は function に焼き込み、既存 defaults は維持
- [ ] **初期シード**: `CONFIG_DIR` 優先 → イメージ内 `/app/seed-config` → 空生成（警告必須）
- [ ] **seed-config の配置**: リポジトリに空ファイルを含む `seed-config` が存在する
- [ ] **seed-config のコピー**: Dockerfile（docker/containerd）で `/app/seed-config` にコピーする
- [ ] **Warning 方針**: 未デプロイ時は必ず Warning 表示
- [ ] **Registry Ready**: `/v2/` 200 の待機を deploy/E2E に入れる
- [ ] **Provisioner**: deploy 毎に実行、空 resources は Warning + no-op
- [ ] **Hot Reload**: 1s 間隔、原子的更新、エラー時は旧設定維持
- [ ] **Lock/Atomic**: `.deploy.lock` と tmp→rename を採用
- [ ] **固定タグ運用**: `latest`/`<BRAND>_TAG` を上書き（E2E はユニークタグ許容）
- [ ] **containerd pull**: `IMAGE_PULL_POLICY=always` を合意
- [ ] **compose 切替**: include で docker/containerd を切替、E2E は include 変更なし

### Phase 1: CLI 再設計
1. `cli/internal/commands/app.go` から build サブコマンド削除
2. `cli/internal/commands/deploy.go` 追加（build.go をベースに縮小）
3. `cli/internal/workflows/deploy.go` 追加（関数イメージのみ）
4. `cli/internal/wire/wire.go` に deploy wiring 追加
5. 旧 build 関連コードを削除（未参照のテスト含む）

### Phase 2: Generator の簡略化
1. `cli/internal/generator/go_builder.go` を deploy 専用に分割
2. base image の build は「存在しなければ作る」扱い
3. `buildControlImages`/compose build の削除
4. `GenerateFiles` は保持（config 生成のみ）

### Phase 3: 設定マージ機構
1. `cli/internal/generator` に `merge_config.go` 追加
2. `CONFIG_DIR` に集約ファイルを出力（原子的更新）
3. マージルール実装（functions/routing/resources）
4. 排他ロック（ファイルロック）追加

#### ロック仕様
- ロックファイル: `${CONFIG_DIR}/.deploy.lock`
- 粒度: **env 単位**
- 同時 deploy は lock 待ち、タイムアウトは 30s
- `CONFIG_DIR` が未作成の場合は **mkdir -p** してから lock を取得する

#### 原子的更新仕様
- `${CONFIG_DIR}/<name>.yml.tmp` に書き込み
- `fsync` → `rename` で置換

### Phase 4: Gateway ホットリロード
1. `services/gateway/config.py` に runtime-config パス追加
2. `FunctionRegistry` / `RouteMatcher` に reload 機構追加
3. エラー時は旧設定を維持、ログ出力のみ
4. hot reload の動作確認テスト追加
5. `services/gateway/seed-config/` を追加し、`/app/seed-config` にコピーする

### Phase 5: Provisioner 都度実行
1. `services/provisioner/src/main.py` を `CONFIG_DIR` 参照に変更
2. `esb deploy` から `docker compose run --rm provisioner` を実行
3. 失敗時は deploy 失敗扱い（exit code 非 0）

### Phase 6: Compose 更新
1. `docker-compose.*.yml` の `gateway` 依存から `provisioner` 削除
2. `CONFIG_DIR` を `/app/runtime-config` にマウント
3. `provisioner` も `/app/runtime-config` を参照
4. **初期設定はイメージ内 `/app/seed-config` に固定**（compose での外部マウントは不要）
5. containerd では agent に `IMAGE_PULL_POLICY=always` を追加

### Phase 7: E2E 更新
1. build フェーズ削除 → deploy フェーズ化
2. A/B テンプレート連続デプロイのシナリオ追加
3. **Registry Ready 待機**の追加（`ensure_infra_up` 直後に実施）
4. ホットリロード待機（poll + timeout）を追加
5. 競合後勝ちの検証テスト追加

### Phase 8: ドキュメント整理
1. `build` に関する記述を削除
2. `deploy` 新フローの説明追加
3. control-plane 供給前提を README に追記
4. `docker-compose.yml` の include 切替手順を README に追記

---

## レビューフェーズ

### Review 1: CLI / Generator 変更後
- `esb build` が完全に消えていること
- `esb deploy` が関数イメージのみビルドすること

### Review 2: マージ / ホットリロード実装後
- 競合時の後勝ちが正しく動作すること
- 削除が起きていないこと
- Gateway が再起動なしで反映すること
- atomic update が守られていること

### Review 3: E2E 完了後
- 新フローで E2E が通ること
- 並列実行時の分離が崩れていないこと
- A/B テンプレート競合で後勝ちが証明されていること
- Registry Ready 待機により **クリーン起動時の push 失敗が再発しない**こと

---

## 依頼事項
- 実装者は本ドキュメントに沿って実装
- レビューフェーズ（3回）でレビュー依頼する

---

## 未解決リスク / 要調査（差分に限定）
- **RustFS (S3) 回帰の可能性**: 以前は正常稼働していた事実があるため、
  **差分（compose/env/reset/seed 周り）に限定して原因を切り分ける**。
  - ログ: `manager.rs:120: expected value`
  - 影響: E2E が deploy 前に停止
  - 対応方針: RustFS 自体の改変は行わず、以下の差分のみ検証する  
    1) `CONFIG_DIR` 必須化に伴う `docker compose down` の失敗/未実行  
    2) E2E での env 生成・上書き順序（RUSTFS_* の注入含む）  
    3) compose include/プロジェクト名変更の影響（volume 競合/不整合）
