// Where: cli/internal/domain/template/renderer_snapshot_test.go
// What: Snapshot tests for renderer outputs.
// Why: Detect unintended template changes with stable fixtures.
package template

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRendererSnapshots(t *testing.T) {
	t.Run("dockerfile", func(t *testing.T) {
		fn := FunctionSpec{
			Name:    "lambda-hello",
			CodeURI: "functions/hello/",
			Handler: "lambda_function.lambda_handler",
			Runtime: "python3.12",
		}
		dockerConfig := DockerConfig{
			SitecustomizeSource: "runtime/python/extensions/sitecustomize/site-packages/sitecustomize.py",
		}
		content, err := RenderDockerfile(fn, dockerConfig, "", "latest")
		if err != nil {
			t.Fatalf("RenderDockerfile: %v", err)
		}
		assertSnapshot(t, "dockerfile_simple.golden", content)
	})

	t.Run("functions", func(t *testing.T) {
		functions := []FunctionSpec{
			{
				Name:      "lambda-hello",
				ImageName: "lambda-hello",
				Environment: map[string]string{
					"S3_ENDPOINT": "http://esb-storage:9000",
				},
				Scaling: ScalingSpec{
					MaxCapacity: intPtr(5),
				},
				Timeout:    10,
				MemorySize: 256,
				Events: []EventSpec{
					{
						Type:               "Schedule",
						ScheduleExpression: "rate(5 minutes)",
					},
				},
			},
		}

		content, err := RenderFunctionsYml(functions, "", "latest")
		if err != nil {
			t.Fatalf("RenderFunctionsYml: %v", err)
		}
		assertSnapshot(t, "functions_simple.golden", content)
	})

	t.Run("routing", func(t *testing.T) {
		functions := []FunctionSpec{
			{
				Name: "lambda-hello",
				Events: []EventSpec{
					{
						Path:   "/api/hello",
						Method: "post",
					},
				},
			},
		}

		content, err := RenderRoutingYml(functions)
		if err != nil {
			t.Fatalf("RenderRoutingYml: %v", err)
		}
		assertSnapshot(t, "routing_simple.golden", content)
	})
}

func assertSnapshot(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join("testdata", "renderer", name)
	if os.Getenv("UPDATE_SNAPSHOTS") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir snapshot dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write snapshot: %v", err)
		}
		return
	}

	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("snapshot missing %s (set UPDATE_SNAPSHOTS=1): %v", path, err)
	}
	if content != string(expected) {
		t.Fatalf("snapshot mismatch for %s\n---want\n%s\n---got\n%s", path, expected, content)
	}
}
