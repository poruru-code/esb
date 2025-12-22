# 🏗️ Phase 2: Storage & Database (IaC) 実装計画

## 概要

Lambda関数コード内で `boto3.create_table` 等を書く「命令的」な管理から、SAMテンプレートに基づく「宣言的」な管理へ移行します。

## アーキテクチャ

```mermaid
graph TD
    Template["template.yaml"] --> Parser["Parser (Extended)"]
    Parser --> |Resource Definitions| Provisioner["Provisioner Script (New)"]
    Provisioner --> |Boto3 (Port 8001)| ScyllaDB[(ScyllaDB)]
    Provisioner --> |Boto3 (Port 9000)| RustFS[(RustFS)]
    
    subgraph "Host Machine"
        Template
        Parser
        Provisioner
    end
    
    subgraph "Docker Containers"
        ScyllaDB
        RustFS
    end

```

## 実装ステップ

### Step 1. Parser の拡張 (`tools/generator/parser.py`)

現在の `functions` 抽出に加え、`Resources` ブロックから DynamoDB と S3 の定義を抽出するように拡張します。

**取得すべきプロパティ:**

* **DynamoDB**: `TableName`, `KeySchema`, `AttributeDefinitions`, `GlobalSecondaryIndexes`, `BillingMode`, `ProvisionedThroughput`
* **S3**: `BucketName`

### Step 2. Provisioner の実装 (`tools/provisioner/main.py`)

抽出された定義に基づき、実際にリソースを作成するPythonスクリプトを新規作成します。

**主な機能:**

1. **Wait for Service**: ScyllaDB や RustFS が起動し、リクエストを受け付けられるようになるまでポーリングして待機します。
2. **Idempotency (冪等性)**: 既にテーブルやバケットが存在する場合は作成をスキップ（または差分更新※今回はスキップのみでOK）します。
3. **Parameter Sanitization**: CloudFormation の定義を `boto3` の引数形式に変換します。特に ScyllaDB (Alternator) が厳密でないパラメータの扱いを調整します。

---

## 💻 実装コード案

### 1. `tools/generator/parser.py` (拡張)

返り値の辞書に `resources` キーを追加し、解析結果を含めます。

```python
# (既存の import とクラス定義はそのまま)

def parse_sam_template(content: str, parameters: dict | None = None) -> dict:
    # ... (既存の functions 解析ロジック) ...

    # --- Phase 2: Resources 解析追加 ---
    dynamodb_tables = []
    s3_buckets = []

    for logical_id, resource in resources.items():
        resource_type = resource.get("Type", "")
        props = resource.get("Properties", {})

        # DynamoDB
        if resource_type == "AWS::DynamoDB::Table":
            table_name = props.get("TableName")
            # TableNameがない場合はLogical IDを使用（CloudFormationの挙動に寄せる）
            if not table_name:
                table_name = logical_id
            
            table_name = _resolve_intrinsic(table_name, parameters)
            
            dynamodb_tables.append({
                "TableName": table_name,
                "KeySchema": props.get("KeySchema"),
                "AttributeDefinitions": props.get("AttributeDefinitions"),
                "GlobalSecondaryIndexes": props.get("GlobalSecondaryIndexes"),
                "BillingMode": props.get("BillingMode", "PROVISIONED"),
                "ProvisionedThroughput": props.get("ProvisionedThroughput")
            })

        # S3 Bucket
        elif resource_type == "AWS::S3::Bucket":
            bucket_name = props.get("BucketName")
            if not bucket_name:
                bucket_name = logical_id.lower() # S3は小文字推奨
            
            bucket_name = _resolve_intrinsic(bucket_name, parameters)
            s3_buckets.append({"BucketName": bucket_name})

    # 戻り値に resources を追加
    return {
        "functions": functions,
        "resources": {
            "dynamodb": dynamodb_tables,
            "s3": s3_buckets
        }
    }

```

### 2. `tools/provisioner/main.py` (新規作成)

実際にリソースを作成するスクリプトです。`boto3` が必要になります。

