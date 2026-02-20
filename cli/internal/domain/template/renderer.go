// Where: cli/internal/domain/template/renderer.go
// What: Render Dockerfile, functions.yml, and routing.yml outputs.
// Why: Provide Go equivalents of the Python generator templates.
package template

import (
	"bytes"
	"embed"
	"fmt"
	"path"
	"strings"
	"sync"
	"text/template"

	runtimeassets "github.com/poruru-code/esb/cli/assets"
	"github.com/poruru-code/esb/cli/internal/domain/runtime"
	"github.com/poruru-code/esb/cli/internal/meta"

	"github.com/Masterminds/sprig/v3"
	"github.com/poruru-code/esb/cli/internal/domain/manifest"
	"gopkg.in/yaml.v3"
)

// DefaultSitecustomizeSource is the default sitecustomize.py path used by the build pipeline.
const DefaultSitecustomizeSource = "runtime-hooks/python/sitecustomize/site-packages/sitecustomize.py"

//go:embed templates/*.tmpl
var templateFS embed.FS

var templateCache sync.Map

func RenderDockerfile(
	fn FunctionSpec,
	dockerConfig DockerConfig,
	registry string,
	tag string,
) (string, error) {
	if tag == "" {
		tag = "latest"
	}
	registry = normalizeRegistry(registry)
	profile, err := runtime.Resolve(fn.Runtime)
	if err != nil {
		return "", err
	}
	isImageWrapper := strings.TrimSpace(fn.ImageSource) != ""

	sitecustomize := dockerConfig.SitecustomizeSource
	if sitecustomize == "" {
		sitecustomize = DefaultSitecustomizeSource
	}

	lambdaBase := meta.ImagePrefix + "-lambda-base"
	baseImage := lambdaBase + ":" + tag
	if isImageWrapper {
		baseImage = strings.TrimSpace(fn.ImageSource)
	} else {
		if profile.Kind == runtime.KindJava {
			if profile.JavaBaseImage == "" {
				return "", fmt.Errorf("java base image is required for runtime %s", profile.Name)
			}
			baseImage = profile.JavaBaseImage
		} else if registry != "" {
			baseImage = fmt.Sprintf("%s%s:%s", registry, lambdaBase, tag)
		}
	}

	handler := fn.Handler
	originalHandler := ""
	javaWrapperSource := ""
	useJavaAgent := false
	javaAgentSource := ""
	if profile.Kind == runtime.KindJava {
		useJavaAgent = true
		javaAgentSource = path.Join("functions", fn.Name, "lambda-java-agent.jar")
		if !isImageWrapper {
			originalHandler = handler
			handler = "com.runtime.lambda.HandlerWrapper::handleRequest"
			javaWrapperSource = path.Join("functions", fn.Name, "lambda-java-wrapper.jar")
		}
	}

	data := dockerfileTemplateData{
		Name:                fn.Name,
		ImageWrapper:        isImageWrapper,
		BaseImage:           baseImage,
		SitecustomizeSource: sitecustomize,
		UsePip:              !isImageWrapper && profile.UsesPip && fn.HasRequirements,
		JavaWrapperSource:   javaWrapperSource,
		UseJavaAgent:        useJavaAgent,
		JavaAgentSource:     javaAgentSource,
		OriginalHandler:     originalHandler,
		CodeURI:             fn.CodeURI,
		AppCodeJarPath:      fn.AppCodeJarPath,
		Handler:             handler,
		Layers:              fn.Layers,
		PythonVersion:       profile.PythonVersion,
	}

	templateName := "python/dockerfile.tmpl"
	if profile.Kind == runtime.KindJava {
		templateName = "java/dockerfile.tmpl"
	}
	return renderTemplate(templateName, data)
}

