// Where: cli/internal/infra/templategen/generate_test.go
// What: Tests for GenerateFiles staging/output behavior.
// Why: Validate file generation and parser injection.
package templategen

import (
	"archive/zip"
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poruru-code/esb/cli/internal/domain/manifest"
	"github.com/poruru-code/esb/cli/internal/domain/template"
	"github.com/poruru-code/esb/cli/internal/infra/config"
	"github.com/poruru-code/esb/cli/internal/meta"
	"gopkg.in/yaml.v3"
)

type stubParser struct {
	calls  int
	params map[string]string
	result template.ParseResult
}

func (s *stubParser) Parse(_ string, params map[string]string) (template.ParseResult, error) {
	s.calls++
	s.params = params
	return s.result, nil
}

func TestResolveImageFunctionRuntime(t *testing.T) {
	got, err := resolveImageFunctionRuntime("lambda-image", nil)
	if err != nil {
		t.Fatalf("resolve default runtime: %v", err)
	}
	if got != "python3.12" {
		t.Fatalf("expected default runtime python3.12, got %q", got)
	}

	got, err = resolveImageFunctionRuntime("lambda-image", map[string]string{"lambda-image": "java21"})
	if err != nil {
		t.Fatalf("resolve java runtime: %v", err)
	}
	if got != "java21" {
		t.Fatalf("expected java21 runtime, got %q", got)
	}

	if _, err := resolveImageFunctionRuntime("lambda-image", map[string]string{"lambda-image": "nodejs20.x"}); err == nil {
		t.Fatalf("expected unsupported runtime error")
	}
}

func TestApplyImageSourceOverrides(t *testing.T) {
	functions := []template.FunctionSpec{
		{
			Name:        "lambda-image",
			ImageSource: "public.ecr.aws/example/original:latest",
		},
	}

	err := applyImageSourceOverrides(functions, map[string]string{
		"lambda-image": "public.ecr.aws/example/override:v1",
	})
	if err != nil {
		t.Fatalf("apply image source overrides: %v", err)
	}
	if got := functions[0].ImageSource; got != "public.ecr.aws/example/override:v1" {
		t.Fatalf("unexpected image source override: %q", got)
	}
}

func TestApplyImageSourceOverridesRejectsNonImageFunction(t *testing.T) {
	functions := []template.FunctionSpec{
		{
			Name:    "lambda-code",
			Runtime: "python3.12",
			CodeURI: "functions/code/",
		},
	}

	err := applyImageSourceOverrides(functions, map[string]string{
		"lambda-code": "public.ecr.aws/example/override:v1",
	})
	if err == nil {
		t.Fatalf("expected error for non-image function override")
	}
}

