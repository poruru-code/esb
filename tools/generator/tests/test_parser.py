from tools.generator.parser import parse_sam_template


class TestSamParser:
    """SAMテンプレートパーサーのテスト"""

    def test_parse_simple_function(self):
        """シンプルな関数定義をパースできる"""
        sam_content = """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Resources:
  HelloFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-hello
      CodeUri: functions/hello/
      Handler: lambda_function.lambda_handler
      Runtime: python3.12
"""
        result = parse_sam_template(sam_content)

        assert len(result["functions"]) == 1
        func = result["functions"][0]
        assert func["name"] == "lambda-hello"
        assert func["code_uri"] == "functions/hello/"
        assert func["handler"] == "lambda_function.lambda_handler"
        assert func["runtime"] == "python3.12"

    def test_parse_function_with_environment(self):
        """環境変数を含む関数をパースできる"""
        sam_content = """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Resources:
  S3TestFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-s3-test
      CodeUri: functions/s3-test/
      Handler: lambda_function.lambda_handler
      Runtime: python3.12
      Environment:
        Variables:
          S3_ENDPOINT: "http://onpre-storage:9000"
          BUCKET_NAME: "test-bucket"
"""
        result = parse_sam_template(sam_content)

        assert len(result["functions"]) == 1
        func = result["functions"][0]
        assert func["environment"]["S3_ENDPOINT"] == "http://onpre-storage:9000"
        assert func["environment"]["BUCKET_NAME"] == "test-bucket"

    def test_parse_globals(self):
        """Globalsセクションからデフォルト値を取得できる"""
        sam_content = """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Globals:
  Function:
    Runtime: python3.12
    Handler: lambda_function.lambda_handler
    Timeout: 30

Resources:
  HelloFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-hello
      CodeUri: functions/hello/
"""
        result = parse_sam_template(sam_content)

        func = result["functions"][0]
        # Globals から継承
        assert func["runtime"] == "python3.12"
        assert func["handler"] == "lambda_function.lambda_handler"

    def test_skip_non_function_resources(self):
        """Function以外のリソースはスキップする"""
        sam_content = """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Resources:
  HelloFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-hello
      CodeUri: functions/hello/
      Runtime: python3.12
  
  MyLayer:
    Type: AWS::Serverless::LayerVersion
    Properties:
      LayerName: my-layer
      ContentUri: layers/my-layer/
"""
        result = parse_sam_template(sam_content)

        # Function のみ抽出
        assert len(result["functions"]) == 1
        assert result["functions"][0]["name"] == "lambda-hello"

    def test_parse_function_with_events(self):
        """Events (API Gateway) を含む関数をパースできる"""
        sam_content = """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Resources:
  HelloFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-hello
      CodeUri: functions/hello/
      Handler: lambda_function.lambda_handler
      Runtime: python3.12
      Events:
        ApiEvent:
          Type: Api
          Properties:
            Path: /api/hello
            Method: post
"""
        result = parse_sam_template(sam_content)

        assert len(result["functions"]) == 1
        func = result["functions"][0]
        assert "events" in func
        assert len(func["events"]) == 1
        assert func["events"][0]["path"] == "/api/hello"
        assert func["events"][0]["method"] == "post"
