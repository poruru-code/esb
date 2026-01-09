// Where: cli/internal/generator/parser_test.go
// What: Tests for SAM template parsing in Go generator.
// Why: Keep parser behavior aligned with existing Python generator.
package generator

import "testing"

func TestParseSAMTemplateSimpleFunction(t *testing.T) {
	content := `
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
`

	result, err := ParseSAMTemplate(content, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(result.Functions))
	}
	fn := result.Functions[0]
	if fn.Name != "lambda-hello" {
		t.Fatalf("unexpected name: %s", fn.Name)
	}
	if fn.CodeURI != "functions/hello/" {
		t.Fatalf("unexpected code uri: %s", fn.CodeURI)
	}
	if fn.Handler != "lambda_function.lambda_handler" {
		t.Fatalf("unexpected handler: %s", fn.Handler)
	}
	if fn.Runtime != "python3.12" {
		t.Fatalf("unexpected runtime: %s", fn.Runtime)
	}
}

func TestParseSAMTemplateGlobalsDefaults(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Globals:
  Function:
    Runtime: python3.11
    Handler: app.handler
    Timeout: 25
    MemorySize: 256
Resources:
  HelloFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-hello
      CodeUri: functions/hello/
`

	result, err := ParseSAMTemplate(content, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	fn := result.Functions[0]
	if fn.Runtime != "python3.11" {
		t.Fatalf("unexpected runtime: %s", fn.Runtime)
	}
	if fn.Handler != "app.handler" {
		t.Fatalf("unexpected handler: %s", fn.Handler)
	}
	if fn.Timeout != 25 {
		t.Fatalf("unexpected timeout: %d", fn.Timeout)
	}
	if fn.MemorySize != 256 {
		t.Fatalf("unexpected memory size: %d", fn.MemorySize)
	}
}

func TestParseSAMTemplateGlobalsEnvironment(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Globals:
  Function:
    Environment:
      Variables:
        GLOBAL_ONLY: global
        SHARED: global
Resources:
  HelloFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-hello
      CodeUri: functions/hello/
      Environment:
        Variables:
          SHARED: local
          LOCAL_ONLY: local
`

	result, err := ParseSAMTemplate(content, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	fn := result.Functions[0]
	if fn.Environment["GLOBAL_ONLY"] != "global" {
		t.Fatalf("expected global env to be applied")
	}
	if fn.Environment["LOCAL_ONLY"] != "local" {
		t.Fatalf("expected local env to be applied")
	}
	if fn.Environment["SHARED"] != "local" {
		t.Fatalf("expected local env to override global")
	}
}

func TestParseSAMTemplateEventsAndScaling(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Resources:
  ApiFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-api
      CodeUri: functions/api/
      Events:
        ApiEvent:
          Type: Api
          Properties:
            Path: /api/hello
            Method: post
      ReservedConcurrentExecutions: 5
      ProvisionedConcurrencyConfig:
        ProvisionedConcurrentExecutions: 2
`

	result, err := ParseSAMTemplate(content, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	fn := result.Functions[0]
	if len(fn.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(fn.Events))
	}
	if fn.Events[0].Path != "/api/hello" || fn.Events[0].Method != "post" {
		t.Fatalf("unexpected event: %+v", fn.Events[0])
	}
	if fn.Scaling.MaxCapacity == nil || *fn.Scaling.MaxCapacity != 5 {
		t.Fatalf("unexpected max capacity: %+v", fn.Scaling.MaxCapacity)
	}
	if fn.Scaling.MinCapacity == nil || *fn.Scaling.MinCapacity != 2 {
		t.Fatalf("unexpected min capacity: %+v", fn.Scaling.MinCapacity)
	}
}

func TestParseSAMTemplateResourcesAndLayers(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Globals:
  Function:
    Layers:
      - !Ref CommonLayer
Resources:
  CommonLayer:
    Type: AWS::Serverless::LayerVersion
    Properties:
      LayerName: common-layer
      ContentUri: layers/common/
  MyBucket:
    Type: AWS::S3::Bucket
    Properties:
      BucketName: my-bucket
  MyTable:
    Type: AWS::DynamoDB::Table
    Properties:
      TableName: my-table
  AppFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-app
      CodeUri: functions/app/
`

	result, err := ParseSAMTemplate(content, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Resources.S3) != 1 || result.Resources.S3[0].BucketName != "my-bucket" {
		t.Fatalf("unexpected s3 resources: %+v", result.Resources.S3)
	}
	if len(result.Resources.DynamoDB) != 1 || result.Resources.DynamoDB[0].TableName != "my-table" {
		t.Fatalf("unexpected dynamodb resources: %+v", result.Resources.DynamoDB)
	}
	fn := result.Functions[0]
	if len(fn.Layers) != 1 || fn.Layers[0].Name != "common-layer" {
		t.Fatalf("unexpected layers: %+v", fn.Layers)
	}
}

func TestResolveIntrinsicSubstitution(t *testing.T) {
	params := map[string]string{"Prefix": "prod"}
	value := resolveIntrinsic("func-${Prefix}", params)
	if value != "func-prod" {
		t.Fatalf("unexpected substitution: %s", value)
	}
}

func TestParseSAMTemplateArchitecturesAndRuntimeManagementConfig(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Globals:
  Function:
    Architectures:
      - arm64
    RuntimeManagementConfig:
      UpdateRuntimeOn: Auto
Resources:
  SharedLayer:
    Type: AWS::Serverless::LayerVersion
    Properties:
      LayerName: shared
      ContentUri: layers/shared/
      CompatibleArchitectures:
        - x86_64
        - arm64
  DefaultFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: default
      CodeUri: functions/default/
      Handler: handler.default
      Runtime: python3.12
  OverrideFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: override
      CodeUri: functions/override/
      Handler: handler.override
      Runtime: python3.12
      Architectures:
        - x86_64
      RuntimeManagementConfig:
        UpdateRuntimeOn: Update
`
	result, err := ParseSAMTemplate(content, nil)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if len(result.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(result.Functions))
	}

	find := func(name string) *FunctionSpec {
		for i := range result.Functions {
			if result.Functions[i].Name == name {
				return &result.Functions[i]
			}
		}
		return nil
	}

	defaultFn := find("default")
	overrideFn := find("override")
	if defaultFn == nil || overrideFn == nil {
		t.Fatalf("functions missing: default=%v override=%v", defaultFn, overrideFn)
	}
	if len(defaultFn.Architectures) != 1 || defaultFn.Architectures[0] != "arm64" {
		t.Fatalf("unexpected default architectures: %v", defaultFn.Architectures)
	}
	if defaultFn.RuntimeManagementConfig.UpdateRuntimeOn != "Auto" {
		t.Fatalf("unexpected runtime management: %+v", defaultFn.RuntimeManagementConfig)
	}
	if len(overrideFn.Architectures) != 1 || overrideFn.Architectures[0] != "x86_64" {
		t.Fatalf("unexpected override architectures: %v", overrideFn.Architectures)
	}
	if overrideFn.RuntimeManagementConfig.UpdateRuntimeOn != "Update" {
		t.Fatalf("unexpected override runtime management: %+v", overrideFn.RuntimeManagementConfig)
	}
	if len(result.Resources.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(result.Resources.Layers))
	}
	layer := result.Resources.Layers[0]
	if len(layer.CompatibleArchitectures) != 2 {
		t.Fatalf("expected 2 compatible architectures, got %d", len(layer.CompatibleArchitectures))
	}
}
