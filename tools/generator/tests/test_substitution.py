
import pytest
from tools.generator import parser

@pytest.fixture
def sample_template():
    return """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Globals:
    Function:
        Timeout: 30

Resources:
  MyLayer:
    Type: AWS::Serverless::LayerVersion
    Properties:
      LayerName: !Sub layer-${Prefix}
      ContentUri: ./layers/${Prefix}/

  MyFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: !Sub func-${Prefix}
      CodeUri: ./src/${Prefix}/
      Handler: app.handler
      Runtime: python3.12
      Layers:
        - !Ref MyLayer
"""

def test_parse_sam_template_with_substitution(sample_template):
    """Ensure parameter substitution applies to CodeUri, LayerName, and ContentUri."""
    params = {"Prefix": "prod"}
    
    parsed = parser.parse_sam_template(sample_template, parameters=params)
    functions = parsed["functions"]
    layers = parsed["resources"]["layers"]
    
    # 1. Function CodeUri.
    func = functions[0]
    assert func["name"] == "func-prod"
    assert func["code_uri"] == "./src/prod/"
    
    # 2. Layer Name & ContentUri.
    # Layer resolution in parser yields a list of dicts.
    # Verify the layers resource itself.
    assert len(layers) == 1
    layer = layers[0]
    assert layer["name"] == "layer-prod"
    assert layer["content_uri"] == "./layers/prod/"
    
    # 3. Function linked layers.
    assert len(func["layers"]) == 1
    linked_layer = func["layers"][0]
    assert linked_layer["name"] == "layer-prod"
    assert linked_layer["content_uri"] == "./layers/prod/"

def test_resolve_intrinsic_regex():
    """Regex test for _resolve_intrinsic (internal but important)."""
    from tools.generator.parser import _resolve_intrinsic
    
    params = {"Env": "dev", "Region": "ap-northeast-1"}
    
    # Single param
    assert _resolve_intrinsic("app-${Env}", params) == "app-dev"
    
    # Multiple params
    assert _resolve_intrinsic("${Env}-${Region}", params) == "dev-ap-northeast-1"
    
    # No match -> keep as is
    assert _resolve_intrinsic("${Unknown}", params) == "${Unknown}"
