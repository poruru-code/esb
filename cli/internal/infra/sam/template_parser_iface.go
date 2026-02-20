// Where: cli/internal/infra/sam/template_parser_iface.go
// What: Parser interface abstraction for SAM parsing.
// Why: Allow swapping implementations (custom vs goformation).
package sam

import "github.com/poruru-code/esb/cli/internal/domain/template"

type Parser interface {
	Parse(content string, parameters map[string]string) (template.ParseResult, error)
}

type DefaultParser struct{}

func (DefaultParser) Parse(content string, parameters map[string]string) (template.ParseResult, error) {
	return ParseSAMTemplate(content, parameters)
}
