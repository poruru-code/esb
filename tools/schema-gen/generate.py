import json
import os
import subprocess
import sys

# --- 設定 ---
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
SCHEMA_FILE = os.path.join(BASE_DIR, "sam.schema.json")
# Extensions are extracted from AWS CloudFormation Resource Specification
# See tools/schema-gen/extract_specs.py for details.
# Source URL: https://dnwj8swjjbsbt.cloudfront.net/latest/gzip/CloudFormationResourceSpecification.json
EXTENSIONS_DIR = os.path.join(BASE_DIR, "extensions")
TEMP_SCHEMA_FILE = os.path.join(BASE_DIR, "sam.schema.merged.temp.json")

# 出力先: cli/internal/generator/schema/sam_generated.go
# ツールからルートに戻ってパスを解決
PROJECT_ROOT = os.path.abspath(os.path.join(BASE_DIR, "../../"))
OUTPUT_FILE = os.path.join(PROJECT_ROOT, "cli/internal/generator/schema/sam_generated.go")

# 削除対象の汎用的なtitle（構造体名の衝突・不安定化の原因）
TITLES_TO_REMOVE = {"Properties", "Type", "Auth"}


def load_json(filepath):
    with open(filepath, "r", encoding="utf-8") as f:
        return json.load(f)


def sanitize_title(title):
    return title.replace("::", "").replace(".", "")


def make_ref_schema(ref_name, prefix=""):
    """
    参照名から $ref スキーマを作成する。
    """
    target = ref_name
    if prefix and not ref_name.startswith("AWS::") and "::" not in ref_name:
        # プレフィックス付与 (e.g. CorsConfiguration -> AWS::S3::Bucket.CorsConfiguration)
        target = f"{prefix}.{ref_name}"

    # AWS::S3::Bucket.LifecycleConfiguration -> AWSS3BucketLifecycleConfiguration
    # schema-generate は通常Definitionsのキーを使うので、
    # 参照先もDefinitionsのキーに合わせる必要がある？
    # create_struct_titleと同じ変換をしておくのが安全か、
    # あるいはDefinitionsのキーそのものを使うか。
    # ここでは definitions のキー (元のType名) を指すようにし、
    # generate.py側でそれを登録する設計にする。

    return {"$ref": f"#/definitions/{target}"}


def convert_cf_type(prop_def, full_type_name_prefix=""):
    """
    CloudFormation Specのプロパティ定義をJSON Schemaに変換する
    """
    schema = {}

    # Documentation
    if "Documentation" in prop_def:
        schema["description"] = prop_def["Documentation"]

    # PrimitiveType
    if "PrimitiveType" in prop_def:
        # User requested permissive types to handle Intrinsic Functions (Ref, Fn::Sub, etc.)
        # CloudFormation templates often use {"Ref": "..."} (Object) in place of primitives.
        # By not setting a specific "type", we encourage the generator to use interface{} (Any).
        pass

    # Type (Complex or List/Map)
    elif "Type" in prop_def:
        type_name = prop_def["Type"]
        if type_name == "List":
            schema["type"] = "array"
            # Handle ItemType or PrimitiveItemType
            if "ItemType" in prop_def:
                # Reference to another definition
                item_type = prop_def["ItemType"]
                # If item types are not primitives (don't start with AWS::)
                # and match our heuristic
                if (
                    full_type_name_prefix
                    and not item_type.startswith("AWS::")
                    and item_type != "Tag"
                ):
                    # Property Types are usually flattened under the Resource
                    # e.g. AWS::S3::Bucket.SomeType
                    # We use the resource_prefix to fully qualify the reference
                    resource_prefix = full_type_name_prefix.split(".")[0]
                    full_type = f"{resource_prefix}.{item_type}"
                    sanitized = sanitize_title(full_type)
                    schema["items"] = {"$ref": f"#/definitions/{sanitized}"}
                else:
                    sanitized = sanitize_title(item_type)
                    schema["items"] = {"$ref": f"#/definitions/{sanitized}"}
            elif "PrimitiveItemType" in prop_def:
                # List of primitives - keep it open as well
                schema["items"] = {}
        elif type_name == "Map":
            schema["type"] = "object"
            # Map of ...? CF spec doesn't always say.
            # Use ItemType if present
            if "ItemType" in prop_def:
                item_type = prop_def["ItemType"]
                if (
                    full_type_name_prefix
                    and not item_type.startswith("AWS::")
                    and item_type != "Tag"
                ):
                    resource_prefix = full_type_name_prefix.split(".")[0]
                    full_type = f"{resource_prefix}.{item_type}"
                    sanitized = sanitize_title(full_type)
                    schema["additionalProperties"] = {"$ref": f"#/definitions/{sanitized}"}
                else:
                    sanitized = sanitize_title(item_type)
                    schema["additionalProperties"] = {"$ref": f"#/definitions/{sanitized}"}
            else:
                schema["additionalProperties"] = True
        else:
            # Reference to another custom type
            if full_type_name_prefix and not type_name.startswith("AWS::") and type_name != "Tag":
                resource_prefix = full_type_name_prefix.split(".")[0]
                full_type = f"{resource_prefix}.{type_name}"
                sanitized = sanitize_title(full_type)
                schema["$ref"] = f"#/definitions/{sanitized}"
            else:
                sanitized = sanitize_title(type_name)
                schema["$ref"] = f"#/definitions/{sanitized}"

    return schema


