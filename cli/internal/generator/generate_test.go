// Where: cli/internal/generator/generate_test.go
// What: Tests for GenerateFiles staging/output behavior.
// Why: Validate file generation and parser injection.
package generator

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/config"
)

type stubParser struct {
	calls  int
	params map[string]string
	result ParseResult
}

func (s *stubParser) Parse(_ string, params map[string]string) (ParseResult, error) {
	s.calls++
	s.params = params
	return s.result, nil
}

func TestGenerateFilesUsesParserOverride(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	funcDir := filepath.Join(root, "functions", "hello")
	mustMkdirAll(t, funcDir)
	writeTestFile(t, filepath.Join(funcDir, "app.py"), "print('hello')")
	writeTestFile(t, filepath.Join(funcDir, "requirements.txt"), "requests\n")
	writeTestFile(t, filepath.Join(root, "sitecustomize.py"), "print('site')")

	parser := &stubParser{
		result: ParseResult{
			Functions: []FunctionSpec{
				{
					Name:    "lambda-hello",
					CodeURI: "functions/hello/",
					Handler: "app.handler",
					Runtime: "python3.12",
				},
			},
		},
	}

	cfg := config.GeneratorConfig{
		App: config.AppConfig{Tag: "v1"},
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   ".esb/",
		},
		Parameters: map[string]any{
			"Prefix": "dev",
		},
	}
	opts := GenerateOptions{
		ProjectRoot:         root,
		Parser:              parser,
		Parameters:          map[string]string{"Stage": "prod"},
		SitecustomizeSource: "sitecustomize.py",
	}

	functions, err := GenerateFiles(cfg, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if parser.calls != 1 {
		t.Fatalf("expected parser to be called once, got %d", parser.calls)
	}
	if parser.params["Prefix"] != "dev" || parser.params["Stage"] != "prod" {
		t.Fatalf("unexpected params: %#v", parser.params)
	}
	if len(functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(functions))
	}

	outputDir := filepath.Join(root, ".esb")
	dockerfilePath := filepath.Join(outputDir, "functions", "lambda-hello", "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		t.Fatalf("expected dockerfile to exist: %v", err)
	}
	content := readFile(t, dockerfilePath)
	if !strings.Contains(content, "pip install -r") {
		t.Fatalf("expected requirements install")
	}

	sitecustomizePath := filepath.Join(outputDir, "functions", "lambda-hello", "sitecustomize.py")
	if _, err := os.Stat(sitecustomizePath); err != nil {
		t.Fatalf("expected sitecustomize to be staged: %v", err)
	}

	functionsYml := filepath.Join(outputDir, "config", "functions.yml")
	routingYml := filepath.Join(outputDir, "config", "routing.yml")
	if _, err := os.Stat(functionsYml); err != nil {
		t.Fatalf("expected functions.yml to exist: %v", err)
	}
	if _, err := os.Stat(routingYml); err != nil {
		t.Fatalf("expected routing.yml to exist: %v", err)
	}
}

func TestGenerateFilesStagesLayersAndZip(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	for _, name := range []string{"one", "two"} {
		funcDir := filepath.Join(root, "functions", name)
		mustMkdirAll(t, funcDir)
		writeTestFile(t, filepath.Join(funcDir, "app.py"), "print('hi')")
	}

	commonLayer := filepath.Join(root, "layers", "common", "python", "common")
	mustMkdirAll(t, commonLayer)
	writeTestFile(t, filepath.Join(commonLayer, "__init__.py"), "# layer")

	zipPath := filepath.Join(root, "layers", "zip-layer.zip")
	mustMkdirAll(t, filepath.Dir(zipPath))
	writeZip(t, zipPath, map[string]string{
		"python/zip_layer/__init__.py": "# zip layer",
	})

	parser := &stubParser{
		result: ParseResult{
			Functions: []FunctionSpec{
				{
					Name:    "lambda-one",
					CodeURI: "functions/one/",
					Layers: []LayerSpec{
						{Name: "common-layer", ContentURI: "layers/common/"},
						{Name: "zip-layer", ContentURI: "layers/zip-layer.zip"},
					},
				},
				{
					Name:    "lambda-two",
					CodeURI: "functions/two/",
					Layers: []LayerSpec{
						{Name: "common-layer", ContentURI: "layers/common/"},
						{Name: "zip-layer", ContentURI: "layers/zip-layer.zip"},
					},
				},
			},
		},
	}

	cfg := config.GeneratorConfig{
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   "out/",
		},
	}
	opts := GenerateOptions{ProjectRoot: root, Parser: parser}

	if _, err := GenerateFiles(cfg, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	cacheDir := filepath.Join(root, "out", ".layers_cache")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("expected layer cache directory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected layer cache entries")
	}

	foundZip := false
	err = filepath.WalkDir(cacheDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, filepath.Join("python", "zip_layer", "__init__.py")) {
			foundZip = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk cache: %v", err)
	}
	if !foundZip {
		t.Fatalf("expected zip layer to be unpacked")
	}

	for _, fn := range []string{"lambda-one", "lambda-two"} {
		dockerfilePath := filepath.Join(root, "out", "functions", fn, "Dockerfile")
		content := readFile(t, dockerfilePath)
		if !strings.Contains(content, "COPY functions/"+fn+"/layers/") {
			t.Fatalf("expected dockerfile to include layers for %s", fn)
		}
		layerRoot := filepath.Join(root, "out", "functions", fn, "layers")
		if _, err := os.Stat(layerRoot); err != nil {
			t.Fatalf("expected per-function layers dir for %s: %v", fn, err)
		}
	}

	commonLayerPath := filepath.Join(root, "out", "functions", "lambda-one", "layers", "common-layer", "python", "common", "__init__.py")
	if _, err := os.Stat(commonLayerPath); err != nil {
		t.Fatalf("expected common layer to be staged: %v", err)
	}
	zipLayerPath := filepath.Join(root, "out", "functions", "lambda-one", "layers", "zip-layer", "python", "zip_layer", "__init__.py")
	if _, err := os.Stat(zipLayerPath); err != nil {
		t.Fatalf("expected zip layer to be staged: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "out", "layers")); err == nil {
		t.Fatalf("did not expect shared layers dir")
	}
}

