import gzip
import json
import os
import urllib.request

# Source: AWS CloudFormation Resource Specification
# URL: https://dnwj8swjjbsbt.cloudfront.net/latest/gzip/CloudFormationResourceSpecification.json
SPEC_URL = (
    "https://dnwj8swjjbsbt.cloudfront.net/latest/gzip/CloudFormationResourceSpecification.json"
)
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
EXTENSIONS_DIR = os.path.join(BASE_DIR, "extensions")

TARGET_RESOURCES = ["AWS::S3::Bucket", "AWS::DynamoDB::Table"]


def download_and_extract():
    print(f"Downloading spec from {SPEC_URL}...")
    try:
        with urllib.request.urlopen(SPEC_URL) as response:
            with gzip.GzipFile(fileobj=response) as uncompressed:
                spec_data = json.load(uncompressed)
    except Exception as e:
        print(f"Failed to download spec: {e}")
        return

    resource_types = spec_data.get("ResourceTypes", {})

    if not os.path.exists(EXTENSIONS_DIR):
        os.makedirs(EXTENSIONS_DIR)

    for resource_name in TARGET_RESOURCES:
        if resource_name in resource_types:
            print(f"Extracting {resource_name}...")
            resource_def = resource_types[resource_name]
            # Inject typeName for generate.py consumption
            resource_def["typeName"] = resource_name

            # Resolve PropertyTypes if needed?
            # The CF spec splits definitions into "ResourceTypes" and "PropertyTypes".
            # SAM schema generator likely expects fully resolved properties
            # or we need to include PropertyTypes as definitions.
            # However, my generate.py structure wraps it in "Properties".
            # If the properties reference "PropertyTypes", we need those definitions too.
            # But wait, sam.schema.json has its own definitions.
            # If S3 Bucket uses "AWS::S3::Bucket.LifecycleConfiguration",
            # that is in PropertyTypes.

            # Simple approach: Check if we can dump PropertyTypes
            # as standard definitions too? Or just extraction is enough?
            # Let's extract only the resource first.
            # If references are broken (e.g. they point to
            # #/definitions/AWS::S3::Bucket.LifecycleConfiguration),
            # we might need to verify if `generate.py` or the
            # `schema-generate` tool handles them.
            # Typically CF spec references are generic.

            # The CF spec uses local references like "PropertyTypes":
            # { "AWS::S3::Bucket.LifecycleConfiguration": ... }
            # We might need to extract dependent property types and
            # put them in the same file or separate files.
            # For this MVP, let's see what happens if we just save the resource.
            # Actually, robust way is to download PropertyTypes relevant to the resource.

            filename = resource_name.replace("::", "-").lower() + ".json"
            filepath = os.path.join(EXTENSIONS_DIR, filename)

            # For better self-containment, we might want to resolve dependencies,
            # but let's start simple.
            with open(filepath, "w") as f:
                json.dump(resource_def, f, indent=2)
            print(f"Saved to {filepath}")
        else:
            print(f"Warning: {resource_name} not found in spec.")

    # Also extract relevant PropertyTypes?
    # AWS::S3::Bucket needs "AWS::S3::Bucket.LifecycleConfiguration", etc.
    # Instead of complex resolution, let's just dump ALL PropertyTypes
    # relevant to the targets into the same file?
    # Use a merged structure?
    # My generate.py merges "extensions/*.json" into "definitions".
    # So if I create "aws-s3-bucket-props.json" with
    # typeName="AWS::S3::Bucket.LifecycleConfiguration", it might work
    # if I adjust generate.py to handle non-resource definitions.

    # Or, I can extract PropertyTypes and save them as individual files too.
    # Let's try to extract all PropertyTypes that start with the Resource Name.

    property_types = spec_data.get("PropertyTypes", {})
    for prop_type_name in property_types:
        for target in TARGET_RESOURCES:
            if prop_type_name.startswith(target):
                print(f"Extracting PropertyType {prop_type_name}...")
                prop_def = property_types[prop_type_name]
                prop_def["typeName"] = prop_type_name

                filename = prop_type_name.replace("::", "-").replace(".", "-").lower() + ".json"
                filepath = os.path.join(EXTENSIONS_DIR, filename)
                with open(filepath, "w") as f:
                    json.dump(prop_def, f, indent=2)


if __name__ == "__main__":
    download_and_extract()
