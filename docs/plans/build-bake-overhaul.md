# Docker Bake/Compose ビルドチェーン再設計プラン（AS-IS / To-BE）

## 前提
- 対象: CLI ビルド / E2E ランナー / docker-bake.hcl / docker-compose 定義
- 目的: E2E 並列ビルド衝突の解消、compose up の安定化、共有キャッシュ活用の最大化

---

## AS-IS（現状）

### ビルド経路
- `esb build` は以下 2 系統でビルド:
  - `docker buildx bake`: meta / base images / functions images
  - `docker compose build`: control-plane images（gateway/agent/provisioner, containerd は runtime-node 含む）
- bake は `docker-bake.hcl` と Go 側で生成する一時 HCL の合成で動作

### キャッシュ
- bake: `.esb/buildx-cache/<group>/<target>` を `cache-from/cache-to` に利用
- compose build: `cache_from/cache_to` を compose YAML 内で指定

### 排他制御
- `withBuildLock("meta")`, `withBuildLock("base-images")`, `withBuildLock("bake")`
- ロック配置は `~/.esb/.cache/staging/.lock-*`

### タグ
- 既定タグは `latest`（`ESB_TAG` が未指定の場合）
- E2E 並列ビルドで同一タグが共有され衝突しやすい

### compose up 依存
- `additional_contexts` が必須（BuildKit / compose v2.20+ 前提）
- 一部環境で compose build/up が失敗

---

## To-BE（目標）

### ビルドチェーンの一本化
- 全イメージ（meta/base/control/functions）を `docker buildx bake` で統一
- `docker-bake.hcl` をビルドチェーンの唯一のソースにする

### E2E 並列衝突の排他改善
- E2E ごとにユニークタグを強制（例: `e2e-<env>-<shortsha>`）
- 共有キャッシュは維持しつつ、キャッシュ書き込みは bake 単位でロック

### compose up の安定化
- compose は起動専用（build は実行しない）
- `docker compose up` は `image:` のみ参照（`build:` を削除 or override 化）
- BuildKit / compose version 依存を削減

### buildx driver の前提
- 当面は buildx driver を `default`（docker driver）に固定
- docker-container への一本化は次フェーズで実施（builder 切替のみで移行できる設計に寄せる）
- `mise run setup:buildx` は将来の docker-container 向け準備として任意実行

---

# 実装プラン（Decision-Complete）

## 1. E2E タグ分離の実装
**対象ファイル**
- `e2e/runner/executor.py`
- `e2e/runner/utils.py`

**変更内容**
- ユニークタグ生成関数を追加:
  - `build_unique_tag(env_name: str) -> str`
  - `git rev-parse --short HEAD` が取得できる場合は組み込み、取得不能時は `time.time()` を用いる
- `build_env_base["ESB_TAG"]` と `compose_env["ESB_TAG"]` に同一タグを注入

**期待効果**
- 並列 E2E でビルド・起動に使うタグが競合しない

---

## 2. CLI ビルドを Bake に統一
**対象ファイル**
- `cli/internal/generator/go_builder.go`
- `cli/internal/generator/go_builder_helpers.go`
- `cli/internal/generator/bake.go`

**変更内容**
- `compose.BuildProject` 呼び出しを廃止
- control-plane 用 bake グループを追加:
  - group name: `esb-control`
  - targets: `gateway-<mode>`, `agent-<mode>`, `provisioner`, `runtime-node-containerd`（mode=containerd 時のみ）
- compose build で使っていた `additional_contexts` / `args` / `secrets` を bake target に移植
- 既存 `withBuildLock("bake")` を reuse し cache 書き込み競合を抑制
- containerd のローカル registry は `127.0.0.1:<port>/` を build registry に使用（IPv6 `localhost` 回避）
- buildx bake は常に `--builder default` を指定（docker driver に統一）

**具体的なターゲット定義例**
- `gateway`: context `services/gateway`, dockerfile `Dockerfile.docker` / `Dockerfile.containerd`
- `agent`: context `services/agent`, dockerfile `Dockerfile.docker` / `Dockerfile.containerd`
- `provisioner`: context `services/provisioner`, dockerfile `Dockerfile`
- `runtime-node`: context `services/runtime-node`, dockerfile `Dockerfile.containerd`
- additional contexts:
  - `meta`, `meta_module`, `config`, `common`, `generator_assets`, `python-base`, `os-base`
- base images は build args (`PYTHON_BASE_IMAGE` / `OS_BASE_IMAGE`) で参照し、`python-base` / `os-base` は `docker-image://` で注入

---

## 3. docker-bake.hcl の拡張
**対象ファイル**
- `docker-bake.hcl`

**変更内容**
- control-plane 用 target を追加:
  - `gateway-docker`, `gateway-containerd`, `agent-docker`, `agent-containerd`, `provisioner`, `runtime-node-containerd`
- group 定義を追加:
  - `group "control-images-docker" { targets = [...] }`
  - `group "control-images-containerd" { targets = [...] }`
- cache 設定を統一:
  - `cache-from` / `cache-to` を control targets にも適用
- contexts 定義を bake に集約

---

## 4. Compose を runtime-only 化
**対象ファイル**
- `docker-compose.docker.yml`
- `docker-compose.containerd.yml`
- `docker-compose.fc.yml`
- `docker-compose.fc-node.yml`

**変更内容**
- `build:` セクションを削除（または dev-only override に移動）
- `image:` のみ参照するように整理
- `meta-builder` の build 依存を不要化（削除 or disabled）
- build-only サービス（`meta-builder` / `os-base` / `python-base`）は compose から削除

**補足**
- 開発用に必要なら `docker-compose.build.yml` を新規追加し、通常 compose には含めない

---

## 5. buildx driver 初期化（mise）
**対象ファイル**
- `.mise.toml`

**変更内容**
- `setup:buildx` タスクは将来の docker-container 移行用に保持
- `mise run setup` には組み込まず任意実行とする

---

## 6. Branding tool との同期
**方針**
- 本 repo で編集するファイルは branding tool の生成物のため、最終的に `esb-branding-tool` 側の
  テンプレート（`tools/branding/templates/*`）へ反映し、再生成で同期する

---

## テスト計画

### 単体/既存テスト
- `go test ./...`

### E2E
- `python e2e/run_tests.py --profile e2e-docker --build-only --parallel`
- 2 つ以上のプロファイルを並列実行し、タグ衝突が発生しないことを確認

---

## 期待される成果
- E2E 並列ビルド時のタグ/キャッシュ衝突が解消
- compose up が BuildKit/compose 依存で失敗しない構成へ移行
- docker-bake.hcl がビルド定義の唯一のソースとなる
