# レジストリ常設 + docker-container builder + キャッシュ活用プラン

## 前提
- 目的: docker/containerd モード共通でレジストリを常設し、docker-container driver を使ってキャッシュ有効性を最大化する。
- 制約: 外部からの関数イメージ import 要件があるため、docker モードでも registry を立てる。

---

## AS-IS（現状）
- docker モードでは registry を前提にしていない。
- buildx builder は default（docker driver）前提の構成が混在。
- cache-to は docker driver で失敗するため抑制される構成が入っている（実効キャッシュは弱い）。
- compose build と esb build が並存し、ビルド経路の責務が分散。

---

## To-BE（目標）
- どのモードでも registry を起動する（docker / containerd 共通）。
- buildx は docker-container driver を標準化。
- bake / compose のビルド経路から cache-to を有効化し、共有キャッシュを活かす。
- 参照・配布のため、registry を通じた import/export を明確化。

---

## 変更方針（決定）
- **registry 常設:** docker モードでも registry を起動する。
- **docker-container driver:** すべてのビルドは docker-container driver を使う。
- **cache 有効化:** buildx bake / compose build ともに cache-to を有効化し、ローカル/registry キャッシュを活かす。
- **ブランド置換:** 生成物（docker-compose.*.yml / docker-bake.hcl / meta.go / default.env など）は\n+  既存の `esb` 表記を維持し、branding tool の置換で反映する。環境変数での置換は行わない。\n+  Go 実装は `meta.go`（`meta.Slug` など）を参照する。

---

## 実装計画

### 1) registry 常設化（docker モードも対象）
- 既存 registry 起動ロジックを docker モードにも適用。
- compose の registry サービスを常時起動対象にする。
- `BuildRegistry` / `RuntimeRegistry` を docker モードでも設定。
- 関数取得先は `CONTAINER_REGISTRY` を docker モードでも必須にする。
- docker モードはホスト到達性を優先し `127.0.0.1:5010` を既定にする。\n+  containerd モードは `registry:5010`（内部 DNS）を既定にする。
- `127.0.0.1:<port>` を採用し、IPv6 `localhost` の問題を回避。

### 2) buildx builder の docker-container 標準化
- `docker buildx` builder を docker-container driver で初期化・維持。
- 既存の builder 判定/分岐は削除し、`docker-container` 前提で統一。
- `BUILDX_BUILDER=<slug>-buildx` をデフォルトで設定し、compose build も同一 builder を使う。

### 3) cache-to の再有効化
- bake / compose の cache-to を復活し、共有キャッシュを活用。
- キャッシュ先は以下を使い分ける：
  - ローカル: `.<brand>/buildx-cache`（高速・ローカル最適化）
  - レジストリ: `type=registry`（並列/CI 共有）

### 4) ビルド経路の責務整理
- `esb build` を主経路として統一。
- compose build は run-only ではなく buildable を維持。
- bake で base/control/functions を確実に先に構築し、compose との依存を明確化。

---

## 影響範囲
- `cli/internal/generator/*`
- `docker-bake.hcl`
- `docker-compose*.yml`
- branding tool templates（`esb-branding-tool/tools/branding/templates/*`）

---

## テスト方針
- `esb build` が registry 起動込みで成功すること
- `docker compose up --build` が registry 起動込みで成功すること
- `e2e/run_tests.py --parallel` がキャッシュを活かして安定すること
- キャッシュヒット率の確認（buildログ/速度観察）

---

## リスク/注意点
- docker-container builder の初期化失敗時のハンドリングが必要。
- registry の常設により起動コスト/ポート競合が増えるため、ドキュメントで明示。
- キャッシュ先を registry に寄せる場合、ストレージ容量と掃除方針が必要。
