// Where: cli/internal/helpers/template_parser.go
// What: Template parser adapter for workflows.
// Why: Translate SAM templates into manifest resources via the configured parser.
package helpers

import (
	"github.com/poruru/edge-serverless-box/cli/internal/generator"
	"github.com/poruru/edge-serverless-box/cli/internal/ports"
)

type templateParser struct {
	parser generator.Parser
}

func NewTemplateParser(parser generator.Parser) ports.TemplateParser {
	if parser == nil {
		parser = generator.DefaultParser{}
	}
	return templateParser{parser: parser}
}

func (t templateParser) Parse(content string, params map[string]string) (generator.ParseResult, error) {
	return t.parser.Parse(content, params)
}