func TestGenerateFilesUsesParserOverride(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	funcDir := filepath.Join(root, "functions", "hello")
	mustMkdirAll(t, funcDir)
	writeTestFile(t, filepath.Join(funcDir, "app.py"), "print('hello')")
	writeTestFile(t, filepath.Join(funcDir, "requirements.txt"), "requests\n")
	writeTestFile(t, filepath.Join(root, "sitecustomize.py"), "print('site')")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
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
		App: config.AppConfig{},
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   meta.OutputDir + "/",
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
	if functions[0].ImageName != "lambda-hello" {
		t.Fatalf("unexpected image name: %s", functions[0].ImageName)
	}

	outputDir := filepath.Join(root, meta.OutputDir)
	runtimeBaseDockerfile := filepath.Join(outputDir, runtimeBaseContextDirName, runtimeBasePythonDockerfileRel)
	if _, err := os.Stat(runtimeBaseDockerfile); err != nil {
		t.Fatalf("expected runtime base dockerfile to be staged: %v", err)
	}
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

func TestGenerateFilesWritesWarningsToInjectedOutput(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	parser := &stubParser{
		result: template.ParseResult{
			Warnings: []string{"sample warning"},
		},
	}

	cfg := config.GeneratorConfig{
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   "out/",
		},
	}
	var out bytes.Buffer
	opts := GenerateOptions{
		ProjectRoot: root,
		Parser:      parser,
		Out:         &out,
	}

	if _, err := GenerateFiles(cfg, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(out.String(), "Warning: sample warning") {
		t.Fatalf("expected warning output, got %q", out.String())
	}
}

func TestGenerateFilesVerboseWritesToInjectedOutput(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:        "lambda-image",
					ImageSource: "public.ecr.aws/example/image:latest",
					ImageName:   "lambda-image",
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
	var out bytes.Buffer
	opts := GenerateOptions{
		ProjectRoot: root,
		Parser:      parser,
		Out:         &out,
		Verbose:     true,
	}

	if _, err := GenerateFiles(cfg, opts); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	log := out.String()
	if !strings.Contains(log, "Parsing template...") {
		t.Fatalf("expected parse log, got %q", log)
	}
	if !strings.Contains(log, "Processing function: lambda-image") {
		t.Fatalf("expected function log, got %q", log)
	}
}

func TestGenerateFilesStagesLayersAndZip(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
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
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:    "lambda-one",
					CodeURI: "functions/one/",
					Layers: []manifest.LayerSpec{
						{Name: "common-layer", ContentURI: "layers/common/"},
						{Name: "zip-layer", ContentURI: "layers/zip-layer.zip"},
					},
				},
				{
					Name:    "lambda-two",
					CodeURI: "functions/two/",
					Layers: []manifest.LayerSpec{
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

func TestGenerateFilesStagesJavaJarAndWrapper(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	jarPath := filepath.Join(root, "converter_jsoncrb.jar")
	writeTestFile(t, jarPath, "jar")

	wrapperPath := filepath.Join(root, "runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar")
	mustMkdirAll(t, filepath.Dir(wrapperPath))
	writeTestFile(t, wrapperPath, "runtime-wrapper")
	agentPath := filepath.Join(root, "runtime-hooks", "java", "agent", "lambda-java-agent.jar")
	mustMkdirAll(t, filepath.Dir(agentPath))
	writeTestFile(t, agentPath, "runtime-agent")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:    "lambda-java",
					CodeURI: "converter_jsoncrb.jar",
					Handler: "com.example.Handler::handleRequest",
					Runtime: "java21",
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

	jarDest := filepath.Join(root, "out", "functions", "lambda-java", "src", "lib", "converter_jsoncrb.jar")
	if _, err := os.Stat(jarDest); err != nil {
		t.Fatalf("expected jar to be staged: %v", err)
	}

	wrapperDest := filepath.Join(root, "out", "functions", "lambda-java", "lambda-java-wrapper.jar")
	if _, err := os.Stat(wrapperDest); err != nil {
		t.Fatalf("expected wrapper jar to be staged: %v", err)
	}
	if got := readFile(t, wrapperDest); got != "runtime-wrapper" {
		t.Fatalf("expected runtime wrapper jar content, got %q", got)
	}
	agentDest := filepath.Join(root, "out", "functions", "lambda-java", "lambda-java-agent.jar")
	if _, err := os.Stat(agentDest); err != nil {
		t.Fatalf("expected agent jar to be staged: %v", err)
	}
	if got := readFile(t, agentDest); got != "runtime-agent" {
		t.Fatalf("expected runtime agent jar content, got %q", got)
	}

	dockerfilePath := filepath.Join(root, "out", "functions", "lambda-java", "Dockerfile")
	content := readFile(t, dockerfilePath)
	if !strings.Contains(content, "FROM public.ecr.aws/lambda/java:21") {
		t.Fatalf("expected java base image in dockerfile")
	}
	if !strings.Contains(content, `ENV LAMBDA_ORIGINAL_HANDLER="com.example.Handler::handleRequest"`) {
		t.Fatalf("expected original handler env in dockerfile")
	}
	if !strings.Contains(content, "COPY functions/lambda-java/lambda-java-wrapper.jar /var/task/lib/lambda-java-wrapper.jar") {
		t.Fatalf("expected wrapper jar copy in dockerfile")
	}
	if !strings.Contains(content, "COPY functions/lambda-java/lambda-java-agent.jar /var/task/lib/lambda-java-agent.jar") {
		t.Fatalf("expected agent jar copy in dockerfile")
	}
	if !strings.Contains(content, "ENV JAVA_AGENT_PRESENT=1") {
		t.Fatalf("expected java agent flag in dockerfile")
	}
	if !strings.Contains(content, "ENV JAVA_TOOL_OPTIONS=\"-javaagent:/var/task/lib/lambda-java-agent.jar") {
		t.Fatalf("expected java tool options with agent")
	}
	if !strings.Contains(content, "app_jar=\"lib/converter_jsoncrb.jar\"") {
		t.Fatalf("expected explicit app jar path in dockerfile")
	}
	if !strings.Contains(content, "jar xf \"${app_jar}\"") {
		t.Fatalf("expected app extraction from app jar in dockerfile")
	}
	if !strings.Contains(content, `CMD [ "com.runtime.lambda.HandlerWrapper::handleRequest" ]`) {
		t.Fatalf("expected wrapper handler cmd in dockerfile")
	}
}

func TestGenerateFilesJavaDirectoryCodeURIDoesNotExtractAppJar(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	javaCodeDir := filepath.Join(root, "functions", "java")
	mustMkdirAll(t, javaCodeDir)
	writeTestFile(t, filepath.Join(javaCodeDir, "Handler.java"), "class Handler {}")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:    "lambda-java-dir",
					CodeURI: "functions/java/",
					Handler: "com.example.Handler::handleRequest",
					Runtime: "java21",
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

	dockerfilePath := filepath.Join(root, "out", "functions", "lambda-java-dir", "Dockerfile")
	content := readFile(t, dockerfilePath)
	if strings.Contains(content, "app_jar=") {
		t.Fatalf("did not expect explicit app jar path for directory CodeUri")
	}
	if strings.Contains(content, "jar xf \"${app_jar}\"") {
		t.Fatalf("did not expect app extraction for directory CodeUri")
	}
}

func TestGenerateFilesStagesJavaRuntimeJarsForMultipleFunctions(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	jarAPath := filepath.Join(root, "converter_a.jar")
	writeTestFile(t, jarAPath, "jar-a")
	jarBPath := filepath.Join(root, "converter_b.jar")
	writeTestFile(t, jarBPath, "jar-b")

	wrapperPath := filepath.Join(root, "runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar")
	mustMkdirAll(t, filepath.Dir(wrapperPath))
	writeTestFile(t, wrapperPath, "runtime-wrapper")
	agentPath := filepath.Join(root, "runtime-hooks", "java", "agent", "lambda-java-agent.jar")
	mustMkdirAll(t, filepath.Dir(agentPath))
	writeTestFile(t, agentPath, "runtime-agent")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:    "lambda-java-a",
					CodeURI: "converter_a.jar",
					Handler: "com.example.Handler::handleRequest",
					Runtime: "java21",
				},
				{
					Name:    "lambda-java-b",
					CodeURI: "converter_b.jar",
					Handler: "com.example.Handler::handleRequest",
					Runtime: "java21",
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

	for _, fn := range []string{"lambda-java-a", "lambda-java-b"} {
		wrapperDest := filepath.Join(root, "out", "functions", fn, "lambda-java-wrapper.jar")
		if got := readFile(t, wrapperDest); got != "runtime-wrapper" {
			t.Fatalf("expected staged wrapper jar for %s, got %q", fn, got)
		}
		agentDest := filepath.Join(root, "out", "functions", fn, "lambda-java-agent.jar")
		if got := readFile(t, agentDest); got != "runtime-agent" {
			t.Fatalf("expected staged agent jar for %s, got %q", fn, got)
		}
	}
}

func TestGenerateFilesStagesJavaRuntimeAsIndependentCopies(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	jarPath := filepath.Join(root, "converter.jar")
	writeTestFile(t, jarPath, "jar")

	wrapperPath := filepath.Join(root, "runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar")
	mustMkdirAll(t, filepath.Dir(wrapperPath))
	writeTestFile(t, wrapperPath, "runtime-wrapper")
	agentPath := filepath.Join(root, "runtime-hooks", "java", "agent", "lambda-java-agent.jar")
	mustMkdirAll(t, filepath.Dir(agentPath))
	writeTestFile(t, agentPath, "runtime-agent")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:    "lambda-java",
					CodeURI: "converter.jar",
					Handler: "com.example.Handler::handleRequest",
					Runtime: "java21",
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

	stagedWrapper := filepath.Join(root, "out", "functions", "lambda-java", "lambda-java-wrapper.jar")
	stagedAgent := filepath.Join(root, "out", "functions", "lambda-java", "lambda-java-agent.jar")
	if got := readFile(t, stagedWrapper); got != "runtime-wrapper" {
		t.Fatalf("expected staged wrapper content, got %q", got)
	}
	if got := readFile(t, stagedAgent); got != "runtime-agent" {
		t.Fatalf("expected staged agent content, got %q", got)
	}

	writeTestFile(t, wrapperPath, "mutated-wrapper")
	writeTestFile(t, agentPath, "mutated-agent")

	if got := readFile(t, stagedWrapper); got != "runtime-wrapper" {
		t.Fatalf("staged wrapper should not be affected by source mutation, got %q", got)
	}
	if got := readFile(t, stagedAgent); got != "runtime-agent" {
		t.Fatalf("staged agent should not be affected by source mutation, got %q", got)
	}
}

func TestGenerateFilesFailsWhenCodeURIDoesNotExist(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:    "lambda-missing",
					CodeURI: "functions/missing/",
					Handler: "handler.main",
					Runtime: "python3.12",
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
	_, err := GenerateFiles(cfg, GenerateOptions{ProjectRoot: root, Parser: parser})
	if err == nil {
		t.Fatalf("expected error for missing code uri")
	}
	if !strings.Contains(err.Error(), "code uri not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateFilesFailsWhenJavaRuntimeJarsMissing(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	if err := os.Remove(filepath.Join(root, "runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar")); err != nil {
		t.Fatalf("remove runtime java wrapper: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "runtime-hooks", "java", "agent", "lambda-java-agent.jar")); err != nil {
		t.Fatalf("remove runtime java agent: %v", err)
	}
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	jarPath := filepath.Join(root, "converter.jar")
	writeTestFile(t, jarPath, "jar")
	mustMkdirAll(t, filepath.Join(root, "runtime-hooks", "java", "wrapper"))
	mustMkdirAll(t, filepath.Join(root, "runtime-hooks", "java", "agent"))

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:    "lambda-java",
					CodeURI: "converter.jar",
					Handler: "com.example.Handler::handleRequest",
					Runtime: "java21",
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
	_, err := GenerateFiles(cfg, GenerateOptions{ProjectRoot: root, Parser: parser})
	if err == nil {
		t.Fatalf("expected error for missing java runtime jars")
	}
	if !strings.Contains(err.Error(), "java wrapper jar not found") &&
		!strings.Contains(err.Error(), "java agent jar not found") &&
		!strings.Contains(err.Error(), "java runtime agent source not found") &&
		!strings.Contains(err.Error(), "java runtime wrapper source not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateFilesDoesNotStageRuntimeBaseJavaJars(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	funcDir := filepath.Join(root, "functions", "hello")
	mustMkdirAll(t, funcDir)
	writeTestFile(t, filepath.Join(funcDir, "app.py"), "print('hello')")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
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
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   "out/",
		},
	}
	if _, err := GenerateFiles(cfg, GenerateOptions{ProjectRoot: root, Parser: parser}); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	runtimeBaseJavaAgent := filepath.Join(root, "out", runtimeBaseContextDirName, "runtime-hooks", "java", "agent", "lambda-java-agent.jar")
	if _, err := os.Stat(runtimeBaseJavaAgent); !os.IsNotExist(err) {
		if err != nil {
			t.Fatalf("stat runtime base java agent: %v", err)
		}
		t.Fatalf("did not expect runtime base java agent to be staged")
	}
	runtimeBaseJavaWrapper := filepath.Join(root, "out", runtimeBaseContextDirName, "runtime-hooks", "java", "wrapper", "lambda-java-wrapper.jar")
	if _, err := os.Stat(runtimeBaseJavaWrapper); !os.IsNotExist(err) {
		if err != nil {
			t.Fatalf("stat runtime base java wrapper: %v", err)
		}
		t.Fatalf("did not expect runtime base java wrapper to be staged")
	}
}

func TestGenerateFilesImageFunctionOmitsImportManifest(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:        "lambda-image",
					ImageSource: "public.ecr.aws/example/repo:latest",
					Timeout:     30,
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

	functions, err := GenerateFiles(cfg, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(functions))
	}
	if functions[0].Runtime != "python3.12" {
		t.Fatalf("expected default image runtime python3.12, got %q", functions[0].Runtime)
	}

	functionDir := filepath.Join(root, "out", "functions", "lambda-image")
	if _, err := os.Stat(functionDir); err != nil {
		t.Fatalf("expected staged function directory for image function: %v", err)
	}
	dockerfilePath := filepath.Join(functionDir, "Dockerfile")
	if _, err := os.Stat(dockerfilePath); err != nil {
		t.Fatalf("expected dockerfile for image function: %v", err)
	}
	dockerfile := readFile(t, dockerfilePath)
	if !strings.Contains(dockerfile, "FROM public.ecr.aws/example/repo:latest") {
		t.Fatalf("expected dockerfile to use image source as base, got: %s", dockerfile)
	}
	if strings.Contains(dockerfile, "CMD [") {
		t.Fatalf("did not expect CMD override for image wrapper dockerfile")
	}

	manifestPath := filepath.Join(root, "out", "config", "image-import.json")
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Fatalf("did not expect image-import.json, got err=%v", err)
	}

	content := readFile(t, filepath.Join(root, "out", "config", "functions.yml"))
	if !strings.Contains(content, "image: \""+meta.ImagePrefix+"-lambda-image:latest\"") {
		t.Fatalf("expected image entry in functions.yml, got: %s", content)
	}
}

func TestGenerateFilesImageFunctionUsesImageSourceOverride(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:        "lambda-image",
					ImageSource: "public.ecr.aws/example/repo:latest",
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
	opts := GenerateOptions{
		ProjectRoot: root,
		Parser:      parser,
		ImageSources: map[string]string{
			"lambda-image": "public.ecr.aws/example/override:v1",
		},
	}

	functions, err := GenerateFiles(cfg, opts)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(functions))
	}

	dockerfilePath := filepath.Join(root, "out", "functions", "lambda-image", "Dockerfile")
	dockerfile := readFile(t, dockerfilePath)
	if !strings.Contains(dockerfile, "FROM public.ecr.aws/example/override:v1") {
		t.Fatalf("expected dockerfile to use image override, got: %s", dockerfile)
	}
}