func RenderFunctionsYml(functions []FunctionSpec, registry, tag string) (string, error) {
	if strings.TrimSpace(tag) == "" {
		tag = "latest"
	}
	registry = normalizeRegistry(registry)
	data := functionsTemplateData{}
	for _, fn := range functions {
		imageName := strings.TrimSpace(fn.ImageName)
		imageRef := strings.TrimSpace(fn.ImageRef)
		if imageName != "" {
			imageRef = fmt.Sprintf("%s%s-%s:%s", registry, meta.ImagePrefix, imageName, tag)
		}
		if imageRef == "" {
			return "", fmt.Errorf("image name is required for function %s", fn.Name)
		}

		hasSchedules := false
		for _, e := range fn.Events {
			if e.Type == "Schedule" {
				hasSchedules = true
				break
			}
		}
		entry := functionTemplateContext{
			Name:         fn.Name,
			Image:        imageRef,
			Timeout:      optionalInt(fn.Timeout),
			MemorySize:   optionalInt(fn.MemorySize),
			Environment:  fn.Environment,
			Events:       fn.Events,
			HasSchedules: hasSchedules,
		}
		if fn.Scaling.MaxCapacity != nil || fn.Scaling.MinCapacity != nil {
			scaling := map[string]any{}
			if fn.Scaling.MaxCapacity != nil {
				scaling["max_capacity"] = *fn.Scaling.MaxCapacity
			}
			if fn.Scaling.MinCapacity != nil {
				scaling["min_capacity"] = *fn.Scaling.MinCapacity
			}
			entry.Scaling = scaling
		}
		data.Functions = append(data.Functions, entry)
	}

	return renderTemplate("functions.yml.tmpl", data)
}

func RenderRoutingYml(functions []FunctionSpec) (string, error) {
	data := routingTemplateData{}
	for _, fn := range functions {
		entry := routingFunction{
			Name:   fn.Name,
			Events: fn.Events,
		}
		data.Functions = append(data.Functions, entry)
	}
	return renderTemplate("routing.yml.tmpl", data)
}

func RenderResourcesYml(spec manifest.ResourcesSpec) (string, error) {
	resources := map[string]any{}
	if len(spec.DynamoDB) > 0 {
		resources["dynamodb"] = spec.DynamoDB
	}
	if len(spec.S3) > 0 {
		resources["s3"] = spec.S3
	}
	if len(spec.Layers) > 0 {
		resources["layers"] = spec.Layers
	}
	payload := map[string]any{
		"resources": resources,
	}
	data, err := marshalYAML(payload, 2)
	if err != nil {
		return "", err
	}
	header := "# Auto-generated by SAM Template Generator\n# DO NOT EDIT MANUALLY - Regenerate with: esb deploy\n\n"
	return header + string(data), nil
}

func marshalYAML(value any, indent int) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(indent)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func renderTemplate(name string, data any) (string, error) {
	tmpl, err := loadTemplate(name)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func loadTemplate(name string) (*template.Template, error) {
	if value, ok := templateCache.Load(name); ok {
		cached, ok := value.(*template.Template)
		if !ok {
			return nil, fmt.Errorf("template cache type mismatch for %s", name)
		}
		return cached, nil
	}
	fs, pathName := resolveTemplateSource(name)
	baseName := path.Base(pathName)
	tmpl, err := template.New(baseName).Funcs(sprig.TxtFuncMap()).ParseFS(fs, pathName)
	if err != nil {
		return nil, err
	}
	templateCache.Store(name, tmpl)
	return tmpl, nil
}

func resolveTemplateSource(name string) (embed.FS, string) {
	switch name {
	case "python/dockerfile.tmpl":
		return runtimeassets.RuntimeTemplatesFS, "runtime-templates/python/templates/dockerfile.tmpl"
	case "java/dockerfile.tmpl":
		return runtimeassets.RuntimeTemplatesFS, "runtime-templates/java/templates/dockerfile.tmpl"
	default:
		return templateFS, "templates/" + name
	}
}

type dockerfileTemplateData struct {
	Name                string
	ImageWrapper        bool
	BaseImage           string
	SitecustomizeSource string
	UsePip              bool
	JavaWrapperSource   string
	UseJavaAgent        bool
	JavaAgentSource     string
	OriginalHandler     string
	CodeURI             string
	AppCodeJarPath      string
	Handler             string
	Layers              []manifest.LayerSpec
	PythonVersion       string
}

type functionsTemplateData struct {
	Functions []functionTemplateContext
}

type functionTemplateContext struct {
	Name         string
	Image        string
	Timeout      *int
	MemorySize   *int
	Environment  map[string]string
	Scaling      map[string]any
	Events       []EventSpec
	HasSchedules bool
}

type routingTemplateData struct {
	Functions []routingFunction
}

type routingFunction struct {
	Name   string
	Events []EventSpec
}

func normalizeRegistry(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	result := value
	return &result
}
