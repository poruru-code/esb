// Where: cli/internal/sam/parser.go
// What: Thin wrappers around the SAM parser for decoding and resolving templates.
// Why: Centralize parser dependencies outside generator/provisioner packages.
package sam

import samparser "github.com/poruru-code/aws-sam-parser-go/parser"

// Context aliases the parser context for intrinsic resolution.
type Context = samparser.Context

// Resolver aliases the parser resolver interface.
type Resolver = samparser.Resolver

// DecodeYAML parses YAML content into a raw map structure.
func DecodeYAML(content string) (map[string]any, error) {
	return samparser.DecodeYAML(content)
}

// ResolveAll resolves intrinsic functions using the provided resolver.
func ResolveAll(ctx *Context, data any, resolver Resolver) (any, error) {
	return samparser.ResolveAll(ctx, data, resolver)
}
