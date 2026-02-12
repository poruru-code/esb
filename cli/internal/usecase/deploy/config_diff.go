// Where: cli/internal/usecase/deploy/config_diff.go
// What: Config snapshot/diff helpers for deploy merge summaries.
// Why: Surface merged config changes after deploy to improve UX.
package deploy

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	domaincfg "github.com/poruru/edge-serverless-box/cli/internal/domain/config"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/ui"
	"gopkg.in/yaml.v3"
)

var errTemplatePathRequired = errors.New("template path is required")

func loadConfigSnapshot(configDir string) (domaincfg.Snapshot, error) {
	snapshot := domaincfg.Snapshot{
		Functions: map[string]any{},
		Routes:    map[string]any{},
		Resources: map[string]map[string]any{},
	}
	if strings.TrimSpace(configDir) == "" {
		return snapshot, nil
	}

	functions, err := loadYamlFile(filepath.Join(configDir, "functions.yml"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
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
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
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
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
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

func diffConfigSnapshots(before, after domaincfg.Snapshot) domaincfg.Diff {
	return domaincfg.DiffSnapshots(before, after)
}

func emitConfigMergeSummary(printer ui.UserInterface, configDir string, diff domaincfg.Diff) {
	if printer == nil {
		return
	}
	rows := []ui.KeyValue{
		{Key: "Staging config", Value: configDir},
		{Key: "Routes", Value: domaincfg.FormatCountsLabel(diff.Routes)},
		{Key: "Functions", Value: domaincfg.FormatCountsLabel(diff.Functions)},
	}
	for _, key := range []string{"dynamodb", "s3", "layers"} {
		counts := diff.Resources[key]
		if counts.Total == 0 && counts.Added == 0 && counts.Updated == 0 && counts.Removed == 0 {
			continue
		}
		rows = append(rows, ui.KeyValue{
			Key:   fmt.Sprintf("Resources.%s", key),
			Value: domaincfg.FormatCountsLabel(counts),
		})
	}
	printer.Block("🧩", "Config merge summary", rows)
}

func emitTemplateDeltaSummary(printer ui.UserInterface, configDir string, diff domaincfg.Diff) {
	if printer == nil {
		return
	}
	rows := []ui.KeyValue{
		{Key: "Template config", Value: configDir},
		{Key: "Routes", Value: domaincfg.FormatTemplateCounts(diff.Routes)},
		{Key: "Functions", Value: domaincfg.FormatTemplateCounts(diff.Functions)},
	}
	for _, key := range []string{"dynamodb", "s3", "layers"} {
		counts := diff.Resources[key]
		if counts.Total == 0 && counts.Added == 0 && counts.Updated == 0 && counts.Removed == 0 {
			continue
		}
		rows = append(rows, ui.KeyValue{
			Key:   fmt.Sprintf("Resources.%s", key),
			Value: domaincfg.FormatTemplateCounts(counts),
		})
	}
	printer.Block("🧾", "Template delta summary", rows)
}

func loadYamlFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode config %s: %w", path, err)
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

func resolveTemplateConfigDir(templatePath, outputDir, env string) (string, error) {
	trimmed := strings.TrimSpace(templatePath)
	if trimmed == "" {
		return "", errTemplatePathRequired
	}
	baseDir := filepath.Dir(trimmed)
	normalized := domaincfg.NormalizeOutputDir(outputDir)
	path := normalized
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}
	path = filepath.Clean(path)
	return filepath.Join(path, env, "config"), nil
}
