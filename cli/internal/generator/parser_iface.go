// Where: cli/internal/generator/parser_iface.go
// What: Parser interface abstraction for SAM parsing.
// Why: Allow swapping implementations (custom vs goformation).
package generator

type Parser interface {
	Parse(content string, parameters map[string]string) (ParseResult, error)
}

type DefaultParser struct{}

func (DefaultParser) Parse(content string, parameters map[string]string) (ParseResult, error) {
	return ParseSAMTemplate(content, parameters)
}
