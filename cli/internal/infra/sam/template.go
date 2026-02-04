// Where: cli/internal/infra/sam/template.go
// What: SAM template decoding and resource extraction env.
// Why: Isolate schema decoding from generator and provisioner packages.
package sam

import (
	"fmt"

	samparser "github.com/poruru-code/aws-sam-parser-go/parser"
	"github.com/poruru-code/aws-sam-parser-go/schema"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"
)

// Template contains decoded SAM template fields used by the generator.
type Template struct {
	Resources map[string]any
}

// FunctionProperties captures relevant AWS::Serverless::Function properties.
type FunctionProperties struct {
	FunctionName                 any `json:"FunctionName,omitempty"`
	CodeURI                      any `json:"CodeUri,omitempty"`
	Handler                      any `json:"Handler,omitempty"`
	Runtime                      any `json:"Runtime,omitempty"`
	Timeout                      any `json:"Timeout,omitempty"`
	MemorySize                   any `json:"MemorySize,omitempty"`
	Events                       any `json:"Events,omitempty"`
	ReservedConcurrentExecutions any `json:"ReservedConcurrentExecutions,omitempty"`
	ProvisionedConcurrencyConfig any `json:"ProvisionedConcurrencyConfig,omitempty"`
	Layers                       any `json:"Layers,omitempty"`
	Architectures                any `json:"Architectures,omitempty"`
	Environment                  any `json:"Environment,omitempty"`
	RuntimeManagementConfig      any `json:"RuntimeManagementConfig,omitempty"`
}

// DecodeTemplate converts a resolved SAM template into a Template struct.
func DecodeTemplate(resolved map[string]any) (Template, error) {
	var model schema.SamModel
	if err := samparser.Decode(resolved, &model, nil); err != nil {
		return Template{}, fmt.Errorf("decode template: %w", err)
	}
	return Template{Resources: model.Resources}, nil
}

// DecodeFunctionProps decodes Lambda function properties into a typed struct.
func DecodeFunctionProps(props map[string]any) (FunctionProperties, error) {
	var spec FunctionProperties
	if err := samparser.Decode(props, &spec, nil); err != nil {
		return FunctionProperties{}, fmt.Errorf("decode function properties: %w", err)
	}
	return spec, nil
}

// DecodeMap converts a value into map form using the SAM decoder.
func DecodeMap(value any) (map[string]any, error) {
	var out map[string]any
	if err := samparser.Decode(value, &out, nil); err != nil {
		return nil, fmt.Errorf("decode map: %w", err)
	}
	return out, nil
}

// DecodeDynamoDBProps decodes DynamoDB table properties into manifest specs.
func DecodeDynamoDBProps(props map[string]any) (manifest.DynamoDBSpec, error) {
	var spec manifest.DynamoDBSpec
	if err := samparser.Decode(props, &spec, nil); err != nil {
		return manifest.DynamoDBSpec{}, fmt.Errorf("decode dynamodb properties: %w", err)
	}
	return spec, nil
}

// DecodeS3BucketProps decodes S3 bucket properties into manifest specs.
func DecodeS3BucketProps(props map[string]any) (manifest.S3Spec, error) {
	var spec manifest.S3Spec
	if err := samparser.Decode(props, &spec, nil); err != nil {
		return manifest.S3Spec{}, fmt.Errorf("decode s3 properties: %w", err)
	}
	return spec, nil
}
