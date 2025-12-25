"""
SAM Template Parser

SAMテンプレート(YAML)をパースし、Lambda関数の情報を抽出します。
CloudFormation intrinsic functions (!Sub, !Ref等) を安全に処理します。
"""

import yaml
from typing import Any


class CfnLoader(yaml.SafeLoader):
    """CloudFormation intrinsic functionsを処理するYAMLローダー"""

    pass


def cfn_constructor(loader: yaml.Loader, node: yaml.Node) -> Any:
    """CloudFormation タグ用のコンストラクタ"""
    if isinstance(node, yaml.ScalarNode):
        return loader.construct_scalar(node)
    elif isinstance(node, yaml.SequenceNode):
        return loader.construct_sequence(node)
    elif isinstance(node, yaml.MappingNode):
        return loader.construct_mapping(node)
    return ""


# CloudFormation タグを登録
for tag in ["!Ref", "!Sub", "!GetAtt", "!ImportValue", "!If", "!Join", "!Select", "!Split"]:
    yaml.add_constructor(tag, cfn_constructor, Loader=CfnLoader)


def parse_sam_template(content: str, parameters: dict | None = None) -> dict:
    """
    SAMテンプレート文字列をパースし、Lambda関数のリストを返す

    Args:
        content: SAMテンプレートのYAML文字列
        parameters: パラメータ置換用の辞書（オプション）

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

    # Globalsからデフォルト値を取得
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

        # AWS::Serverless::Function のみ対象
        if resource_type != "AWS::Serverless::Function":
            continue

        props = resource.get("Properties", {})

        # 関数名を取得（!Sub等を解決）
        function_name = props.get("FunctionName", logical_id)
        function_name = _resolve_intrinsic(function_name, parameters)

        # コードURI
        code_uri = props.get("CodeUri", "./")
        code_uri = _resolve_intrinsic(code_uri, parameters)
        if not code_uri.endswith("/"):
            code_uri += "/"

        # ハンドラ（PropertiesまたはGlobalsから）
        handler = props.get("Handler", default_handler)

        # ランタイム（PropertiesまたはGlobalsから）
        runtime = props.get("Runtime", default_runtime)

        # 環境変数
        env_vars = props.get("Environment", {}).get("Variables", {})
        # 環境変数の値も解決
        resolved_env = {}
        for key, value in env_vars.items():
            resolved_env[key] = _resolve_intrinsic(value, parameters)

        # --- Phase 1: Events (API Gateway) 解析 ---
        events = props.get("Events", {})
        api_routes = []
        for event_name, event_props in events.items():
            # Type: Api (API Gateway) のみを対象にする
            if event_props.get("Type") == "Api":
                evt_properties = event_props.get("Properties", {})
                path = evt_properties.get("Path")
                method = evt_properties.get("Method")

                if path and method:
                    api_routes.append({"path": path, "method": method})

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
            }
        )

    # --- Phase 2: Resources & Layers 解析 ---
    dynamodb_tables = []
    s3_buckets = []
    layers = {}  # logical_id -> {name, content_uri}

    # 先に LayerVersion を解析
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

    # Phase 3: Function に Layer 情報を紐付け
    for func in functions:
        # この時点では func は辞書
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
    CloudFormation intrinsic functionを解決する

    簡易実装: !Sub ${Param} 形式のみ対応
    """
    if not isinstance(value, str):
        return str(value) if value is not None else ""

    # ${Param} 形式を置換
    import re

    def replace_param(match):
        param_name = match.group(1)
        return parameters.get(param_name, f"${{{param_name}}}")

    return re.sub(r"\$\{(\w+)\}", replace_param, value)
