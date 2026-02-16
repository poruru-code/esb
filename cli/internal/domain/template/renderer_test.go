// Where: cli/internal/domain/template/renderer_test.go
// What: Tests for template renderers.
// Why: Ensure output formats stay stable during Go migration.
package template

import (
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"
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
		SitecustomizeSource: "runtime/python/extensions/sitecustomize/site-packages/sitecustomize.py",
	}

	content, err := RenderDockerfile(fn, dockerConfig, "", "latest")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expectedBase := "FROM " + meta.ImagePrefix + "-lambda-base:latest"
	if !strings.Contains(content, expectedBase) {
		t.Fatalf("unexpected base image, expected %s, got: %s", expectedBase, content)
	}
	if !strings.Contains(content, "COPY runtime/python/extensions/sitecustomize/site-packages/sitecustomize.py") {
		t.Fatalf("expected sitecustomize copy")
	}
	if !strings.Contains(content, "ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}") {
		t.Fatalf("expected PYTHONPATH env for /opt/python")
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

func TestRenderDockerfileJavaRuntime(t *testing.T) {
	cases := []struct {
		runtime string
		base    string
	}{
		{runtime: "java21", base: "public.ecr.aws/lambda/java:21"},
	}

	for _, tc := range cases {
		t.Run(tc.runtime, func(t *testing.T) {
			fn := FunctionSpec{
				Name:    "lambda-java",
				CodeURI: "functions/java/",
				Handler: "com.example.Handler::handleRequest",
				Runtime: tc.runtime,
			}

			content, err := RenderDockerfile(fn, DockerConfig{}, "", "latest")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if !strings.Contains(content, "FROM "+tc.base) {
				t.Fatalf("expected java base image %s, got: %s", tc.base, content)
			}
			if !strings.Contains(content, `ENV LAMBDA_ORIGINAL_HANDLER="com.example.Handler::handleRequest"`) {
				t.Fatalf("expected original handler env var")
			}
			if !strings.Contains(content, "COPY functions/lambda-java/lambda-java-wrapper.jar /var/task/lib/lambda-java-wrapper.jar") {
				t.Fatalf("expected java wrapper copy")
			}
			if !strings.Contains(content, "COPY functions/lambda-java/lambda-java-agent.jar /var/task/lib/lambda-java-agent.jar") {
				t.Fatalf("expected java agent copy")
			}
			if !strings.Contains(content, "ENV JAVA_AGENT_PRESENT=1") {
				t.Fatalf("expected java agent flag")
			}
			if !strings.Contains(content, "ENV JAVA_TOOL_OPTIONS=\"-javaagent:/var/task/lib/lambda-java-agent.jar") {
				t.Fatalf("expected java tool options with agent")
			}
			if !strings.Contains(content, `CMD [ "com.runtime.lambda.HandlerWrapper::handleRequest" ]`) {
				t.Fatalf("expected wrapper handler command")
			}
			if strings.Contains(content, "sitecustomize.py") {
				t.Fatalf("did not expect sitecustomize for java runtime")
			}
			if strings.Contains(content, "pip install") {
				t.Fatalf("did not expect pip install for java runtime")
			}
		})
	}
}

func TestRenderDockerfileImageWrapperPython(t *testing.T) {
	fn := FunctionSpec{
		Name:        "lambda-image",
		ImageSource: "public.ecr.aws/example/repo:latest",
		Runtime:     "python3.12",
	}
	dockerConfig := DockerConfig{
		SitecustomizeSource: "functions/lambda-image/sitecustomize.py",
	}

	content, err := RenderDockerfile(fn, dockerConfig, "", "latest")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(content, "FROM public.ecr.aws/example/repo:latest") {
		t.Fatalf("expected image source base")
	}
	if !strings.Contains(content, "COPY functions/lambda-image/sitecustomize.py /opt/python/sitecustomize.py") {
		t.Fatalf("expected sitecustomize copy")
	}
	if !strings.Contains(content, "ENV PYTHONPATH=/opt/python${PYTHONPATH:+:${PYTHONPATH}}") {
		t.Fatalf("expected PYTHONPATH env for image wrapper")
	}
	if strings.Contains(content, "COPY functions/") && strings.Contains(content, "${LAMBDA_TASK_ROOT}/") {
		t.Fatalf("did not expect function code copy for image wrapper")
	}
	if strings.Contains(content, "CMD [") {
		t.Fatalf("did not expect CMD override for image wrapper")
	}
}

func TestRenderDockerfileImageWrapperJava(t *testing.T) {
	fn := FunctionSpec{
		Name:        "lambda-image-java",
		ImageSource: "123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/repo:v1",
		Runtime:     "java21",
	}

	content, err := RenderDockerfile(fn, DockerConfig{}, "", "latest")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !strings.Contains(content, "FROM 123456789012.dkr.ecr.ap-northeast-1.amazonaws.com/repo:v1") {
		t.Fatalf("expected image source base")
	}
	if !strings.Contains(content, "COPY functions/lambda-image-java/lambda-java-agent.jar /var/task/lib/lambda-java-agent.jar") {
		t.Fatalf("expected java agent copy")
	}
	if strings.Contains(content, "lambda-java-wrapper.jar") {
		t.Fatalf("did not expect wrapper jar for image wrapper")
	}
	if strings.Contains(content, "LAMBDA_ORIGINAL_HANDLER") {
		t.Fatalf("did not expect original handler override")
	}
	if strings.Contains(content, `CMD [ "com.runtime.lambda.HandlerWrapper::handleRequest" ]`) {
		t.Fatalf("did not expect wrapper handler command for image wrapper")
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
	image, ok := entry["image"].(string)
	if !ok || image == "" {
		t.Fatalf("expected image entry in functions.yml")
	}
	if !strings.Contains(image, "lambda-hello:latest") {
		t.Fatalf("unexpected image value: %s", image)
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

func TestRenderFunctionsYmlForImageSource(t *testing.T) {
	functions := []FunctionSpec{
		{
			Name:        "lambda-image",
			ImageSource: "public.ecr.aws/example/repo:latest",
			ImageName:   "lambda-image",
			ImageRef:    "registry:5010/public.ecr.aws/example/repo:latest",
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
	functionsNode, ok := parsed["functions"].(map[string]any)
	if !ok {
		t.Fatalf("expected functions map")
	}
	entry, ok := functionsNode["lambda-image"].(map[string]any)
	if !ok {
		t.Fatalf("expected lambda-image map")
	}
	if entry["image"] != meta.ImagePrefix+"-lambda-image:latest" {
		t.Fatalf("unexpected image: %v", entry["image"])
	}
	if _, ok := entry["image_source"]; ok {
		t.Fatalf("image_source should not be rendered")
	}
	if _, ok := entry["image_ref"]; ok {
		t.Fatalf("image_ref should not be rendered")
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
