package artifactcore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func mergeFunctionsYML(srcDir, destDir string) error {
	srcPath := filepath.Join(srcDir, "functions.yml")
	destPath := filepath.Join(destDir, "functions.yml")

	srcData, srcExists, err := loadYAML(srcPath)
	if err != nil {
		return wrapRequiredSourceLoadError(srcPath, err)
	}
	if !srcExists {
		return fmt.Errorf("required file not found: %w", MissingReferencedPathError{Path: srcPath})
	}
	if srcData == nil {
		srcData = map[string]any{}
	}

	existingData, _, err := loadYAML(destPath)
	if err != nil {
		return err
	}
	if existingData == nil {
		existingData = map[string]any{}
	}

	srcFunctions := asMap(srcData["functions"])
	existingFunctions := asMap(existingData["functions"])
	if existingFunctions == nil {
		existingFunctions = make(map[string]any)
	}
	for name, fn := range srcFunctions {
		existingFunctions[name] = fn
	}

	srcDefaults := asMap(srcData["defaults"])
	existingDefaults := asMap(existingData["defaults"])
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
	return atomicWriteYAML(destPath, merged)
}

func mergeDefaultsSection(existingDefaults, srcDefaults map[string]any, key string) {
	if srcDefaults == nil {
		return
	}
	srcSection := asMap(srcDefaults[key])
	if srcSection == nil {
		return
	}
	existingSection := asMap(existingDefaults[key])
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

func mergeRoutingYML(srcDir, destDir string) error {
	srcPath := filepath.Join(srcDir, "routing.yml")
	destPath := filepath.Join(destDir, "routing.yml")

	srcData, srcExists, err := loadYAML(srcPath)
	if err != nil {
		return wrapRequiredSourceLoadError(srcPath, err)
	}
	if !srcExists {
		return fmt.Errorf("required file not found: %w", MissingReferencedPathError{Path: srcPath})
	}
	if srcData == nil {
		srcData = map[string]any{}
	}

	existingData, _, err := loadYAML(destPath)
	if err != nil {
		return err
	}
	if existingData == nil {
		existingData = map[string]any{}
	}

	existingRoutes := asSlice(existingData["routes"])
	routeIndex := make(map[string]int)
	for i, route := range existingRoutes {
		key := routeKey(asMap(route))
		if key == "" {
			continue
		}
		routeIndex[key] = i
	}

	srcRoutes := asSlice(srcData["routes"])
	for _, route := range srcRoutes {
		routeMap := asMap(route)
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

	merged := map[string]any{"routes": existingRoutes}
	return atomicWriteYAML(destPath, merged)
}

func routeKey(route map[string]any) string {
	pathVal, _ := route["path"].(string)
	method, _ := route["method"].(string)
	if pathVal == "" {
		return ""
	}
	if method == "" {
		method = "GET"
	}
	return fmt.Sprintf("%s:%s", pathVal, method)
}

func mergeResourcesYML(srcDir, destDir string) error {
	srcPath := filepath.Join(srcDir, "resources.yml")
	destPath := filepath.Join(destDir, "resources.yml")

	srcData, srcExists, err := loadYAML(srcPath)
	if err != nil {
		return err
	}
	if !srcExists {
		return nil
	}
	if srcData == nil {
		srcData = map[string]any{}
	}

	existingData, _, err := loadYAML(destPath)
	if err != nil {
		return err
	}
	if existingData == nil {
		existingData = map[string]any{}
	}

	srcResources := asMap(srcData["resources"])
	if srcResources == nil {
		srcResources = make(map[string]any)
	}
	existingResources := asMap(existingData["resources"])
	if existingResources == nil {
		existingResources = make(map[string]any)
	}

	mergedDynamo := mergeResourceList(
		asSlice(existingResources["dynamodb"]),
		asSlice(srcResources["dynamodb"]),
		"TableName",
	)
	if len(mergedDynamo) > 0 {
		existingResources["dynamodb"] = mergedDynamo
	}

	mergedS3 := mergeResourceList(
		asSlice(existingResources["s3"]),
		asSlice(srcResources["s3"]),
		"BucketName",
	)
	if len(mergedS3) > 0 {
		existingResources["s3"] = mergedS3
	}

	mergedLayers := mergeResourceList(
		asSlice(existingResources["layers"]),
		asSlice(srcResources["layers"]),
		"Name",
	)
	if len(mergedLayers) > 0 {
		existingResources["layers"] = mergedLayers
	}

	merged := map[string]any{"resources": existingResources}
	return atomicWriteYAML(destPath, merged)
}

func mergeResourceList(existing, src []any, keyField string) []any {
	index := make(map[string]int)
	for i, item := range existing {
		m := asMap(item)
		key, _ := m[keyField].(string)
		if key == "" {
			continue
		}
		index[key] = i
	}
	for _, item := range src {
		m := asMap(item)
		key, _ := m[keyField].(string)
		if key == "" {
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

func wrapRequiredSourceLoadError(srcPath string, err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return fmt.Errorf("required file not found: %w", MissingReferencedPathError{Path: srcPath})
	}
	return err
}
