// Where: cli/internal/generator/renderer.go
// What: Render Dockerfile, functions.yml, and routing.yml outputs.
// Why: Provide Go equivalents of the Python generator templates.
package generator

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

const defaultSitecustomizeSource = "cli/internal/generator/assets/site-packages/sitecustomize.py"

type DockerConfig struct {
	SitecustomizeSource string
}

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
	runtime := fn.Runtime
	if runtime == "" {
		runtime = "python3.12"
	}
	pythonVersion := strings.TrimPrefix(runtime, "python")
	if pythonVersion == runtime {
		pythonVersion = "3.12"
	}

	sitecustomize := dockerConfig.SitecustomizeSource
	if sitecustomize == "" {
		sitecustomize = defaultSitecustomizeSource
	}

	baseImage := "esb-lambda-base:" + tag
	if registry != "" {
		baseImage = fmt.Sprintf("%s/esb-lambda-base:%s", registry, tag)
	}

	data := dockerfileTemplateData{
		Name:                fn.Name,
		BaseImage:           baseImage,
		SitecustomizeSource: sitecustomize,
		CodeURI:             fn.CodeURI,
		Handler:             fn.Handler,
		HasRequirements:     fn.HasRequirements,
		Layers:              fn.Layers,
		PythonVersion:       pythonVersion,
	}

	return renderTemplate("dockerfile.tmpl", data)
}

func RenderFunctionsYml(functions []FunctionSpec, registry, tag string) (string, error) {
	if tag == "" {
		tag = "latest"
	}

	data := functionsTemplateData{
		Registry: registry,
		Tag:      tag,
	}
	for _, fn := range functions {
		entry := functionTemplateContext{
			Name:        fn.Name,
			Timeout:     optionalInt(fn.Timeout),
			MemorySize:  optionalInt(fn.MemorySize),
			Environment: fn.Environment,
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
		return value.(*template.Template), nil
	}
	tmpl, err := template.New(name).Funcs(sprig.TxtFuncMap()).ParseFS(templateFS, "templates/"+name)
	if err != nil {
		return nil, err
	}
	templateCache.Store(name, tmpl)
	return tmpl, nil
}

type dockerfileTemplateData struct {
	Name                string
	BaseImage           string
	SitecustomizeSource string
	CodeURI             string
	Handler             string
	HasRequirements     bool
	Layers              []LayerSpec
	PythonVersion       string
}

type functionsTemplateData struct {
	Registry  string
	Tag       string
	Functions []functionTemplateContext
}

type functionTemplateContext struct {
	Name        string
	Timeout     *int
	MemorySize  *int
	Environment map[string]string
	Scaling     map[string]any
}

type routingTemplateData struct {
	Functions []routingFunction
}

type routingFunction struct {
	Name   string
	Events []EventSpec
}

func optionalInt(value int) *int {
	if value == 0 {
		return nil
	}
	result := value
	return &result
}
