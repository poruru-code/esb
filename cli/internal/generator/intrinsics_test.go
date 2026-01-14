package generator

import (
	"reflect"
	"strings"
	"testing"
)

func TestParserContext_Resolve(t *testing.T) {
	ctx := NewParserContext(map[string]string{
		"MyParam": "Value1",
		"Stage":   "prod",
	})

	tests := []struct {
		name  string
		input any
		want  any
	}{
		{
			name:  "plain string",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "simple ref",
			input: map[string]any{"Ref": "MyParam"},
			want:  "Value1",
		},
		{
			name:  "pseudo param prod",
			input: map[string]any{"Ref": "AWS::Region"},
			want:  "local-Region",
		},
		{
			name:  "simple sub",
			input: map[string]any{"Fn::Sub": "hello-${MyParam}"},
			want:  "hello-Value1",
		},
		{
			name: "sub with map",
			input: map[string]any{"Fn::Sub": []any{
				"hello-${Var}",
				map[string]any{"Var": "World"},
			}},
			want: "hello-World",
		},
		{
			name: "sub with map and ref",
			input: map[string]any{"Fn::Sub": []any{
				"${MyParam}-${Var}",
				map[string]any{"Var": "World"},
			}},
			want: "Value1-World",
		},
		{
			name: "recursive sub vars",
			input: map[string]any{"Fn::Sub": []any{
				"GREET-${Var}",
				map[string]any{"Var": map[string]any{"Ref": "MyParam"}},
			}},
			want: "GREET-Value1",
		},
		{
			name:  "pseudo param prod in sub",
			input: map[string]any{"Fn::Sub": "region is ${AWS::Region}"},
			want:  "region is local-Region",
		},
		{
			name: "join with nested ref",
			input: map[string]any{"Fn::Join": []any{
				"-",
				[]any{"a", map[string]any{"Ref": "MyParam"}, "c1"},
			}},
			want: "a-Value1-c1",
		},
		{
			name:  "getatt string",
			input: map[string]any{"Fn::GetAtt": "MyRes.Arn"},
			want:  "arn:aws:local:Arn:global:MyRes/Arn",
		},
		{
			name:  "getatt list",
			input: map[string]any{"Fn::GetAtt": []any{"MyRes", "Arn"}},
			want:  "arn:aws:local:Arn:global:MyRes/Arn",
		},
		{
			name:  "split",
			input: map[string]any{"Fn::Split": []any{"|", "a|b|c"}},
			want:  []any{"a", "b", "c"},
		},
		{
			name:  "select",
			input: map[string]any{"Fn::Select": []any{1, []any{"a", "b", "c"}}},
			want:  "b",
		},
		{
			name:  "importvalue",
			input: map[string]any{"Fn::ImportValue": "MyExport"},
			want:  "imported-MyExport",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ctx.resolve(tt.input, 0); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("resolve() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParserContext_Conditions(t *testing.T) {
	ctx := NewParserContext(map[string]string{
		"Env": "prod",
	})
	ctx.RawConditions = map[string]any{
		"IsProd": true,
	}

	tests := []struct {
		name  string
		input any
		want  any
	}{
		{
			name:  "equals true",
			input: map[string]any{"Fn::Equals": []any{"${Env}", "prod"}},
			want:  true,
		},
		{
			name:  "equals false",
			input: map[string]any{"Fn::Equals": []any{"${Env}", "dev"}},
			want:  false,
		},
		{
			name: "if true",
			input: map[string]any{"Fn::If": []any{
				"IsProd",
				"prod-value",
				"dev-value",
			}},
			want: "prod-value",
		},
		{
			name: "nested if",
			input: map[string]any{"Fn::If": []any{
				"IsProd",
				map[string]any{"Ref": "Env"},
				"dev",
			}},
			want: "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if strings.HasPrefix(tt.name, "equals") {
				if got := ctx.EvaluateCondition(tt.input); got != tt.want.(bool) {
					t.Errorf("EvaluateCondition() = %v, want %v", got, tt.want)
				}
			} else {
				if got := ctx.resolve(tt.input, 0); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("resolve() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestParseSAMTemplateNesting(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Parameters:
  Env:
    Type: String
Resources:
  MyFunc:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: !Sub "func-${Env}"
      CodeUri: ./
      Handler: index.handler
      Runtime: nodejs18.x
      Environment:
        Variables:
          DB_URL: !Join [ "", [ "http://", !GetAtt MyTable.Arn ] ]
  MyTable:
    Type: AWS::DynamoDB::Table
    Properties:
      TableName: !Ref Env
`
	ctx := map[string]string{"Env": "dev"}
	res, err := ParseSAMTemplate(content, ctx)
	if err != nil {
		t.Fatalf("ParseSAMTemplate failed: %v", err)
	}

	fn := res.Functions[0]
	if fn.Name != "func-dev" {
		t.Errorf("expected FunctionName func-dev, got %s", fn.Name)
	}

	dbURL := fn.Environment["DB_URL"]
	if dbURL != "http://arn:aws:local:Arn:global:MyTable/Arn" {
		t.Errorf("expected DB_URL http://arn:aws:local:Arn:global:MyTable/Arn, got %s", dbURL)
	}

	if len(res.Resources.DynamoDB) == 0 || res.Resources.DynamoDB[0].TableName != "dev" {
		t.Errorf("expected DynamoDB TableName dev, got %+v", res.Resources.DynamoDB)
	}
}

func TestParseSAMTemplateWithConditions(t *testing.T) {
	content := `
AWSTemplateFormatVersion: '2010-09-09'
Parameters:
  Env:
    Type: String
    Default: dev
Conditions:
  IsProd: !Equals [ !Ref Env, "prod" ]
Resources:
  MyFunc:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: !If [ IsProd, "prod-func", "dev-func" ]
      CodeUri: ./
      Handler: index.handler
      Runtime: nodejs18.x
`
	// Test Dev
	res, err := ParseSAMTemplate(content, map[string]string{"Env": "dev"})
	if err != nil {
		t.Fatalf("ParseSAMTemplate dev failed: %v", err)
	}
	if res.Functions[0].Name != "dev-func" {
		t.Errorf("expected dev-func, got %s", res.Functions[0].Name)
	}

	// Test Prod
	res, err = ParseSAMTemplate(content, map[string]string{"Env": "prod"})
	if err != nil {
		t.Fatalf("ParseSAMTemplate prod failed: %v", err)
	}
	if res.Functions[0].Name != "prod-func" {
		t.Errorf("expected prod-func, got %s", res.Functions[0].Name)
	}
}
