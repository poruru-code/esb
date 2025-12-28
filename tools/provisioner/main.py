#!/usr/bin/env python3
import sys
import time
import boto3
from botocore.exceptions import ClientError, EndpointConnectionError
from pathlib import Path

# Resolve the project root path.
project_root = Path(__file__).parent.parent.parent
sys.path.append(str(project_root))
from tools.generator.parser import parse_sam_template  # noqa: E402

# Default settings.
SCYLLADB_ENDPOINT = "http://localhost:8001"
RUSTFS_ENDPOINT = "http://localhost:9000"
AWS_REGION = "ap-northeast-1"
RUSTFS_ACCESS_KEY = "rustfsadmin"
RUSTFS_SECRET_KEY = "rustfsadmin"
DYNAMODB_ACCESS_KEY = "dummy"
DYNAMODB_SECRET_KEY = "dummy"


def get_dynamodb_client():
    return boto3.client(
        "dynamodb",
        endpoint_url=SCYLLADB_ENDPOINT,
        region_name=AWS_REGION,
        aws_access_key_id=DYNAMODB_ACCESS_KEY,
        aws_secret_access_key=DYNAMODB_SECRET_KEY,
    )


def get_s3_client():
    return boto3.client(
        "s3",
        endpoint_url=RUSTFS_ENDPOINT,
        region_name=AWS_REGION,
        aws_access_key_id=RUSTFS_ACCESS_KEY,
        aws_secret_access_key=RUSTFS_SECRET_KEY,
    )


def wait_for_service(client, service_name, max_retries=30):
    """Wait until the service responds."""
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
    # Guard against list_tables failures in test environments (mocked, etc.).
    try:
        existing_tables = client.list_tables()["TableNames"]
    except Exception:
        existing_tables = []

    for table_def in tables:
        name = table_def["TableName"]
        if name in existing_tables:
            print(f"Table '{name}' already exists. Skipping.")
            continue

        print(f"Creating DynamoDB Table: {name}")

        # Convert/sanitize for boto3 parameters.
        params = {
            "TableName": name,
            "KeySchema": table_def["KeySchema"],
            "AttributeDefinitions": table_def["AttributeDefinitions"],
            "BillingMode": table_def["BillingMode"],
        }

        # Adjust ProvisionedThroughput (not needed for PAY_PER_REQUEST).
        if table_def["BillingMode"] == "PROVISIONED":
            if table_def.get("ProvisionedThroughput"):
                params["ProvisionedThroughput"] = {
                    "ReadCapacityUnits": table_def["ProvisionedThroughput"]["ReadCapacityUnits"],
                    "WriteCapacityUnits": table_def["ProvisionedThroughput"]["WriteCapacityUnits"],
                }
            else:
                params["ProvisionedThroughput"] = {"ReadCapacityUnits": 1, "WriteCapacityUnits": 1}

        # GlobalSecondaryIndexes
        if table_def.get("GlobalSecondaryIndexes"):
            gsis = []
            for gsi in table_def["GlobalSecondaryIndexes"]:
                gsi_def = {
                    "IndexName": gsi["IndexName"],
                    "KeySchema": gsi["KeySchema"],
                    "Projection": gsi["Projection"],
                }
                if table_def["BillingMode"] == "PROVISIONED":
                    if gsi.get("ProvisionedThroughput"):
                        gsi_def["ProvisionedThroughput"] = gsi["ProvisionedThroughput"]
                    else:
                        gsi_def["ProvisionedThroughput"] = {
                            "ReadCapacityUnits": 1,
                            "WriteCapacityUnits": 1,
                        }
                gsis.append(gsi_def)
            params["GlobalSecondaryIndexes"] = gsis

        try:
            client.create_table(**params)
            print(f"✅ Created DynamoDB Table: {name}")
        except Exception as e:
            print(f"❌ Failed to create table {name}: {e}")


def provision_s3(buckets):
    client = get_s3_client()
    try:
        existing_buckets = [b["Name"] for b in client.list_buckets().get("Buckets", [])]
    except Exception:
        existing_buckets = []

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


def main(template_path=None):
    # Load template.
    if template_path is None:
        template_path = project_root / "tests/fixtures/template.yaml"
    else:
        template_path = Path(template_path)

    if not template_path.exists():
        print(f"Template not found: {template_path}")
        sys.exit(1)

    with open(template_path, "r", encoding="utf-8") as f:
        content = f.read()

    # Parse.
    parsed = parse_sam_template(content)
    resources = parsed.get("resources", {})

    # Run provisioning.
    if resources.get("dynamodb"):
        provision_dynamodb(resources["dynamodb"])

    if resources.get("s3"):
        provision_s3(resources["s3"])


if __name__ == "__main__":
    main()
