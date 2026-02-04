# Build Phase Speedup Plan (Docker Bake meta)

> 注記(2026-02-04): `tools/traceability` は削除済みで、このドキュメントの手順は現行実装と一致しません。再検討が必要です。

## 背景
E2E のビルド phase が重い。主因は以下:
- すべての Dockerfile で `build-meta` ステージが走り、`version.json` を毎回生成している。
- E2E は `esb build --no-cache` を多用しており、メタ生成とビルドが毎回フル実行される。

本プランでは `version.json` を Bake で一度だけ生成し、全イメージへ配布する構成に変更する。
その前提として `image_runtime` を `version.json` から削除する。

## 現状計測（修正前サマリ）
- 日付: 2026-01-26
- 実行コマンド: `uv run e2e/run_tests.py --parallel`（reset は常時実施）
- 結果: 手動中断（ビルドが 15 分以上継続）
- ビルド phase 時間: 約 15 分（途中停止・暫定基準）
- テスト phase 時間: 未到達
- 備考: 計測追加後に再実行し、ベースラインを更新する。

## 目的
- Build phase を短縮する（メタ生成を 1 回に集約）
- イメージごとの `version.json` 生成を廃止し、Bake で生成した 1 つを配布する

## 方針
- `version.json` から `image_runtime` を削除
- Docker Bake で `version.json` を 1 回生成
- `build-meta` ステージを廃止し、`COPY --from=meta /version.json /app/version.json` に統一

## 実装計画
1) **traceability の仕様変更**
   - `tools/traceability/generate_version_json.py` から `image_runtime` を削除
   - 引数 `--image-runtime` を削除
   - 該当ドキュメント更新

2) **Bake ターゲット追加**
   - `docker-bake.hcl` を追加
   - `meta` ターゲットで `version.json` を生成
   - `output=type=local,dest=...` で `version.json` を出力

3) **Dockerfile の更新**
   - 全 Dockerfile から `build-meta` ステージ削除
   - `COPY --from=meta /version.json /app/version.json` に統一

4) **ビルド経路の更新**
   - `esb build`（Go）: `--build-context meta=...` を常に渡す
   - `docker compose build`: `additional_contexts` に `meta=...` を追加
   - E2E build phase は `ESB_META_REUSE=1` をセットし、同一リポジトリ内の再利用を許可

5) **検証**
   - Build phase の時間比較（計測ログで確認）
   - `version.json` の内容・配置を確認
   - コンポーネントの起動確認

## リスクと対策
- **リスク**: Bake が実行されない場合 `version.json` が欠落
  - **対策**: `esb build` と E2E build phase の先頭で Bake を必須化
- **リスク**: build context が渡らず COPY 失敗
  - **対策**: CI で `docker build` 実行時の失敗を明示的に検知

## 成果物の確認ポイント
- `version.json` 生成が 1 回のみで済んでいること
- E2E build phase の時間短縮
- `/app/version.json` が全イメージに配置されていること