func TestGenerateFilesLayerNesting(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
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
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:    "lambda-nesting-test",
					Runtime: "python3.12",
					CodeURI: "functions/my-func/",
					Layers: []manifest.LayerSpec{
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

func TestGenerateFilesIntegrationOutputs(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, `
AWSTemplateFormatVersion: '2010-09-09'
Transform: AWS::Serverless-2016-10-31
Resources:
  HelloFunction:
    Type: AWS::Serverless::Function
    Properties:
      FunctionName: lambda-hello
      CodeUri: functions/hello/
      Handler: app.handler
      Runtime: python3.12
      Timeout: 10
      MemorySize: 256
      Environment:
        Variables:
          S3_ENDPOINT: http://esb-storage:9000
      Events:
        HelloApi:
          Type: Api
          Properties:
            Path: /api/hello
            Method: post
        Nightly:
          Type: Schedule
          Properties:
            Schedule: rate(5 minutes)
`)

	funcDir := filepath.Join(root, "functions", "hello")
	mustMkdirAll(t, funcDir)
	writeTestFile(t, filepath.Join(funcDir, "app.py"), "print('hello')")

	cfg := config.GeneratorConfig{
		Paths: config.PathsConfig{
			SamTemplate: "template.yaml",
			OutputDir:   "out/",
		},
	}
	functions, err := GenerateFiles(cfg, GenerateOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(functions) != 1 {
		t.Fatalf("expected 1 function, got %d", len(functions))
	}

	expectedFunctions, err := template.RenderFunctionsYml(functions, "", "latest")
	if err != nil {
		t.Fatalf("RenderFunctionsYml: %v", err)
	}
	expectedRouting, err := template.RenderRoutingYml(functions)
	if err != nil {
		t.Fatalf("RenderRoutingYml: %v", err)
	}

	functionsYml := filepath.Join(root, "out", "config", "functions.yml")
	routingYml := filepath.Join(root, "out", "config", "routing.yml")
	if got := readFile(t, functionsYml); got != expectedFunctions {
		t.Fatalf("functions.yml mismatch")
	}
	if got := readFile(t, routingYml); got != expectedRouting {
		t.Fatalf("routing.yml mismatch")
	}
}

func TestGenerateFilesRendersRoutingEvents(t *testing.T) {
	root := t.TempDir()
	writeRuntimeBaseFixture(t, root)
	templatePath := filepath.Join(root, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	funcDir := filepath.Join(root, "functions", "events")
	mustMkdirAll(t, funcDir)
	writeTestFile(t, filepath.Join(funcDir, "app.py"), "print('events')")

	parser := &stubParser{
		result: template.ParseResult{
			Functions: []template.FunctionSpec{
				{
					Name:    "lambda-events",
					Runtime: "python3.12",
					CodeURI: "functions/events/",
					Events: []template.EventSpec{
						{
							Path:   "/api/events",
							Method: "POST",
						},
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
	if _, err := GenerateFiles(cfg, GenerateOptions{ProjectRoot: root, Parser: parser}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	routingYml := filepath.Join(root, "out", "config", "routing.yml")
	content := readFile(t, routingYml)

	type route struct {
		Path     string `yaml:"path"`
		Method   string `yaml:"method"`
		Function string `yaml:"function"`
	}
	var parsed struct {
		Routes []route `yaml:"routes"`
	}
	if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}
	if len(parsed.Routes) != 1 {
		t.Fatalf("expected one route, got %d", len(parsed.Routes))
	}
	if parsed.Routes[0].Path != "/api/events" {
		t.Fatalf("unexpected path: %s", parsed.Routes[0].Path)
	}
	if parsed.Routes[0].Function != "lambda-events" {
		t.Fatalf("unexpected function: %s", parsed.Routes[0].Function)
	}
	if parsed.Routes[0].Method != http.MethodPost {
		t.Fatalf("unexpected method: %s", parsed.Routes[0].Method)
	}
}

func TestResolveTemplatePathExpandsHome(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	templatePath := filepath.Join(tmpHome, "template.yaml")
	writeTestFile(t, templatePath, "Resources: {}")

	got, err := resolveTemplatePath("~/template.yaml", "/tmp")
	if err != nil {
		t.Fatalf("resolve template path: %v", err)
	}
	if got != templatePath {
		t.Fatalf("expected %s, got %s", templatePath, got)
	}
}

func writeRuntimeBaseFixture(t *testing.T, root string) {
	t.Helper()
	pythonDir := filepath.Join(root, "runtime-hooks", "python")
	javaDir := filepath.Join(root, "runtime-hooks", "java")
	templatesDir := filepath.Join(root, "cli", "assets", "runtime-templates")
	writeTestFile(
		t,
		filepath.Join(pythonDir, "docker", "Dockerfile"),
		"FROM public.ecr.aws/lambda/python:3.12\nCOPY runtime-hooks/python/sitecustomize/site-packages/sitecustomize.py /opt/python/sitecustomize.py\nCOPY runtime-hooks/python/trace-bridge/layer/ /opt/python/\n",
	)
	writeTestFile(
		t,
		filepath.Join(pythonDir, "sitecustomize", "site-packages", "sitecustomize.py"),
		"# test sitecustomize\n",
	)
	writeTestFile(
		t,
		filepath.Join(pythonDir, "trace-bridge", "layer", "trace_bridge.py"),
		"# test trace bridge\n",
	)
	writeTestFile(
		t,
		filepath.Join(javaDir, "agent", "lambda-java-agent.jar"),
		"test java agent\n",
	)
	writeTestFile(
		t,
		filepath.Join(javaDir, "wrapper", "lambda-java-wrapper.jar"),
		"test java wrapper\n",
	)
	writeTestFile(
		t,
		filepath.Join(templatesDir, "python", "templates", "dockerfile.tmpl"),
		"FROM public.ecr.aws/lambda/python:3.12\n",
	)
	writeTestFile(
		t,
		filepath.Join(templatesDir, "java", "templates", "dockerfile.tmpl"),
		"FROM public.ecr.aws/lambda/java:21\n",
	)
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	mustMkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
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