func TestGenerateFilesLayerNesting(t *testing.T) {
	root := t.TempDir()
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	funcDir := filepath.Join(root, "functions", "my-func")
	mustMkdirAll(t, funcDir)
	writeTestFile(t, filepath.Join(funcDir, "app.py"), "print('caret')")

	flatDir := filepath.Join(root, "layers", "flat_dir")
	mustMkdirAll(t, flatDir)
	writeTestFile(t, filepath.Join(flatDir, "lib_flat.py"), "# flat")

	nestedDir := filepath.Join(root, "layers", "nested_dir", "python")
	mustMkdirAll(t, nestedDir)
	writeTestFile(t, filepath.Join(nestedDir, "lib_nested.py"), "# nested")

	flatZip := filepath.Join(root, "layers", "flat.zip")
	writeZip(t, flatZip, map[string]string{
		"lib_zip_flat.py": "print('zip flat')",
	})

	nestedZip := filepath.Join(root, "layers", "nested.zip")
	writeZip(t, nestedZip, map[string]string{
		"python/lib_zip_nested.py": "print('zip nested')",
	})

	parser := &stubParser{
		result: ParseResult{
			Functions: []FunctionSpec{
				{
					Name:    "lambda-nesting-test",
					Runtime: "python3.12",
					CodeURI: "functions/my-func/",
					Layers: []LayerSpec{
						{Name: "layer-flat-dir", ContentURI: "layers/flat_dir/"},
						{Name: "layer-nested-dir", ContentURI: "layers/nested_dir/"},
						{Name: "layer-flat-zip", ContentURI: "layers/flat.zip"},
						{Name: "layer-nested-zip", ContentURI: "layers/nested.zip"},
					},
				},
			},
		},
	}

	cfg := config.GeneratorConfig{
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   "out/",
		},
	}
	opts := GenerateOptions{ProjectRoot: root, Parser: parser}

	if _, err := GenerateFiles(cfg, opts); err != nil {
		t.Fatalf("generate: %v", err)
	}

	staged := filepath.Join(root, "out", "functions", "lambda-nesting-test", "layers")

	check := func(path string) {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected path %s to exist: %v", path, err)
		}
	}

	check(filepath.Join(staged, "layer-flat-dir", "python", "lib_flat.py"))
	check(filepath.Join(staged, "layer-nested-dir", "python", "lib_nested.py"))
	if _, err := os.Stat(filepath.Join(staged, "layer-nested-dir", "python", "python")); err == nil {
		t.Fatalf("nested dir should not double nest")
	}
	check(filepath.Join(staged, "layer-flat-zip", "python", "lib_zip_flat.py"))
	check(filepath.Join(staged, "layer-nested-zip", "python", "lib_zip_nested.py"))
	if _, err := os.Stat(filepath.Join(staged, "layer-nested-zip", "python", "python")); err == nil {
		t.Fatalf("nested zip should not double nest")
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(payload)
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	zipFile, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	for name, content := range files {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("zip entry: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
}