def convert_cf_properties(props, required_props, parent_name):
    """
    CF Properties -> JSON Schema Properties
    """
    properties = {}
    required = []

    for name, defn in props.items():
        properties[name] = convert_cf_type(defn, parent_name)
        if name in required_props:
            required.append(name)
        elif defn.get("Required") is True:
            required.append(name)

    return properties, required


def merge_extensions(base_schema):
    """
    extensions/ ディレクトリ内のJSONファイルをマージする
    """
    if "definitions" not in base_schema:
        base_schema["definitions"] = {}

    for filename in os.listdir(EXTENSIONS_DIR):
        if filename.endswith(".json"):
            schema_file = os.path.join(EXTENSIONS_DIR, filename)
            try:
                ext_data = load_json(schema_file)

                # we defer type_name extraction to below
                # props = ext_data.get("Properties", {})

                # We need to process nested property types which are defined
                # at the top level of spec usually?
                # No, standard CF spec has "PropertyTypes" and "ResourceTypes".
                # My extract_specs.py extracts "PropertyTypes" into separate keys
                # in the JSON file possibly?
                # Let's verify extract_specs logic.
                # Actually extract_specs.py saves one file per Resource,
                # containing "Properties" of the resource.
                # But what about sub-types?
                # extract_specs.py iterates over PropertyTypes and saves them too?
                # Let's check extract_specs.py content or output logic.

                # Assuming extract_specs.py creates files where key is the type name.
                # Actually, the file content format from extract_specs.py is:
                # { "ResourceType": "...", "Properties": ... }
                # OR for property types:
                # { "ResourceType": "AWS::S3::Bucket.CorsConfiguration", "Properties": ... }

                # So we can just trust "ResourceType" as the key.

                type_name = ext_data.get("ResourceType") or ext_data.get("typeName")
                if not type_name:
                    raise ValueError(f"Missing typeName or ResourceType in {schema_file}")

                struct_title = sanitize_title(type_name)

                is_resource = (
                    "::" in type_name and "." not in type_name.split("::")[-1]
                )  # Simple check for top-level resource

                # Convert Properties to JSON Schema
                props, req = convert_cf_properties(ext_data.get("Properties", {}), [], type_name)

                definition = {
                    "type": "object",
                    "title": struct_title,
                    "properties": props,
                    "required": req,
                    "additionalProperties": False,
                }

                if is_resource:
                    definition = {
                        "type": "object",
                        "title": struct_title,
                        "properties": {
                            "Type": {"enum": [type_name]},
                            "Properties": {
                                "type": "object",
                                "title": f"{struct_title}Properties",
                                "properties": props,
                                "required": req,
                                "additionalProperties": False,
                            },
                        },
                        "additionalProperties": False,
                    }

                # Use struct_title as key to avoid issues with colons in definition names
                base_schema["definitions"][struct_title] = definition

            except Exception as e:
                print(f"Error merging {schema_file}: {e}")


