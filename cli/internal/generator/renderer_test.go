// Where: cli/internal/generator/renderer_test.go
// What: Tests for generator renderers.
// Why: Ensure output formats stay stable during Go migration.
package generator

import (
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/manifest"
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
	if !strings.Contains(content, "FROM esb-lambda-base:latest") {
		t.Fatalf("unexpected base image: %s", content)
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
			Name: "lambda-hello",
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
	if !ok || functionsNode["lambda-hello"] == nil {
		t.Fatalf("expected lambda-hello entry")
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

func intPtr(value int) *int {
	return &value
}
