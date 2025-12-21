"""
Lambda Layer Library - オンプレミス互換ストレージ/DB接続ユーティリティ

Usage:
    from s3_util import init_storage, get_object, put_object
    from dynamodb_util import init_database, get_item, put_item
"""

from .s3_util import init_storage, get_object, put_object, list_objects, create_bucket
from .dynamodb_util import (
    init_database,
    init_database_resource,
    get_item,
    put_item,
    query,
    create_table,
)

__all__ = [
    # S3互換
    "init_storage",
    "get_object",
    "put_object",
    "list_objects",
    "create_bucket",
    # DynamoDB互換
    "init_database",
    "init_database_resource",
    "get_item",
    "put_item",
    "query",
    "create_table",
]
