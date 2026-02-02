# Deploy 分離プラン（AS-IS / To-BE）

## 前提・目的
- 目的: `docker compose up` による基盤起動と、関数デプロイ（SAM パース/関数生成/Gateway 関連付け/Provision）を分離する。
- ユースケース: 複数の SAM テンプレート（A/B）を段階的にデプロイし、同一環境に統合する。競合は「後勝ち」。
- 削除: **未実施**。デプロイ対象に存在しない関数/ルート/リソースは既存設定から削除しない。

---

## AS-IS（現状）

### ビルド/起動フロー
- `esb build` が以下を**一括**で実行。
  - SAM パース
  - `functions.yml` / `routing.yml` / `resources.yml` 生成
  - 関数イメージ + control-plane（gateway/agent/provisioner/runtime-node）ビルド
- `docker compose up` は control-plane を起動。
- Gateway/Provisioner は build 時に **設定を baked-in** したイメージを利用。

### 設定反映
- Gateway は `functions.yml` / `routing.yml` を起動時に読み込み、**ホットリロードなし**。
- Provisioner は `resources.yml` を起動時に読み込み、**compose 起動時に 1 回だけ適用**。

### 依存関係
- `gateway` は `provisioner` の完了に依存して起動（`depends_on`）。

### 制約
- 1 つのテンプレート前提の設計。
- 設定変更の反映には control-plane の再ビルド/再起動が必要。

---

## To-BE（目標）

### 起動・デプロイ分離
- **基盤起動（必須サービスのみ）**
  - `docker compose up` で以下を先に起動:
    - `db` / `s3` / `logs`
    - `gateway` / `agent`
    - `runtime-node`（containerd のときのみ）
    - `registry` は既存の外部分離構成を継続
  - `provisioner` は **起動必須ではない**（依存を外す）。

- **デプロイ（新規 `esb deploy`）**
  - SAM パース → 関数生成 → 関数イメージのみビルド
  - `functions.yml` / `routing.yml` / `resources.yml` を **集約設定にマージ**
  - `provisioner` を **都度実行**してリソース適用
  - Gateway は **ホットリロードで即時反映**

### 想定フロー（ユーザー操作）
```
# 基盤起動
# docker-compose.docker.yml (docker)
# docker-compose.containerd.yml (containerd)

docker compose -f docker-compose.docker.yml --env-file .env up -d \
  database s3-storage victorialogs gateway agent

# containerd の場合
# docker compose -f docker-compose.containerd.yml --env-file .env up -d \
#   database s3-storage victorialogs runtime-node gateway agent

# A をデプロイ
esb deploy -t A.yml --env prod --mode docker

# B をデプロイ
esb deploy -t B.yml --env prod --mode docker
```

---

## 変更点（設計詳細）

### 1) 新コマンド `esb deploy`
- **フラグ仕様は `esb build` と同一**
  - `--template/-t`, `--env/-e`, `--mode/-m`, `--output/-o`, `--no-cache`, `--verbose`
- **実行内容**
  - SAM パース + 生成（既存 generator を再利用）
  - 関数イメージのみビルド（control-plane は対象外）
  - `lambda-base` が存在しない場合のみ作成
  - 生成された設定を集約ディレクトリにマージ
  - `provisioner` を都度実行

### 2) 集約設定のマージルール（後勝ち）
- **削除はしない（追加/上書きのみ）**
- **functions.yml**
  - `functions` は **関数名キーで上書き**
  - `defaults` は **既存を維持し、不足キーのみ補完**
    - 例: 既存 `defaults.environment.LOG_LEVEL` が無い場合のみ追加
- **routing.yml**
  - ルートキー `(path, method)` で上書き
  - 既存の同一キーは削除し、新規分のみを残す
- **resources.yml**
  - DynamoDB: `TableName` キーで上書き
  - S3: `BucketName` キーで上書き
  - Layers: `Name` キーで上書き

### 3) Runtime Config の配置
- `CONFIG_DIR` を**ランタイム設定のホストパス**として扱う
- 期待される配置:
  - `${CONFIG_DIR}/functions.yml`
  - `${CONFIG_DIR}/routing.yml`
  - `${CONFIG_DIR}/resources.yml`
- `CONFIG_DIR` 未設定時は**既存のデフォルト**を使用:
  - `services/gateway/config`

### 4) Gateway ホットリロード
- Gateway は以下のパスを優先的に参照:
  1. `/app/runtime-config`（`CONFIG_DIR` をマウント）
  2. `/app/config`（baked-in 互換）
- `functions.yml` / `routing.yml` は **mtime 監視 + インターバル**で再読込
  - 例: `CONFIG_RELOAD_INTERVAL=1s`
- ルート/関数の更新は **再起動不要**で即時反映

### 5) Provisioner の都度実行
- `esb deploy` が `docker compose run --rm provisioner` を実行
- Provisioner は以下の順で manifest を参照:
  1. `RESOURCES_YML` 環境変数
  2. `${CONFIG_DIR}/resources.yml`
  3. `/app/config/resources.yml`
- これにより **都度最新の resources.yml を適用**

### 6) Compose の変更
- `gateway` から `provisioner` 依存を削除
- `CONFIG_DIR` を `/app/runtime-config` にマウント
  - gateway / provisioner で共通に使用
- 既存の `build:` は維持（`esb build` 互換のため）

---

## 互換性
- `esb build` は既存通り利用可能（baked-in 方式も残す）
- Runtime config が存在しない場合は従来通り `/app/config` を利用
- 既存の単一テンプレート運用は影響なし

---

## リスクと対策
- **競合の不整合**: ルート/関数はキー単位で上書きするため後勝ち保証
- **削除不可**: 削除は明示的な要件が出るまで保留
- **同時 deploy**: マージ時に排他ロックを導入し競合を防止
- **ホットリロード頻度**: 監視間隔でコストを調整可能

---

## 実装タスク（概要）
- CLI:
  - `esb deploy` の追加
  - deploy workflow + generator 再利用 + function image build のみに限定
- Generator:
  - マージ処理の追加（functions/routing/resources）
  - 集約 config 書き込み + 原子的な更新
- Gateway:
  - runtime config パス優先ロジック
  - functions/routing のホットリロード
- Provisioner:
  - manifest パスの環境変数対応
- Compose:
  - runtime config volume の追加
  - provisioner 依存削除

---

## テスト計画
- Unit
  - マージロジック（functions/routing/resources）
  - Gateway のホットリロード
- E2E
  - A→B の連続 deploy で後勝ち確認
  - provisioner 都度実行で DynamoDB/S3 が再適用されること
  - docker / containerd 両モード
