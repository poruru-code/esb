// Where: cli/internal/infra/sam/template_functions_test.go
// What: Focused unit tests for SAM function parsing helpers.
// Why: Lock parser behavior while refactoring helper file boundaries.
package sam

import (
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
)

func TestParseFunctionsParsesServerlessAndLambdaImage(t *testing.T) {
	resources := map[string]any{
		"ZipFn": map[string]any{
			"Type": "AWS::Serverless::Function",
			"Properties": map[string]any{
				"FunctionName": "zip-fn",
				"CodeUri":      "functions/zip",
				"Handler":      "app.handler",
				"Runtime":      "python3.12",
				"Layers":       []any{map[string]any{"Ref": "CommonLayer"}},
				"Events": map[string]any{
					"ApiEvent": map[string]any{
						"Type": "Api",
						"Properties": map[string]any{
							"Path":   "/zip",
							"Method": "GET",
						},
					},
				},
				"ReservedConcurrentExecutions": 3,
				"ProvisionedConcurrencyConfig": map[string]any{
					"ProvisionedConcurrentExecutions": 1,
				},
			},
		},
		"ImageFn": map[string]any{
			"Type": "AWS::Lambda::Function",
			"Properties": map[string]any{
				"FunctionName": "image-fn",
				"PackageType":  "Image",
				"Code": map[string]any{
					"ImageUri": "public.ecr.aws/example/repo:latest",
				},
				"Timeout":    120,
				"MemorySize": 1024,
			},
		},
	}
	defaults := functionDefaults{
		Runtime:             DefaultLambdaRuntime,
		Handler:             DefaultLambdaHandler,
		Timeout:             DefaultLambdaTimeout,
		Memory:              DefaultLambdaMemory,
		EnvironmentDefaults: map[string]string{"GLOBAL": "1"},
	}
	layerMap := map[string]manifest.LayerSpec{
		"CommonLayer": {
			Name:       "common",
			ContentURI: "layers/common/",
		},
	}

	functions, err := parseFunctions(resources, defaults, layerMap)
	if err != nil {
		t.Fatalf("parseFunctions error: %v", err)
	}
	if len(functions) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(functions))
	}

	zipFn := findFunctionByName(functions, "zip-fn")
	if zipFn == nil {
		t.Fatal("zip-fn not found")
	}
	if zipFn.CodeURI != "functions/zip/" {
		t.Fatalf("zip-fn CodeURI=%q", zipFn.CodeURI)
	}
	if len(zipFn.Events) != 1 || zipFn.Events[0].Method != "get" {
		t.Fatalf("zip-fn events unexpected: %+v", zipFn.Events)
	}
	if zipFn.Scaling.MaxCapacity == nil || *zipFn.Scaling.MaxCapacity != 3 {
		t.Fatalf("zip-fn max scaling unexpected: %+v", zipFn.Scaling.MaxCapacity)
	}
	if zipFn.Scaling.MinCapacity == nil || *zipFn.Scaling.MinCapacity != 1 {
		t.Fatalf("zip-fn min scaling unexpected: %+v", zipFn.Scaling.MinCapacity)
	}
	if len(zipFn.Layers) != 1 || zipFn.Layers[0].Name != "common" {
		t.Fatalf("zip-fn layers unexpected: %+v", zipFn.Layers)
	}
	if zipFn.Environment["GLOBAL"] != "1" {
		t.Fatalf("zip-fn merged env missing GLOBAL: %+v", zipFn.Environment)
	}

	imageFn := findFunctionByName(functions, "image-fn")
	if imageFn == nil {
		t.Fatal("image-fn not found")
	}
	if imageFn.ImageSource != "public.ecr.aws/example/repo:latest" {
		t.Fatalf("image-fn ImageSource=%q", imageFn.ImageSource)
	}
	if imageFn.CodeURI != "" {
		t.Fatalf("image-fn CodeURI should be empty, got %q", imageFn.CodeURI)
	}
}

func TestParseFunctionsRejectsUnresolvedImageURI(t *testing.T) {
	resources := map[string]any{
		"ImageFn": map[string]any{
			"Type": "AWS::Serverless::Function",
			"Properties": map[string]any{
				"FunctionName": "image-fn",
				"PackageType":  "Image",
				"ImageUri":     "${Unresolved}:latest",
			},
		},
	}

	_, err := parseFunctions(resources, functionDefaults{}, nil)
	if err == nil {
		t.Fatal("expected unresolved image uri error")
	}
}

func TestParseEventsAndScalingHelpers(t *testing.T) {
	events := parseEvents(map[string]any{
		"Api": map[string]any{
			"Type": "Api",
			"Properties": map[string]any{
				"Path":   "/v1/items",
				"Method": "POST",
			},
		},
		"Schedule": map[string]any{
			"Type": "Schedule",
			"Properties": map[string]any{
				"Schedule": "rate(1 minute)",
				"Input":    `{"k":"v"}`,
			},
		},
	})
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	scaling := parseScaling(map[string]any{
		"ReservedConcurrentExecutions": 8,
		"ProvisionedConcurrencyConfig": map[string]any{
			"ProvisionedConcurrentExecutions": 4,
		},
	})
	if scaling.MaxCapacity == nil || *scaling.MaxCapacity != 8 {
		t.Fatalf("max scaling unexpected: %+v", scaling.MaxCapacity)
	}
	if scaling.MinCapacity == nil || *scaling.MinCapacity != 4 {
		t.Fatalf("min scaling unexpected: %+v", scaling.MinCapacity)
	}
}

func TestLayerAndRuntimeHelpers(t *testing.T) {
	layerMap := map[string]manifest.LayerSpec{
		"A": {Name: "A"},
		"B": {Name: "B"},
	}
	layers := collectLayers([]any{"A", map[string]any{"Ref": "B"}}, layerMap)
	if len(layers) != 2 {
		t.Fatalf("expected 2 layers, got %d", len(layers))
	}

	rm := runtimeManagementFromConfig(map[string]any{"UpdateRuntimeOn": "Auto"})
	if rm.UpdateRuntimeOn != "Auto" {
		t.Fatalf("runtime management unexpected: %+v", rm)
	}

	if !hasUnresolvedImageURI("${IMAGE}") {
		t.Fatal("expected unresolved image uri")
	}
	if hasUnresolvedImageURI("public.ecr.aws/example/repo:latest") {
		t.Fatal("expected resolved image uri")
	}
}

func findFunctionByName(functions []template.FunctionSpec, name string) *template.FunctionSpec {
	for i := range functions {
		if functions[i].Name == name {
			return &functions[i]
		}
	}
	return nil
}
