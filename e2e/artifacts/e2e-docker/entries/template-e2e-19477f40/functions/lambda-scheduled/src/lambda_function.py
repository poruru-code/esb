import json
import os
from datetime import datetime

import boto3


def lambda_handler(event, context):
    print(f"Scheduled function triggered with event: {json.dumps(event)}")

    table_name = "e2e-test-table"
    endpoint_url = os.environ.get("DYNAMODB_ENDPOINT")

    dynamodb = boto3.resource("dynamodb", endpoint_url=endpoint_url, region_name="ap-northeast-1")
    table = dynamodb.Table(table_name)

    # Write a record to indicate execution
    # We use a fixed ID so we can just check if it's there, or we can use time to see multiple runs
    item_id = "scheduled-run"
    table.put_item(Item={"id": item_id, "last_run": datetime.utcnow().isoformat(), "event": event})

    return {
        "statusCode": 200,
        "body": json.dumps({"message": "Successfully updated schedule record"}),
    }
