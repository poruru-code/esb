// Where: cli/internal/generator/validator_test.go
// What: Tests for SAM schema validation.
package generator

import "testing"

func TestValidateSAMTemplateAcceptsValidFunction(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Resources:
  Fn:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: simple
      CodeUri: functions/simple/
      Handler: handler.handler
      Runtime: python3.12
`
	if _, err := validateSAMTemplate([]byte(content)); err != nil {
		t.Fatalf("expected valid template, got %v", err)
	}
}

func TestValidateSAMTemplateEnvironmentNumbers(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Globals:
  Function:
    Environment:
      Variables:
        TIMEOUT: 30
Resources:
  Fn:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: numeric-env
      CodeUri: functions/simple/
      Handler: handler.handler
      Runtime: python3.12
`
	if _, err := validateSAMTemplate([]byte(content)); err != nil {
		t.Fatalf("expected numeric environment to validate, got %v", err)
	}
}
