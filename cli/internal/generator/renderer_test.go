// Where: cli/internal/generator/renderer_test.go
// What: Tests for generator renderers.
// Why: Ensure output formats stay stable during Go migration.
package generator

import (
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
	"github.com/poruru/edge-serverless-box/meta"
	"gopkg.in/yaml.v3"
)

func TestRenderDockerfileSimple(t *testing.T) {
	fn := FunctionSpec{
		Name:    "lambda-hello",
		CodeURI: "functions/hello/",
		Handler: "lambda_function.lambda_handler",
		Runtime: "python3.12",
	}
	dockerConfig := DockerConfig{
		SitecustomizeSource: "cli/internal/generator/assets/site-packages/sitecustomize.py",
	}

	content, err := RenderDockerfile(fn, dockerConfig, "", "latest")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expectedBase := "FROM " + meta.ImagePrefix + "-lambda-base:latest"
	if !strings.Contains(content, expectedBase) {
		t.Fatalf("unexpected base image, expected %s, got: %s", expectedBase, content)
	}
	if !strings.Contains(content, "COPY cli/internal/generator/assets/site-packages/sitecustomize.py") {
		t.Fatalf("expected sitecustomize copy")
	}
	if !strings.Contains(content, "COPY functions/hello/") {
		t.Fatalf("expected code copy in dockerfile")
	}
	if !strings.Contains(content, `CMD [ "lambda_function.lambda_handler" ]`) {
		t.Fatalf("expected handler in dockerfile")
	}
}

func TestRenderDockerfileWithRequirementsAndLayers(t *testing.T) {
	fn := FunctionSpec{
		Name:            "lambda-hello",
		CodeURI:         "functions/hello/",
		Handler:         "lambda_function.lambda_handler",
		Runtime:         "python3.12",
		HasRequirements: true,
		Layers: []manifest.LayerSpec{
			{Name: "common-layer", ContentURI: "functions/lambda-hello/layers/common"},
		},
	}

	content, err := RenderDockerfile(fn, DockerConfig{}, "", "latest")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(content, "pip install -r") {
		t.Fatalf("expected requirements install")
	}
	if !strings.Contains(content, "COPY functions/lambda-hello/layers/common/ /opt/") {
		t.Fatalf("expected layer copy")
	}
}

func TestRenderFunctionsYml(t *testing.T) {
	functions := []FunctionSpec{
		{
			Name:      "Lambda-Hello",
			ImageName: "lambda-hello",
			Environment: map[string]string{
				"S3_ENDPOINT": "http://esb-storage:9000",
			},
			Scaling: ScalingSpec{
				MaxCapacity: intPtr(5),
				MinCapacity: intPtr(1),
			},
			Timeout:    10,
			MemorySize: 256,
		},
	}

	content, err := RenderFunctionsYml(functions, "", "latest")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}

	if _, ok := parsed["defaults"]; !ok {
		t.Fatalf("expected defaults section")
	}
	functionsNode, ok := parsed["functions"].(map[string]any)
	if !ok || functionsNode["Lambda-Hello"] == nil {
		t.Fatalf("expected Lambda-Hello entry")
	}

	entry, ok := functionsNode["Lambda-Hello"].(map[string]any)
	if !ok {
		t.Fatalf("expected Lambda-Hello config map")
	}
	if _, ok := entry["image"]; ok {
		t.Fatalf("unexpected image entry in functions.yml: %v", entry["image"])
	}
}

func TestRenderRoutingYml(t *testing.T) {
	functions := []FunctionSpec{
		{
			Name: "lambda-hello",
			Events: []EventSpec{
				{Path: "/api/hello", Method: "post"},
			},
		},
	}

	content, err := RenderRoutingYml(functions)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}
	routes, ok := parsed["routes"].([]any)
	if !ok || len(routes) != 1 {
		t.Fatalf("expected routes entry")
	}
}

func TestRenderFunctionsYmlRequiresImageName(t *testing.T) {
	functions := []FunctionSpec{
		{
			Name: "lambda-missing-image",
		},
	}
	if _, err := RenderFunctionsYml(functions, "", "latest"); err == nil {
		t.Fatalf("expected error for missing image name")
	}
}

func TestRenderResourcesYml(t *testing.T) {
	spec := manifest.ResourcesSpec{
		DynamoDB: []manifest.DynamoDBSpec{
			{
				TableName: "test-table",
				KeySchema: []manifest.DynamoDBKeySchema{
					{AttributeName: "PK", KeyType: "HASH"},
				},
			},
		},
		S3: []manifest.S3Spec{
			{BucketName: "test-bucket"},
		},
	}

	content, err := RenderResourcesYml(spec)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(content, "TableName: test-table") {
		t.Fatalf("expected test-table in content, got: %s", content)
	}
	if !strings.Contains(content, "BucketName: test-bucket") {
		t.Fatalf("expected test-bucket in content, got: %s", content)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("yaml unmarshal failed: %v", err)
	}
	resourcesNode, ok := parsed["resources"].(map[string]any)
	if !ok {
		t.Fatalf("expected resources root in yaml")
	}
	dynamo, ok := resourcesNode["dynamodb"].([]any)
	if !ok || len(dynamo) != 1 {
		t.Fatalf("unexpected dynamodb resources content")
	}
	table, ok := dynamo[0].(map[string]any)
	if !ok || table["TableName"] != "test-table" {
		t.Fatalf("unexpected dynamodb table content")
	}
}

func intPtr(value int) *int {
	return &value
}
