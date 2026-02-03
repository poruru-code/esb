// Where: cli/internal/workflows/config_diff.go
// What: Config snapshot/diff helpers for deploy merge summaries.
// Why: Surface merged config changes after deploy to improve UX.
package workflows

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/poruru/edge-serverless-box/cli/internal/ports"
	"github.com/poruru/edge-serverless-box/meta"
)

type configSnapshot struct {
	Functions map[string]any
	Routes    map[string]any
	Resources map[string]map[string]any
}

type configCounts struct {
	Added   int
	Updated int
	Removed int
	Total   int
}

type configDiff struct {
	Functions configCounts
	Routes    configCounts
	Resources map[string]configCounts
}

func loadConfigSnapshot(configDir string) (configSnapshot, error) {
	snapshot := configSnapshot{
		Functions: map[string]any{},
		Routes:    map[string]any{},
		Resources: map[string]map[string]any{},
	}
	if strings.TrimSpace(configDir) == "" {
		return snapshot, nil
	}

	functions, err := loadYamlFile(filepath.Join(configDir, "functions.yml"))
	if err != nil && !os.IsNotExist(err) {
		return snapshot, err
	}
	if len(functions) > 0 {
		if raw, ok := functions["functions"]; ok {
			for name, value := range asMap(raw) {
				snapshot.Functions[name] = value
			}
		}
	}

	routing, err := loadYamlFile(filepath.Join(configDir, "routing.yml"))
	if err != nil && !os.IsNotExist(err) {
		return snapshot, err
	}
	if len(routing) > 0 {
		if raw, ok := routing["routes"]; ok {
			for _, route := range asSlice(raw) {
				routeMap := asMap(route)
				key := routeKey(routeMap)
				if key == "" {
					continue
				}
				snapshot.Routes[key] = routeMap
			}
		}
	}

	resources, err := loadYamlFile(filepath.Join(configDir, "resources.yml"))
	if err != nil && !os.IsNotExist(err) {
		return snapshot, err
	}
	if len(resources) > 0 {
		if raw, ok := resources["resources"]; ok {
			resourceMap := asMap(raw)
			snapshot.Resources["dynamodb"] = extractNamedResources(resourceMap, "dynamodb", "TableName")
			snapshot.Resources["s3"] = extractNamedResources(resourceMap, "s3", "BucketName")
			snapshot.Resources["layers"] = extractNamedResources(resourceMap, "layers", "Name")
		}
	}

	return snapshot, nil
}

func diffConfigSnapshots(before, after configSnapshot) configDiff {
	diff := configDiff{
		Functions: diffMap(before.Functions, after.Functions),
		Routes:    diffMap(before.Routes, after.Routes),
		Resources: map[string]configCounts{},
	}
	for _, key := range []string{"dynamodb", "s3", "layers"} {
		diff.Resources[key] = diffMap(resourceMap(before, key), resourceMap(after, key))
	}
	return diff
}

func emitConfigMergeSummary(ui ports.UserInterface, configDir string, diff configDiff) {
	if ui == nil {
		return
	}
	rows := []ports.KeyValue{
		{Key: "Staging config", Value: configDir},
		{Key: "Routes", Value: formatCountsLabel(diff.Routes)},
		{Key: "Functions", Value: formatCountsLabel(diff.Functions)},
	}
	for _, key := range []string{"dynamodb", "s3", "layers"} {
		counts := diff.Resources[key]
		if counts.Total == 0 && counts.Added == 0 && counts.Updated == 0 && counts.Removed == 0 {
			continue
		}
		rows = append(rows, ports.KeyValue{
			Key:   fmt.Sprintf("Resources.%s", key),
			Value: formatCountsLabel(counts),
		})
	}
	ui.Block("i", "Config merge summary", rows)
}

func emitTemplateDeltaSummary(ui ports.UserInterface, configDir string, diff configDiff) {
	if ui == nil {
		return
	}
	rows := []ports.KeyValue{
		{Key: "Template config", Value: configDir},
		{Key: "Routes", Value: formatTemplateCounts(diff.Routes)},
		{Key: "Functions", Value: formatTemplateCounts(diff.Functions)},
	}
	for _, key := range []string{"dynamodb", "s3", "layers"} {
		counts := diff.Resources[key]
		if counts.Total == 0 && counts.Added == 0 && counts.Updated == 0 && counts.Removed == 0 {
			continue
		}
		rows = append(rows, ports.KeyValue{
			Key:   fmt.Sprintf("Resources.%s", key),
			Value: formatTemplateCounts(counts),
		})
	}
	ui.Block("i", "Template delta summary", rows)
}

func formatCountsLabel(counts configCounts) string {
	return fmt.Sprintf(
		"new %d / updated %d / removed %d (total %d)",
		counts.Added,
		counts.Updated,
		counts.Removed,
		counts.Total,
	)
}

func formatTemplateCounts(counts configCounts) string {
	unchanged := counts.Total - counts.Added - counts.Updated
	if unchanged < 0 {
		unchanged = 0
	}
	return fmt.Sprintf(
		"new %d / updated %d / unchanged %d (template %d)",
		counts.Added,
		counts.Updated,
		unchanged,
		counts.Total,
	)
}

func diffMap(before, after map[string]any) configCounts {
	counts := configCounts{Total: len(after)}
	if before == nil {
		before = map[string]any{}
	}
	if after == nil {
		after = map[string]any{}
	}
	for key, value := range after {
		prev, ok := before[key]
		if !ok {
			counts.Added++
			continue
		}
		if !reflect.DeepEqual(prev, value) {
			counts.Updated++
		}
	}
	for key := range before {
		if _, ok := after[key]; !ok {
			counts.Removed++
		}
	}
	return counts
}

func loadYamlFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func asMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, val := range typed {
			if name, ok := key.(string); ok {
				out[name] = val
			}
		}
		return out
	default:
		return map[string]any{}
	}
}

func asSlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	default:
		return nil
	}
}

func routeKey(route map[string]any) string {
	path, _ := route["path"].(string)
	method, _ := route["method"].(string)
	if path == "" {
		return ""
	}
	if method == "" {
		method = "GET"
	}
	return fmt.Sprintf("%s:%s", path, method)
}

func extractNamedResources(resources map[string]any, key, nameField string) map[string]any {
	out := map[string]any{}
	raw, ok := resources[key]
	if !ok {
		return out
	}
	for _, item := range asSlice(raw) {
		itemMap := asMap(item)
		name, _ := itemMap[nameField].(string)
		if name == "" {
			continue
		}
		out[name] = itemMap
	}
	return out
}

func resourceMap(snapshot configSnapshot, key string) map[string]any {
	if snapshot.Resources == nil {
		return map[string]any{}
	}
	if resources, ok := snapshot.Resources[key]; ok {
		return resources
	}
	return map[string]any{}
}

func resolveTemplateConfigDir(templatePath, outputDir, env string) (string, error) {
	trimmed := strings.TrimSpace(templatePath)
	if trimmed == "" {
		return "", fmt.Errorf("template path is required")
	}
	baseDir := filepath.Dir(trimmed)
	normalized := normalizeOutputDir(outputDir)
	path := normalized
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	path = filepath.Clean(path)
	return filepath.Join(path, env, "config"), nil
}

func normalizeOutputDir(outputDir string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(outputDir), "/\\")
	if trimmed == "" {
		return meta.OutputDir
	}
	return trimmed
}
