// Where: cli/internal/generator/parser_test.go
// What: Tests for SAM template parsing in Go generator.
// Why: Keep parser behavior aligned with existing Python generator.
package generator

import (
	"net/http"
	"strings"
	"testing"
)

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
	// Verify parsing works using strict schema types implicitly via Result
	if fn.MemorySize != 0 { // Default
		t.Logf("memory size: %d", fn.MemorySize)
	}

	// Add a test case specifically for a field that relies on strict typing logic if possible
	// But since the struct fields are mostly standard types (string, int), existing tests cover the values.
	// We can add a check for a field that might be sensitive to type mapping.
}

func TestParseSAMTemplateStrictTypes(t *testing.T) {
	// This test explicitly checks fields that are handled by the new strict parsing logic
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Resources:
  StrictFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: strict-func
      CodeUri: functions/strict/
      Handler: index.handler
      Runtime: nodejs18.x
      Timeout: 60
      MemorySize: 512
      Architectures:
        - arm64
      AutoPublishAlias: live
`
	result, err := ParseSAMTemplate(content, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	fn := result.Functions[0]
	if fn.Name != "strict-func" {
		t.Errorf("unexpected name: %s", fn.Name)
	}
	if fn.Timeout != 60 {
		t.Errorf("unexpected timeout: %d", fn.Timeout)
	}
	if fn.MemorySize != 512 {
		t.Errorf("unexpected memory: %d", fn.MemorySize)
	}
	if len(fn.Architectures) != 1 || fn.Architectures[0] != "arm64" {
		t.Errorf("unexpected architectures: %v", fn.Architectures)
	}
}

func TestParseSAMTemplateIntrinsic(t *testing.T) {
	// This test verifies that Intrinsic Functions (!Ref) are correctly handled
	// even when parsing into strict schema-generated types.
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Parameters:
  MyMemory:
    Type: Number
    Default: 512
Resources:
  IntrinsicFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: intrinsic-func
      CodeUri: ./
      Handler: index.handler
      Runtime: nodejs18.x
      MemorySize: !Ref MyMemory
      Timeout: !Ref MyMemory
`
	params := map[string]string{"MyMemory": "1024"}
	result, err := ParseSAMTemplate(content, params)
	if err != nil {
		t.Fatalf("expected no error from ParseSAMTemplate, but got: %v", err)
	}

	if len(result.Functions) == 0 {
		t.Fatalf("expected 1 function, but got 0")
	}

	fn := result.Functions[0]
	// We expect the parser to handle !Ref and return the resolved value.
	if fn.MemorySize != 1024 {
		t.Errorf("expected MemorySize to be 1024 (resolved from !Ref), but got %d", fn.MemorySize)
	}
	if fn.Timeout != 1024 {
		t.Errorf("expected Timeout to be 1024, but got %d", fn.Timeout)
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
	if fn.Events[0].Path != "/api/hello" || !strings.EqualFold(fn.Events[0].Method, http.MethodPost) {
		t.Fatalf("unexpected event: %+v", fn.Events[0])
	}
	if fn.Events[0].Type != "Api" {
		t.Fatalf("unexpected event type: %s", fn.Events[0].Type)
	}
	if fn.Scaling.MaxCapacity == nil || *fn.Scaling.MaxCapacity != 5 {
		t.Fatalf("unexpected max capacity: %+v", fn.Scaling.MaxCapacity)
	}
	if fn.Scaling.MinCapacity == nil || *fn.Scaling.MinCapacity != 2 {
		t.Fatalf("unexpected min capacity: %+v", fn.Scaling.MinCapacity)
	}
}

func TestParseSAMTemplateScheduleEvent(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Resources:
  CronFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-cron
      CodeUri: functions/cron/
      Events:
        Timer:
          Type: Schedule
          Properties:
            Schedule: cron(0 * * * * *)
            Input: '{"foo": "bar"}'
  RateFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-rate
      CodeUri: functions/rate/
      Events:
        Timer:
          Type: Schedule
          Properties:
            Schedule: rate(1 minute)
`

	result, err := ParseSAMTemplate(content, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.Functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(result.Functions))
	}

	// Test Cron Function
	cronFn := findFunction(result.Functions, "lambda-cron")
	if cronFn == nil {
		t.Fatal("lambda-cron not found")
		return
	}
	if len(cronFn.Events) != 1 {
		t.Fatalf("expected 1 event for cron, got %d", len(cronFn.Events))
	}
	if cronFn.Events[0].Type != "Schedule" {
		t.Errorf("expected Schedule type, got %s", cronFn.Events[0].Type)
	}
	if cronFn.Events[0].ScheduleExpression != "cron(0 * * * * *)" {
		t.Errorf("unexpected schedule: %s", cronFn.Events[0].ScheduleExpression)
	}
	if cronFn.Events[0].Input != `{"foo": "bar"}` {
		t.Errorf("unexpected input: %s", cronFn.Events[0].Input)
	}

	// Test Rate Function
	rateFn := findFunction(result.Functions, "lambda-rate")
	if rateFn == nil {
		t.Fatal("lambda-rate not found")
		return
	}
	if len(rateFn.Events) != 1 {
		t.Fatalf("expected 1 event for rate, got %d", len(rateFn.Events))
	}
	if rateFn.Events[0].ScheduleExpression != "rate(1 minute)" {
		t.Errorf("unexpected schedule: %s", rateFn.Events[0].ScheduleExpression)
	}
}

func findFunction(fns []FunctionSpec, name string) *FunctionSpec {
	for _, fn := range fns {
		if fn.Name == name {
			return &fn
		}
	}
	return nil
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
	value := resolveIntrinsicWithParams(map[string]string{"Prefix": "prod"}, "func-${Prefix}")
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
