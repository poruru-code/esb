"""
DynamoDB-compatible Lambda (ScyllaDB).

A simple Lambda function providing DynamoDB API operations.
"""

import json
import logging
import time
import uuid

import boto3
from common.utils import create_response, handle_ping, parse_event_body

logger = logging.getLogger()
logger.setLevel(logging.INFO)

TABLE_NAME = "e2e-test-table"


def lambda_handler(event, context):
    # RIE Heartbeat
    if ping_response := handle_ping(event):
        return ping_response

    logger.info(f"Received event: {json.dumps(event)}")

    body = parse_event_body(event)
    action = body.get("action", "put_get")

    try:
        dynamodb = boto3.client("dynamodb")

        if action == "put_get":
            # Existing behavior: PutItem -> GetItem
            item_id = str(uuid.uuid4())
            timestamp = int(time.time())
            item = {
                "id": {"S": item_id},
                "timestamp": {"N": str(timestamp)},
                "message": {"S": body.get("message", "Hello from ScyllaDB Lambda")},
            }

            logger.info(f"Putting item: {item}")
            dynamodb.put_item(TableName=TABLE_NAME, Item=item)

            logger.info(f"Getting item: {item_id}")
            response = dynamodb.get_item(TableName=TABLE_NAME, Key={"id": {"S": item_id}})
            retrieved = response.get("Item", {})

            return create_response(
                body={"success": True, "item_id": item_id, "retrieved_item": retrieved}
            )

        elif action == "get":
            # GetItem only
            item_id = body.get("id")
            if not item_id:
                return create_response(
                    status_code=400, body={"success": False, "error": "id is required"}
                )
            response = dynamodb.get_item(TableName=TABLE_NAME, Key={"id": {"S": item_id}})
            item = response.get("Item")
            return create_response(body={"success": True, "item": item, "found": item is not None})

        elif action == "put":
            # PutItem only
            item_id = body.get("id", str(uuid.uuid4()))
            timestamp = int(time.time())
            item = {
                "id": {"S": item_id},
                "timestamp": {"N": str(timestamp)},
                "message": {"S": body.get("message", "Hello from ScyllaDB Lambda")},
            }
            dynamodb.put_item(TableName=TABLE_NAME, Item=item)
            return create_response(body={"success": True, "item_id": item_id})

        elif action == "update":
            # UpdateItem
            item_id = body.get("id")
            if not item_id:
                return create_response(
                    status_code=400, body={"success": False, "error": "id is required"}
                )
            new_message = body.get("message", "Updated message")
            dynamodb.update_item(
                TableName=TABLE_NAME,
                Key={"id": {"S": item_id}},
                UpdateExpression="SET message = :msg, #ts = :ts",
                ExpressionAttributeNames={"#ts": "timestamp"},
                ExpressionAttributeValues={
                    ":msg": {"S": new_message},
                    ":ts": {"N": str(int(time.time()))},
                },
            )
            return create_response(body={"success": True, "item_id": item_id})

        elif action == "delete":
            # DeleteItem
            item_id = body.get("id")
            if not item_id:
                return create_response(
                    status_code=400, body={"success": False, "error": "id is required"}
                )
            dynamodb.delete_item(TableName=TABLE_NAME, Key={"id": {"S": item_id}})
            return create_response(body={"success": True, "item_id": item_id, "deleted": True})

        else:
            return create_response(
                status_code=400, body={"success": False, "error": f"Unknown action: {action}"}
            )

    except Exception as e:
        logger.error(f"Error: {str(e)}")
        return create_response(status_code=500, body={"success": False, "error": str(e)})
