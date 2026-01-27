import logging
import os
import sys

import boto3
import yaml
from botocore.exceptions import ClientError

# Configure basic logging.
# sitecustomize.py will redirect this to VictoriaLogs if VICTORIALOGS_URL is set.
logging.basicConfig(
    level=logging.INFO, format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger("esb.provisioner")

MANIFEST_PATH = "/app/config/resources.yml"


def load_manifest():
    if not os.path.exists(MANIFEST_PATH):
        logger.warning(f"Manifest not found at {MANIFEST_PATH}. Skipping provisioning.")
        return {}

    with open(MANIFEST_PATH, "r") as f:
        return yaml.safe_load(f)


def provision_dynamodb(ddb_client, tables):
    for table in tables:
        name = table.get("TableName")
        if not name:
            continue

        logger.info(f"Provisioning DynamoDB table: {name}")
        try:
            # DynamoDB create_table expects PascalCase arguments which match our spec
            ddb_client.create_table(**table)
            logger.info(f"Successfully created table: {name}")
        except ddb_client.exceptions.ResourceInUseException:
            logger.info(f"Table already exists: {name}")
        except Exception as e:
            logger.error(f"Failed to create table {name}: {e}")
            raise


def provision_s3(s3_client, buckets):
    for bucket in buckets:
        name = bucket.get("BucketName")
        if not name:
            continue

        logger.info(f"Provisioning S3 bucket: {name}")
        try:
            # create_bucket in boto3 is slightly different from SAM spec
            # (SAM uses BucketName, boto3 uses Bucket)
            s3_client.create_bucket(Bucket=name)

            # Apply LifecycleConfiguration if present
            lifecycle = bucket.get("LifecycleConfiguration")
            if lifecycle and lifecycle.get("Rules"):
                if should_skip_lifecycle():
                    logger.warning("Skipping lifecycle configuration for %s", name)
                    lifecycle = None
            if lifecycle and lifecycle.get("Rules"):
                rules = []
                for rule in lifecycle["Rules"]:
                    boto_rule = {
                        "Status": rule.get("Status", "Enabled"),
                        "Filter": {"Prefix": rule.get("Prefix", "")},
                    }
                    if "Id" in rule:
                        boto_rule["ID"] = rule["Id"]
                    if "ExpirationInDays" in rule:
                        boto_rule["Expiration"] = {"Days": int(rule["ExpirationInDays"])}
                    rules.append(boto_rule)

                if rules:
                    logger.info(f"Applying lifecycle configuration to {name}")
                    s3_client.put_bucket_lifecycle_configuration(
                        Bucket=name, LifecycleConfiguration={"Rules": rules}
                    )

            logger.info(f"Successfully created bucket: {name}")
        except ClientError as e:
            code = e.response.get("Error", {}).get("Code")
            if code in ("BucketAlreadyOwnedByYou", "BucketAlreadyExists"):
                logger.info(f"Bucket already exists: {name}")
            else:
                logger.error(f"Failed to create bucket {name}: {e}")
                raise
        except Exception as e:
            logger.error(f"Failed to create bucket {name}: {e}")
            raise


def should_skip_lifecycle() -> bool:
    if os.getenv("ESB_FORCE_S3_LIFECYCLE", "").strip():
        return False
    if os.getenv("ESB_SKIP_S3_LIFECYCLE", "").strip():
        return True
    endpoint = os.getenv("S3_ENDPOINT", "").strip().lower()
    return "s3-storage" in endpoint or "rustfs" in endpoint


def main():
    try:
        manifest = load_manifest()

        # sitecustomize.py handles endpoint redirection via env vars:
        # DYNAMODB_ENDPOINT -> boto3.client("dynamodb")
        # S3_ENDPOINT -> boto3.client("s3")
        ddb = boto3.client("dynamodb")
        s3 = boto3.client("s3")

        dynamodb_tables = manifest.get("DynamoDB", [])
        if dynamodb_tables:
            provision_dynamodb(ddb, dynamodb_tables)

        s3_buckets = manifest.get("S3", [])
        if s3_buckets:
            provision_s3(s3, s3_buckets)

        logger.info("Provisioning completed successfully.")

    except Exception as e:
        logger.error(f"Provisioning failed: {e}")
        sys.exit(1)


if __name__ == "__main__":
    main()