```python
#!/usr/bin/env python3
import sys
import time
import boto3
from botocore.exceptions import ClientError, EndpointConnectionError
from pathlib import Path
import yaml

# プロジェクトルートのパス解決など
sys.path.append(str(Path(__file__).parent.parent.parent))
from tools.generator.parser import parse_sam_template

# 設定（環境変数やConfigから読み込むのが理想ですが、一旦定数定義）
SCYLLADB_ENDPOINT = "http://localhost:8001"
RUSTFS_ENDPOINT = "http://localhost:9000"
AWS_REGION = "ap-northeast-1"
AWS_ACCESS_KEY = "dummy"
AWS_SECRET_KEY = "dummy"

def get_dynamodb_client():
    return boto3.client(
        "dynamodb",
        endpoint_url=SCYLLADB_ENDPOINT,
        region_name=AWS_REGION,
        aws_access_key_id=AWS_ACCESS_KEY,
        aws_secret_access_key=AWS_SECRET_KEY,
    )

def get_s3_client():
    return boto3.client(
        "s3",
        endpoint_url=RUSTFS_ENDPOINT,
        region_name=AWS_REGION,
        aws_access_key_id=AWS_ACCESS_KEY,
        aws_secret_access_key=AWS_SECRET_KEY,
    )

def wait_for_service(client, service_name, max_retries=30):
    """サービスが応答するまで待機"""
    print(f"Waiting for {service_name}...", end="", flush=True)
    for _ in range(max_retries):
        try:
            if service_name == "DynamoDB":
                client.list_tables()
            else:
                client.list_buckets()
            print(" OK!")
            return True
        except (EndpointConnectionError, ClientError):
            time.sleep(1)
            print(".", end="", flush=True)
    print(" Timeout!")
    return False

def provision_dynamodb(tables):
    client = get_dynamodb_client()
    if not wait_for_service(client, "DynamoDB"):
        return

    existing_tables = client.list_tables()["TableNames"]

    for table_def in tables:
        name = table_def["TableName"]
        if name in existing_tables:
            print(f"Example: Table '{name}' already exists. Skipping.")
            continue

        print(f"Creating Table: {name}")
        
        # CloudFormation定義からboto3引数へ変換・サニタイズ
        params = {
            "TableName": name,
            "KeySchema": table_def["KeySchema"],
            "AttributeDefinitions": table_def["AttributeDefinitions"],
            "BillingMode": table_def["BillingMode"]
        }

        # ProvisionedThroughputの調整
        if table_def.get("ProvisionedThroughput"):
            params["ProvisionedThroughput"] = {
                "ReadCapacityUnits": table_def["ProvisionedThroughput"]["ReadCapacityUnits"],
                "WriteCapacityUnits": table_def["ProvisionedThroughput"]["WriteCapacityUnits"]
            }
        elif table_def["BillingMode"] == "PROVISIONED":
            # デフォルト値
            params["ProvisionedThroughput"] = {"ReadCapacityUnits": 1, "WriteCapacityUnits": 1}

        # GSIs
        if table_def.get("GlobalSecondaryIndexes"):
            gsis = []
            for gsi in table_def["GlobalSecondaryIndexes"]:
                gsi_def = {
                    "IndexName": gsi["IndexName"],
                    "KeySchema": gsi["KeySchema"],
                    "Projection": gsi["Projection"]
                }
                # GSI用スループット（簡易処理）
                if gsi.get("ProvisionedThroughput"):
                     gsi_def["ProvisionedThroughput"] = gsi["ProvisionedThroughput"]
                elif table_def["BillingMode"] == "PROVISIONED":
                     gsi_def["ProvisionedThroughput"] = {"ReadCapacityUnits": 1, "WriteCapacityUnits": 1}
                
                gsis.append(gsi_def)
            params["GlobalSecondaryIndexes"] = gsis

        try:
            client.create_table(**params)
            print(f"✅ Created DynamoDB Table: {name}")
        except Exception as e:
            print(f"❌ Failed to create table {name}: {e}")

def provision_s3(buckets):
    client = get_s3_client()
    if not wait_for_service(client, "S3"):
        return

    existing_buckets = [b["Name"] for b in client.list_buckets().get("Buckets", [])]

    for bucket_def in buckets:
        name = bucket_def["BucketName"]
        if name in existing_buckets:
            print(f"Bucket '{name}' already exists. Skipping.")
            continue

        try:
            client.create_bucket(Bucket=name)
            print(f"✅ Created S3 Bucket: {name}")
        except Exception as e:
            print(f"❌ Failed to create bucket {name}: {e}")

def main():
    # テンプレート読み込み (パスは引数等で調整可能にする)
    template_path = Path("tests/e2e/template.yaml")
    if not template_path.exists():
        print("Template not found")
        sys.exit(1)

    with open(template_path, "r") as f:
        content = f.read()
    
    # 解析
    parsed = parse_sam_template(content)
    resources = parsed.get("resources", {})

    # プロビジョニング実行
    if resources.get("dynamodb"):
        provision_dynamodb(resources["dynamodb"])
    
    if resources.get("s3"):
        provision_s3(resources["s3"])

if __name__ == "__main__":
    main()

```

### 次のアクション

1. **依存関係の確認**: `pyproject.toml` に `boto3` が含まれているか確認し、なければ追加してください。
2. **Parser実装**: `tools/generator/parser.py` を上記コード例を参考に修正。
3. **Provisioner作成**: `tools/provisioner/` ディレクトリを作成し、`main.py` を配置。
4. **動作確認**:
```bash
# コンテナ起動
docker compose up -d

# プロビジョニング実行
python -m tools.provisioner.main

```



これで、インフラ構築の自動化（Phase 2）の基盤が整います。