def sanitize_schema(obj):
    """
    再帰的に走査し、問題のある title 属性を削除する
    """
    if isinstance(obj, dict):
        if "title" in obj and obj["title"] in TITLES_TO_REMOVE:
            del obj["title"]

        for value in obj.values():
            sanitize_schema(value)

    elif isinstance(obj, list):
        for item in obj:
            sanitize_schema(item)


def main():
    print(f"Reading {SCHEMA_FILE}...")
    if not os.path.exists(SCHEMA_FILE):
        print("Error: Base schema file not found.")
        sys.exit(1)

    schema_data = load_json(SCHEMA_FILE)

    if os.path.exists(EXTENSIONS_DIR):
        merge_extensions(schema_data)
    else:
        print(f"Extensions directory not found at {EXTENSIONS_DIR}, skipping merge.")

    print("Sanitizing schema (removing duplicate titles)...")
    sanitize_schema(schema_data)

    print(f"Generating Go code to {OUTPUT_FILE}...")
    cmd = [
        "schema-generate",
        "-p",
        "schema",
        "-o",
        OUTPUT_FILE,
        "-s",  # Skip marshalling code generation
        TEMP_SCHEMA_FILE,
    ]

    try:
        # Add a dummy definition to force generation of resources if they are being tree-shaken
        if "definitions" not in schema_data:
            schema_data["definitions"] = {}

        schema_data["definitions"]["ForceGeneration"] = {
            "type": "object",
            "properties": {
                "S3Bucket": {"$ref": "#/definitions/AWSS3Bucket"},
                "DynamoDBTable": {"$ref": "#/definitions/AWSDynamoDBTable"},
            },
        }

        # Add Tag definition as it is missing from base schema but used by resources
        if "Tag" not in schema_data["definitions"]:
            schema_data["definitions"]["Tag"] = {
                "type": "object",
                "properties": {"Key": {"type": "string"}, "Value": {"type": "string"}},
                "required": ["Key", "Value"],
                "additionalProperties": False,
            }

        # Ensure root references ForceGeneration so it's not tree-shaken
        if "properties" not in schema_data:
            schema_data["properties"] = {}
        schema_data["properties"]["ForceGeneration"] = {"$ref": "#/definitions/ForceGeneration"}

        print(f"Writing temp schema to {TEMP_SCHEMA_FILE}...")
        with open(TEMP_SCHEMA_FILE, "w", encoding="utf-8") as f:
            json.dump(schema_data, f, indent=2)

        print(f"Generating Go code to {OUTPUT_FILE}...")

        # Run schema-generate and capture output
        result = subprocess.run(cmd, check=False, capture_output=True, text=True)

        print("--- schema-generate stdout ---")
        print(result.stdout)
        print("--- schema-generate stderr ---")
        print(result.stderr)

        if result.returncode != 0:
            print(f"Generation failed with code {result.returncode}")
            sys.exit(1)

        print("Generation successful.")

        with open(OUTPUT_FILE, "r", encoding="utf-8") as f:
            lines = f.readlines()

        with open(OUTPUT_FILE, "w", encoding="utf-8") as f:
            for line in lines:
                if '"encoding/json"' in line:
                    continue
                f.write(line)
        print("Post-processing successful.")

    except Exception as e:
        print(f"An unexpected error occurred: {e}")
        sys.exit(1)
    finally:
        # 一時ファイルの削除
        # if os.path.exists(TEMP_SCHEMA_FILE):
        #      os.remove(TEMP_SCHEMA_FILE)
        pass


if __name__ == "__main__":
    main()
