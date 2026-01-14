// Where: cli/internal/generator/parser_decode.go
// What: YAML decoding helpers for SAM parser.
// Why: Normalize tagged YAML nodes into generic Go values.
package generator

import (
	"fmt"
	"strconv"

	"gopkg.in/yaml.v3"
)

func decodeYAML(content string) (map[string]any, error) {
	var node yaml.Node
	if err := yaml.Unmarshal([]byte(content), &node); err != nil {
		return nil, err
	}
	if len(node.Content) == 0 {
		return nil, fmt.Errorf("empty yaml document")
	}
	decoded := decodeNode(node.Content[0])
	data, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected yaml root")
	}
	return data, nil
}

func decodeNode(node *yaml.Node) any {
	if node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return nil
		}
		return decodeNode(node.Content[0])
	case yaml.MappingNode:
		m := map[string]any{}
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			key := asString(decodeNode(keyNode))
			if key == "" {
				continue
			}
			m[key] = decodeNode(valueNode)
		}
		// Handle tags on mappings (e.g. !Sub { Key: Val })
		switch node.Tag {
		case "!Sub":
			return map[string]any{"Fn::Sub": m}
		}
		return m
	case yaml.SequenceNode:
		out := make([]any, 0, len(node.Content))
		for _, item := range node.Content {
			out = append(out, decodeNode(item))
		}
		// Handle tags on sequences (e.g. !Join [ "", [] ])
		switch node.Tag {
		case "!Join":
			return map[string]any{"Fn::Join": out}
		case "!Sub":
			return map[string]any{"Fn::Sub": out}
		case "!GetAtt":
			return map[string]any{"Fn::GetAtt": out}
		case "!If":
			return map[string]any{"Fn::If": out}
		case "!Equals":
			return map[string]any{"Fn::Equals": out}
		case "!And":
			return map[string]any{"Fn::And": out}
		case "!Or":
			return map[string]any{"Fn::Or": out}
		case "!Not":
			return map[string]any{"Fn::Not": out}
		case "!Select":
			return map[string]any{"Fn::Select": out}
		case "!Split":
			return map[string]any{"Fn::Split": out}
		}
		return out
	case yaml.ScalarNode:
		return decodeScalar(node)
	default:
		return nil
	}
}

func decodeScalar(node *yaml.Node) any {
	if node == nil {
		return nil
	}
	switch node.Tag {
	case "!!int":
		if value, err := strconv.Atoi(node.Value); err == nil {
			return value
		}
	case "!!float":
		if value, err := strconv.ParseFloat(node.Value, 64); err == nil {
			return value
		}
	case "!!bool":
		if value, err := strconv.ParseBool(node.Value); err == nil {
			return value
		}
	case "!!null":
		return nil
	case "!Ref":
		return map[string]any{"Ref": node.Value}
	case "!Sub":
		return map[string]any{"Fn::Sub": node.Value}
	case "!GetAtt":
		return map[string]any{"Fn::GetAtt": node.Value}
	case "!ImportValue":
		return map[string]any{"Fn::ImportValue": node.Value}
	case "!Condition":
		return map[string]any{"Condition": node.Value}
	}
	return node.Value
}
