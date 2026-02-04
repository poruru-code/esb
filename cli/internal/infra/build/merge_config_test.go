// Where: cli/internal/infra/build/merge_config_test.go
// What: Tests for configuration merge logic.
// Why: Ensure last-write-wins merge strategy works correctly.
package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
	"gopkg.in/yaml.v3"
)

func TestMergeFunctionsYml(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src", "config")
	destDir := filepath.Join(tmpDir, "dest")

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create existing config
	existing := map[string]any{
		"functions": map[string]any{
			"funcA": map[string]any{"handler": "a.handler", "runtime": "python3.9"},
			"funcB": map[string]any{"handler": "b.handler", "runtime": "python3.9"},
		},
		"defaults": map[string]any{
			"timeout": 30,
			"memory":  128,
			"environment": map[string]any{
				"LOG_LEVEL": "INFO",
			},
			"scaling": map[string]any{
				"min_capacity": 0,
			},
		},
	}
	writeYaml(t, filepath.Join(destDir, "functions.yml"), existing)

	// Create new config (funcA updated, funcC added)
	src := map[string]any{
		"functions": map[string]any{
			"funcA": map[string]any{"handler": "a_new.handler", "runtime": "python3.11"},
			"funcC": map[string]any{"handler": "c.handler", "runtime": "python3.11"},
		},
		"defaults": map[string]any{
			"timeout": 60,
			"layers":  []string{"layer1"},
			"environment": map[string]any{
				"LOG_LEVEL":             "DEBUG",
				"AWS_ACCESS_KEY_ID":     "esb",
				"AWS_SECRET_ACCESS_KEY": "secret",
			},
			"scaling": map[string]any{
				"min_capacity": 1,
				"max_capacity": 3,
			},
		},
	}
	writeYaml(t, filepath.Join(srcDir, "functions.yml"), src)

	// Merge
	if err := mergeFunctionsYml(srcDir, destDir); err != nil {
		t.Fatal(err)
	}

	// Verify result
	result := readYaml(t, filepath.Join(destDir, "functions.yml"))
	functions := value.AsMap(result["functions"])

	// funcA should be updated
	funcA := value.AsMap(functions["funcA"])
	if funcA["handler"] != "a_new.handler" {
		t.Errorf("funcA handler not updated: got %v", funcA["handler"])
	}

	// funcB should be preserved
	funcB := value.AsMap(functions["funcB"])
	if funcB["handler"] != "b.handler" {
		t.Errorf("funcB should be preserved: got %v", funcB["handler"])
	}

	// funcC should be added
	funcC := value.AsMap(functions["funcC"])
	if funcC["handler"] != "c.handler" {
		t.Errorf("funcC should be added: got %v", funcC)
	}

	// defaults: existing keys preserved, new keys added
	defaults := value.AsMap(result["defaults"])
	if defaults["timeout"] != 30 { // existing preserved
		t.Errorf("existing timeout should be preserved: got %v", defaults["timeout"])
	}
	if defaults["memory"] != 128 { // existing preserved
		t.Errorf("existing memory should be preserved: got %v", defaults["memory"])
	}
	if defaults["layers"] == nil { // new key added
		t.Errorf("new layers key should be added")
	}
	// defaults.environment: existing preserved, missing keys added
	env := value.AsMap(defaults["environment"])
	if env["LOG_LEVEL"] != "INFO" {
		t.Errorf("existing LOG_LEVEL should be preserved: got %v", env["LOG_LEVEL"])
	}
	if env["AWS_ACCESS_KEY_ID"] != "esb" {
		t.Errorf("AWS_ACCESS_KEY_ID should be added: got %v", env["AWS_ACCESS_KEY_ID"])
	}
	if env["AWS_SECRET_ACCESS_KEY"] != "secret" {
		t.Errorf("AWS_SECRET_ACCESS_KEY should be added: got %v", env["AWS_SECRET_ACCESS_KEY"])
	}
	// defaults.scaling: existing preserved, missing keys added
	scaling := value.AsMap(defaults["scaling"])
	if scaling["min_capacity"] != 0 {
		t.Errorf("existing min_capacity should be preserved: got %v", scaling["min_capacity"])
	}
	if scaling["max_capacity"] != 3 {
		t.Errorf("max_capacity should be added: got %v", scaling["max_capacity"])
	}
}

