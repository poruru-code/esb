**「template.yaml (AWS SAM) を正とする（Single Source of Truth）」**という方針に基づいた、具体的かつ実装可能なマスタープランを提示します。

この計画は、現在「手動設定」や「分散した設定」になっている部分を、SAMテンプレートの解析と自動化ツールによって完全に隠蔽し、開発者が **`template.yaml` と Python コードを書くだけでローカル開発が完結する** 状態を目指します。

---

# 🚀 edge-serverless-box マスタープラン: "True SAM Integration"

## 🎯 ビジョン

**"Define once, Run locally."**
AWS SAM (`template.yaml`) に定義された「関数」「API」「データベース」「ストレージ」が、コマンド一発でローカルのマイクロサービス群（Gateway, ScyllaDB, RustFS）に即座に反映される環境。

---

## 🗺️ ロードマップ概要

| フェーズ | 領域 | 実施内容 | 成果物 |
| --- | --- | --- | --- |
| **Phase 1** | **Compute & Network** | `Events` プロパティの解析と `routing.yml` の自動生成 | ルーティング設定の手動管理廃止 |
| **Phase 2** | **Storage & Database** | `Resources` (DynamoDB/S3) の解析と「プロビジョナー」の実装 | DBテーブル・S3バケットの自動作成 |
| **Phase 3** | **Integrated Experience** | CLI (`esb`) の導入とホットリロード | `docker compose` コマンドの隠蔽 |

---

## 🛠️ Phase 1: Compute & Network (Routing Automation)

**課題:** 現在、関数を定義しても `routing.yml` に追記しないとアクセスできない。
**解決策:** `AWS::Serverless::Function` の `Events` (API Gateway) 定義からルーティングを生成する。

### 実装詳細

#### 1. パーサーの拡張 (`tools/generator/parser.py`)

現在の `parse_sam_template` 関数を拡張し、`Events` プロパティを解析します。

```python
# 抽出イメージ
events = props.get("Events", {})
api_routes = []
for event_name, event_props in events.items():
    if event_props.get("Type") == "Api":
        properties = event_props.get("Properties", {})
        api_routes.append({
            "path": properties.get("Path"),
            "method": properties.get("Method"),
            "function_name": function_name  # 論理IDまたはFunctionName
        })

```

#### 2. レンダラーの改修 (`tools/generator/renderer.py`)

抽出した `api_routes` リストを元に、Jinja2 テンプレートを使って `config/routing.yml` を出力する関数 `render_routing_yml` を追加します。

* **入力**: 解析された API ルート情報のリスト
* **出力**: `tests/e2e/config/routing.yml` (または本番用 `config/routing.yml`)

#### 3. ジェネレータへの統合 (`tools/generator/main.py`)

`generate_files` 関数内で上記を呼び出し、Dockerfile 生成と同時にルーティング定義も更新するようにします。

---

## 🏗️ Phase 2: Storage & Database (Infrastructure as Code)

**課題:** DynamoDB や S3 を使う際、コンテナ起動後に手動でテーブル作成やバケット作成が必要（またはアプリコード内で `create_table` している）。
**解決策:** `template.yaml` のリソース定義を読み取り、ローカルの ScyllaDB/RustFS に対して `boto3` で構築を行う「プロビジョナー」を作成する。

### 実装詳細

#### 1. リソース解析の追加 (`tools/generator/parser.py`)

`Resources` ブロックから以下のタイプを抽出します。

* `AWS::DynamoDB::Table`: `TableName`, `KeySchema`, `AttributeDefinitions`, `GlobalSecondaryIndexes` を抽出。
* `AWS::S3::Bucket`: `BucketName` を抽出。

#### 2. インフラ・プロビジョナーの開発 (`tools/provisioner/main.py`)

これは静的な設定ファイル生成ではなく、**実行時スクリプト** として実装します。

* **役割**: `docker compose up` でサービスが立ち上がった後に実行され、SAM 定義と現在の DB/Storage 状態を同期（Create if not exists）します。
* **技術スタック**: `boto3` を使用し、エンドポイントをローカルコンテナに向けます。

```python
# プロビジョナーの擬似コード
def provision_infrastructure(sam_resources):
    # DynamoDB (ScyllaDB) への接続
    ddb = boto3.resource('dynamodb', endpoint_url='http://localhost:8001')
    
    for table_def in sam_resources['dynamodb_tables']:
        try:
            ddb.create_table(
                TableName=table_def['TableName'],
                KeySchema=table_def['KeySchema'],
                # ...
            )
            print(f"✅ Table created: {table_def['TableName']}")
        except ResourceInUseException:
            pass # 既に存在する場合はスキップ（あるいは更新ロジック）

    # S3 (RustFS) への接続
    s3 = boto3.client('s3', endpoint_url='http://localhost:9000')
    # ...同様にバケット作成

```

---

## 💻 Phase 3: Integrated Experience (CLI & Watcher)

**課題:** 開発者が `python tools/generator...` → `docker compose up` → `python tools/provisioner...` と手順を踏む必要がある。
**解決策:** これらをラップする開発用 CLI を提供する。

### 実装詳細

#### 1. CLI ツール (`esb`) の作成

プロジェクトルートに `esb` (Edge Serverless Box) コマンド（Python スクリプトまたはシェルラッパー）を作成します。

**主なコマンド体系:**

* `esb init`: 初期セットアップ（venv作成、依存インストール）
* `esb build`: `template.yaml` から Dockerfile と config を生成 (Phase 1)
* `esb up`: ビルド → `docker compose up -d` → プロビジョニング (Phase 2) まで一括実行
* `esb down`: 環境の停止

#### 2. 開発体験の完成形

開発者のワークフローは以下のようになります。

```bash
# 1. SAMテンプレートに新しい関数とテーブルを追加
vi template.yaml

# 2. コマンド一発で環境に反映
./esb up

# 3. ログを見る
docker logs -f onpre-gateway

```

---

## 📝 技術的な考慮事項・注意点

1. **CloudFormation 固有関数の扱い (`!Ref`, `!Sub`)**
* `TableName: !Ref MyTable` のような記述がある場合、現在の簡易パーサーでは名前が取れません。
* **対策**: パーサーを強化し、論理ID (Logical ID) をデフォルトのテーブル名として扱うか、パラメータ解決ロジック (`_resolve_intrinsic`) を `Resources` 側にも適用する必要があります。


2. **ScyllaDB (Alternator) の互換性**
* ScyllaDB の Alternator は DynamoDB の全機能をサポートしているわけではありません（例: On-Demand Capacity など）。
* **対策**: プロビジョナー側で、ScyllaDB がサポートしていないパラメータ（`BillingMode` 等）は無視して `create_table` を呼ぶような「サニタイズ処理」を入れます。


3. **コンテナ起動待ち (Wait-for-it)**
* `esb up` 実行時、ScyllaDB が完全に起動する前にプロビジョナーが動くとエラーになります。
* **対策**: プロビジョナー内に「ヘルスチェック・リトライループ」を実装し、ポート 8001/9000 が応答し始めるまで待機させます。



## 最初のステップ（Action Item）

まずは **Phase 1（ルーティング自動化）** から着手することをお勧めします。これにより、開発者はすぐに「`template.yaml` を書くだけで API が増える」という快感を得られ、本方針への信頼感が高まるはずです。

1. `tools/generator/parser.py` に `Events` 解析ロジックを追加する。
2. `tools/generator/renderer.py` に `routing.yml` 生成ロジックを追加する。
3. `tests/e2e/template.yaml` の各関数に `Events` プロパティを追記してテストする。