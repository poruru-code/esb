from tools.generator.parser import parse_sam_template


class TestSamParser:
    """Tests for the SAM template parser."""

    def test_parse_simple_function(self):
        """Parse a simple function definition."""
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
        """Parse a function with environment variables."""
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
          S3_ENDPOINT: "http://esb-storage:9000"
          BUCKET_NAME: "test-bucket"
"""
        result = parse_sam_template(sam_content)

        assert len(result["functions"]) == 1
        func = result["functions"][0]
        assert func["environment"]["S3_ENDPOINT"] == "http://esb-storage:9000"
        assert func["environment"]["BUCKET_NAME"] == "test-bucket"

    def test_parse_globals(self):
        """Get default values from the Globals section."""
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
        # Inherited from Globals.
        assert func["runtime"] == "python3.12"
        assert func["handler"] == "lambda_function.lambda_handler"

    def test_skip_non_function_resources(self):
        """Skip resources that are not Functions."""
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

        # Extract only Function resources.
        assert len(result["functions"]) == 1
        assert result["functions"][0]["name"] == "lambda-hello"

    def test_parse_function_with_events(self):
        """Parse a function that includes API Gateway events."""
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

    def test_parse_function_with_scaling(self):
        """Parse scaling settings (SAM standard properties)."""
        sam_content = """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Resources:
  EchoFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-echo
      ReservedConcurrentExecutions: 5
      ProvisionedConcurrencyConfig:
        ProvisionedConcurrentExecutions: 2
"""
        result = parse_sam_template(sam_content)

        assert len(result["functions"]) == 1
        func = result["functions"][0]
        assert func["scaling"]["max_capacity"] == 5
        assert func["scaling"]["min_capacity"] == 2

    def test_parse_resources(self):
        """Parse DynamoDB and S3 resources."""
        sam_content = """
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31

Resources:
  MyTable:
    Type: AWS::DynamoDB::Table
    Properties:
      TableName: my-test-table
      AttributeDefinitions:
        - AttributeName: id
          AttributeType: S
      KeySchema:
        - AttributeName: id
          KeyType: HASH
      BillingMode: PAY_PER_REQUEST

  MyBucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: my-test-bucket

  LogicalBucket:
    Type: AWS::S3::Bucket
"""
        result = parse_sam_template(sam_content)

        assert "resources" in result
        resources = result["resources"]

        # DynamoDB verification.
        assert len(resources["dynamodb"]) == 1
        table = resources["dynamodb"][0]
        assert table["TableName"] == "my-test-table"
        assert table["BillingMode"] == "PAY_PER_REQUEST"

        # S3 verification.
        assert len(resources["s3"]) == 2
        bucket1 = next(b for b in resources["s3"] if b["BucketName"] == "my-test-bucket")
        assert bucket1 is not None

        # Verify the case where the logical ID becomes the bucket name.
        bucket2 = next(b for b in resources["s3"] if b["BucketName"] == "logicalbucket")
        assert bucket2 is not None
