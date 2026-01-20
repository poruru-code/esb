// Where: cli/internal/helpers/template_loader.go
// What: Template loader adapter for workflows.
// Why: Provide a simple way for workflows to read the SAM template file.
package helpers

import (
	"os"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
)

type templateLoader struct{}

func NewTemplateLoader() ports.TemplateLoader {
	return templateLoader{}
}

func (templateLoader) Read(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