func TestMergeRoutingYml(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src", "config")
	destDir := filepath.Join(tmpDir, "dest")

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create existing config
	existing := map[string]any{
		"routes": []any{
			map[string]any{"path": "/hello", "method": "GET", "function": "funcA"},
			map[string]any{"path": "/world", "method": "POST", "function": "funcB"},
		},
	}
	writeYaml(t, filepath.Join(destDir, "routing.yml"), existing)

	// Create new config (update /hello GET, add /new GET)
	src := map[string]any{
		"routes": []any{
			map[string]any{"path": "/hello", "method": "GET", "function": "funcC"},
			map[string]any{"path": "/new", "method": "GET", "function": "funcD"},
		},
	}
	writeYaml(t, filepath.Join(srcDir, "routing.yml"), src)

	// Merge
	if err := mergeRoutingYml(srcDir, destDir); err != nil {
		t.Fatal(err)
	}

	// Verify result
	result := readYaml(t, filepath.Join(destDir, "routing.yml"))
	routes := value.AsSlice(result["routes"])

	if len(routes) != 3 {
		t.Errorf("expected 3 routes, got %d", len(routes))
	}

	// Find routes by path+method
	routeMap := make(map[string]map[string]any)
	for _, r := range routes {
		rm := value.AsMap(r)
		key := routeKey(rm)
		routeMap[key] = rm
	}

	// /hello GET should be updated to funcC
	if routeMap["/hello:GET"]["function"] != "funcC" {
		t.Errorf("/hello GET should be updated to funcC: got %v", routeMap["/hello:GET"]["function"])
	}

	// /world POST should be preserved
	if routeMap["/world:POST"]["function"] != "funcB" {
		t.Errorf("/world POST should be preserved: got %v", routeMap["/world:POST"]["function"])
	}

	// /new GET should be added
	if routeMap["/new:GET"]["function"] != "funcD" {
		t.Errorf("/new GET should be added: got %v", routeMap["/new:GET"])
	}
}

func TestMergeResourcesYml(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "src", "config")
	destDir := filepath.Join(tmpDir, "dest")

	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create existing config
	existing := map[string]any{
		"resources": map[string]any{
			"dynamodb": []any{
				map[string]any{"TableName": "table1", "KeySchema": "old"},
			},
			"s3": []any{
				map[string]any{"BucketName": "bucket1"},
			},
		},
	}
	writeYaml(t, filepath.Join(destDir, "resources.yml"), existing)

	// Create new config (update table1, add table2)
	src := map[string]any{
		"resources": map[string]any{
			"dynamodb": []any{
				map[string]any{"TableName": "table1", "KeySchema": "new"},
				map[string]any{"TableName": "table2", "KeySchema": "new"},
			},
			"layers": []any{
				map[string]any{"Name": "layer1", "Path": "/opt/layer1"},
			},
		},
	}
	writeYaml(t, filepath.Join(srcDir, "resources.yml"), src)

	// Merge
	if err := mergeResourcesYml(srcDir, destDir); err != nil {
		t.Fatal(err)
	}

	// Verify result
	result := readYaml(t, filepath.Join(destDir, "resources.yml"))
	resources := value.AsMap(result["resources"])

	// DynamoDB tables
	dynamo := value.AsSlice(resources["dynamodb"])
	if len(dynamo) != 2 {
		t.Errorf("expected 2 dynamodb tables, got %d", len(dynamo))
	}

	// table1 should be updated
	table1 := findByKey(dynamo, "TableName", "table1")
	if table1["KeySchema"] != "new" {
		t.Errorf("table1 should be updated: got %v", table1["KeySchema"])
	}

	// S3 buckets should be preserved
	s3 := value.AsSlice(resources["s3"])
	if len(s3) != 1 {
		t.Errorf("expected 1 s3 bucket, got %d", len(s3))
	}

	// Layers should be added
	layers := value.AsSlice(resources["layers"])
	if len(layers) != 1 {
		t.Errorf("expected 1 layer, got %d", len(layers))
	}
}

func writeYaml(t *testing.T, path string, data map[string]any) {
	t.Helper()
	content, err := yaml.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readYaml(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func findByKey(items []any, keyField, keyValue string) map[string]any {
	for _, item := range items {
		m := value.AsMap(item)
		if m[keyField] == keyValue {
			return m
		}
	}
	return nil
}
