// Where: cli/internal/infra/build/merge_config_yaml.go
// What: YAML merge logic for functions, routing, and resources.
// Why: Keep per-config merge behavior isolated from entry flow.
package build

import (
	"fmt"
	"path/filepath"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
)

// mergeFunctionsYml merges functions.yml with last-write-wins for function names.
// defaults are preserved from existing config, with missing keys filled in.
func mergeFunctionsYml(srcDir, destDir string) error {
	srcPath := filepath.Join(srcDir, "functions.yml")
	destPath := filepath.Join(destDir, "functions.yml")

	srcData, err := loadYamlFile(srcPath)
	if err != nil {
		return err
	}

	existingData, _ := loadYamlFile(destPath)

	srcFunctions := value.AsMap(srcData["functions"])
	existingFunctions := value.AsMap(existingData["functions"])
	if existingFunctions == nil {
		existingFunctions = make(map[string]any)
	}
	for name, fn := range srcFunctions {
		existingFunctions[name] = fn
	}

	srcDefaults := value.AsMap(srcData["defaults"])
	existingDefaults := value.AsMap(existingData["defaults"])
	if existingDefaults == nil {
		existingDefaults = make(map[string]any)
	}
	mergeDefaultsSection(existingDefaults, srcDefaults, "environment")
	mergeDefaultsSection(existingDefaults, srcDefaults, "scaling")
	for key, value := range srcDefaults {
		if key == "environment" || key == "scaling" {
			continue
		}
		if _, ok := existingDefaults[key]; !ok {
			existingDefaults[key] = value
		}
	}

	merged := map[string]any{
		"functions": existingFunctions,
	}
	if len(existingDefaults) > 0 {
		merged["defaults"] = existingDefaults
	}

	return atomicWriteYaml(destPath, merged)
}

func mergeDefaultsSection(existingDefaults, srcDefaults map[string]any, key string) {
	if srcDefaults == nil {
		return
	}
	srcSection := value.AsMap(srcDefaults[key])
	if srcSection == nil {
		return
	}
	existingSection := value.AsMap(existingDefaults[key])
	if existingSection == nil {
		existingSection = make(map[string]any)
	}
	for itemKey, itemValue := range srcSection {
		if _, ok := existingSection[itemKey]; !ok {
			existingSection[itemKey] = itemValue
		}
	}
	if len(existingSection) > 0 {
		existingDefaults[key] = existingSection
	}
}

// mergeRoutingYml merges routing.yml with last-write-wins for (path, method) keys.
func mergeRoutingYml(srcDir, destDir string) error {
	srcPath := filepath.Join(srcDir, "routing.yml")
	destPath := filepath.Join(destDir, "routing.yml")

	srcData, err := loadYamlFile(srcPath)
	if err != nil {
		return err
	}

	existingData, _ := loadYamlFile(destPath)

	existingRoutes := value.AsSlice(existingData["routes"])
	routeIndex := make(map[string]int)
	for i, route := range existingRoutes {
		key := routeKey(value.AsMap(route))
		if key != "" {
			routeIndex[key] = i
		}
	}

	srcRoutes := value.AsSlice(srcData["routes"])
	for _, route := range srcRoutes {
		routeMap := value.AsMap(route)
		key := routeKey(routeMap)
		if key == "" {
			continue
		}
		if idx, ok := routeIndex[key]; ok {
			existingRoutes[idx] = route
		} else {
			existingRoutes = append(existingRoutes, route)
			routeIndex[key] = len(existingRoutes) - 1
		}
	}

	merged := map[string]any{
		"routes": existingRoutes,
	}

	return atomicWriteYaml(destPath, merged)
}

// routeKey returns a unique key for a route based on path and method.
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

// mergeResourcesYml merges resources.yml with last-write-wins for resource names.
func mergeResourcesYml(srcDir, destDir string) error {
	srcPath := filepath.Join(srcDir, "resources.yml")
	destPath := filepath.Join(destDir, "resources.yml")

	srcData, err := loadYamlFile(srcPath)
	if err != nil {
		return err
	}

	existingData, _ := loadYamlFile(destPath)

	srcResources := value.AsMap(srcData["resources"])
	if srcResources == nil {
		srcResources = make(map[string]any)
	}
	existingResources := value.AsMap(existingData["resources"])
	if existingResources == nil {
		existingResources = make(map[string]any)
	}

	srcDynamo := value.AsSlice(srcResources["dynamodb"])
	existingDynamo := value.AsSlice(existingResources["dynamodb"])
	mergedDynamo := mergeResourceList(existingDynamo, srcDynamo, "TableName")
	if len(mergedDynamo) > 0 {
		existingResources["dynamodb"] = mergedDynamo
	}

	srcS3 := value.AsSlice(srcResources["s3"])
	existingS3 := value.AsSlice(existingResources["s3"])
	mergedS3 := mergeResourceList(existingS3, srcS3, "BucketName")
	if len(mergedS3) > 0 {
		existingResources["s3"] = mergedS3
	}

	srcLayers := value.AsSlice(srcResources["layers"])
	existingLayers := value.AsSlice(existingResources["layers"])
	mergedLayers := mergeResourceList(existingLayers, srcLayers, "Name")
	if len(mergedLayers) > 0 {
		existingResources["layers"] = mergedLayers
	}

	merged := map[string]any{
		"resources": existingResources,
	}

	return atomicWriteYaml(destPath, merged)
}

// mergeResourceList merges two resource lists using the specified key for deduplication.
func mergeResourceList(existing, src []any, keyField string) []any {
	index := make(map[string]int)
	for i, item := range existing {
		m := value.AsMap(item)
		if key, ok := m[keyField].(string); ok && key != "" {
			index[key] = i
		}
	}

	for _, item := range src {
		m := value.AsMap(item)
		key, ok := m[keyField].(string)
		if !ok || key == "" {
			continue
		}
		if idx, found := index[key]; found {
			existing[idx] = item
		} else {
			existing = append(existing, item)
			index[key] = len(existing) - 1
		}
	}

	return existing
}
