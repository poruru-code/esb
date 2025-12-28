"""
SAM Template Parser

Parse SAM template (YAML) and extract Lambda function information.
Safely handle CloudFormation intrinsic functions (!Sub, !Ref, etc.).
"""

import yaml
from typing import Any


class CfnLoader(yaml.SafeLoader):
    """YAML loader that handles CloudFormation intrinsic functions."""

    pass


def cfn_constructor(loader: yaml.Loader, node: yaml.Node) -> Any:
    """Constructor for CloudFormation tags."""
    if isinstance(node, yaml.ScalarNode):
        return loader.construct_scalar(node)
    elif isinstance(node, yaml.SequenceNode):
        return loader.construct_sequence(node)
    elif isinstance(node, yaml.MappingNode):
        return loader.construct_mapping(node)
    return ""


# Register CloudFormation tags.
for tag in ["!Ref", "!Sub", "!GetAtt", "!ImportValue", "!If", "!Join", "!Select", "!Split"]:
    yaml.add_constructor(tag, cfn_constructor, Loader=CfnLoader)


def parse_sam_template(content: str, parameters: dict | None = None) -> dict:
    """
    Parse a SAM template string and return a list of Lambda functions.

    Args:
        content: SAM template YAML string
        parameters: dict for parameter substitution (optional)

    Returns:
        {
            'functions': [
                {
                    'logical_id': 'HelloFunction',
                    'name': 'lambda-hello',
                    'code_uri': 'functions/hello/',
                    'handler': 'lambda_function.lambda_handler',
                    'runtime': 'python3.12',
                    'environment': {...},
                }
            ]
        }
    """
    if parameters is None:
        parameters = {}

    data = yaml.load(content, Loader=CfnLoader)

    # Get default values from Globals.
    globals_config = data.get("Globals", {}).get("Function", {})
    default_runtime = globals_config.get("Runtime", "python3.12")
    default_handler = globals_config.get("Handler", "lambda_function.lambda_handler")
    default_timeout = globals_config.get("Timeout", 30)
    default_memory = globals_config.get("MemorySize", 128)
    default_layers = globals_config.get("Layers", [])

    functions = []
    resources = data.get("Resources", {})

    for logical_id, resource in resources.items():
        resource_type = resource.get("Type", "")

        # Only target AWS::Serverless::Function.
        if resource_type != "AWS::Serverless::Function":
            continue

        props = resource.get("Properties", {})

        # Get function name (resolve !Sub, etc.).
        function_name = props.get("FunctionName", logical_id)
        function_name = _resolve_intrinsic(function_name, parameters)

        # Code URI.
        code_uri = props.get("CodeUri", "./")
        code_uri = _resolve_intrinsic(code_uri, parameters)
        if not code_uri.endswith("/"):
            code_uri += "/"

        # Handler (from Properties or Globals).
        handler = props.get("Handler", default_handler)

        # Runtime (from Properties or Globals).
        runtime = props.get("Runtime", default_runtime)

        # Environment variables.
        env_vars = props.get("Environment", {}).get("Variables", {})
        # Resolve environment variable values as well.
        resolved_env = {}
        for key, value in env_vars.items():
            resolved_env[key] = _resolve_intrinsic(value, parameters)

        # --- Phase 1: Events (API Gateway) parsing ---
        events = props.get("Events", {})
        api_routes = []
        for event_name, event_props in events.items():
            # Only handle Type: Api (API Gateway).
            if event_props.get("Type") == "Api":
                evt_properties = event_props.get("Properties", {})
                path = evt_properties.get("Path")
                method = evt_properties.get("Method")

                if path and method:
                    api_routes.append({"path": path, "method": method})

        # --- Phase 1.5: Scaling (SAM Standard) parsing ---
        max_capacity = props.get("ReservedConcurrentExecutions")
        provisioned_config = props.get("ProvisionedConcurrencyConfig", {})
        min_capacity = provisioned_config.get("ProvisionedConcurrentExecutions")

        scaling_config = {}
        if max_capacity is not None:
            scaling_config["max_capacity"] = max_capacity
        if min_capacity is not None:
            scaling_config["min_capacity"] = min_capacity

        functions.append(
            {
                "logical_id": logical_id,
                "name": function_name,
                "code_uri": code_uri,
                "handler": handler,
                "runtime": runtime,
                "timeout": props.get("Timeout", default_timeout),
                "memory_size": props.get("MemorySize", default_memory),
                "environment": resolved_env,
                "events": api_routes,
                "scaling": scaling_config,
            }
        )

    # --- Phase 2: Resources & Layers parsing ---
    dynamodb_tables = []
    s3_buckets = []
    layers = {}  # logical_id -> {name, content_uri}

    # Parse LayerVersion first.
    for logical_id, resource in resources.items():
        resource_type = resource.get("Type", "")
        props = resource.get("Properties", {})

        if resource_type == "AWS::Serverless::LayerVersion":
            layer_name = props.get("LayerName", logical_id)
            layer_name = _resolve_intrinsic(layer_name, parameters)
            content_uri = props.get("ContentUri", "./")
            content_uri = _resolve_intrinsic(content_uri, parameters)
            if not content_uri.endswith("/"):
                content_uri += "/"

            layers[logical_id] = {"name": layer_name, "content_uri": content_uri}

        # DynamoDB
        elif resource_type == "AWS::DynamoDB::Table":
            table_name = props.get("TableName", logical_id)
            table_name = _resolve_intrinsic(table_name, parameters)

            dynamodb_tables.append(
                {
                    "TableName": table_name,
                    "KeySchema": props.get("KeySchema"),
                    "AttributeDefinitions": props.get("AttributeDefinitions"),
                    "GlobalSecondaryIndexes": props.get("GlobalSecondaryIndexes"),
                    "BillingMode": props.get("BillingMode", "PROVISIONED"),
                    "ProvisionedThroughput": props.get("ProvisionedThroughput"),
                }
            )

        # S3 Bucket
        elif resource_type == "AWS::S3::Bucket":
            bucket_name = props.get("BucketName", logical_id.lower())
            bucket_name = _resolve_intrinsic(bucket_name, parameters)
            s3_buckets.append({"BucketName": bucket_name})

    # Phase 3: Attach layer information to functions.
    for func in functions:
        # At this point func is a dict.
        logical_id = func["logical_id"]
        resource = resources.get(logical_id, {})
        props = resource.get("Properties", {})

        layer_refs = props.get("Layers", default_layers)
        func_layers = []

        for ref in layer_refs:
            layer_id = ref
            if isinstance(ref, dict) and "Ref" in ref:
                layer_id = ref["Ref"]

            if layer_id in layers:
                func_layers.append(layers[layer_id])

        func["layers"] = func_layers

    return {
        "functions": functions,
        "resources": {
            "dynamodb": dynamodb_tables,
            "s3": s3_buckets,
            "layers": list(layers.values()),
        },
    }


def _resolve_intrinsic(value: Any, parameters: dict) -> str:
    """
    Resolve a CloudFormation intrinsic function.

    Simple implementation: only supports !Sub ${Param} format.
    """
    if not isinstance(value, str):
        return str(value) if value is not None else ""

    # Replace ${Param} format.
    import re

    def replace_param(match):
        param_name = match.group(1)
        return parameters.get(param_name, f"${{{param_name}}}")

    return re.sub(r"\$\{(\w+)\}", replace_param, value)
