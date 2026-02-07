// Where: cli/internal/infra/build/merge_config.go
// What: Configuration merge logic for deploy workflow.
// Why: Support multiple template deployments with last-write-wins merge strategy.
package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/value"
	"github.com/poruru/edge-serverless-box/cli/internal/infra/staging"
	"gopkg.in/yaml.v3"
)

const deployLockTimeout = 30 * time.Second

// MergeConfig merges new configuration files into the existing CONFIG_DIR.
// It implements the "last-write-wins" merge strategy for multiple deployments.
func MergeConfig(outputDir, templatePath, composeProject, env string) error {
	configDir, err := staging.ConfigDir(templatePath, composeProject, env)
	if err != nil {
		return err
	}
	if err := ensureDir(configDir); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	// Acquire deploy lock
	lockPath := filepath.Join(configDir, ".deploy.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}
	defer func() {
		_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		_ = lockFile.Close()
	}()
	if err := acquireDeployLock(lockFile, deployLockTimeout); err != nil {
		return fmt.Errorf("failed to acquire deploy lock: %w", err)
	}

	srcConfigDir := filepath.Join(outputDir, "config")

	// Merge functions.yml
	if err := mergeFunctionsYml(srcConfigDir, configDir); err != nil {
		return fmt.Errorf("failed to merge functions.yml: %w", err)
	}

	// Merge routing.yml
	if err := mergeRoutingYml(srcConfigDir, configDir); err != nil {
		return fmt.Errorf("failed to merge routing.yml: %w", err)
	}

	// Merge resources.yml
	if err := mergeResourcesYml(srcConfigDir, configDir); err != nil {
		return fmt.Errorf("failed to merge resources.yml: %w", err)
	}
	// Merge image-import.json
	if err := mergeImageImportManifest(srcConfigDir, configDir); err != nil {
		return fmt.Errorf("failed to merge image-import.json: %w", err)
	}

	return nil
}

// mergeFunctionsYml merges functions.yml with last-write-wins for function names.
// defaults are preserved from existing config, with missing keys filled in.
func mergeFunctionsYml(srcDir, destDir string) error {
	srcPath := filepath.Join(srcDir, "functions.yml")
	destPath := filepath.Join(destDir, "functions.yml")

	// Load source config
	srcData, err := loadYamlFile(srcPath)
	if err != nil {
		return err
	}

	// Load existing config (if any)
	existingData, _ := loadYamlFile(destPath)

	// Merge functions (last-write-wins)
	srcFunctions := value.AsMap(srcData["functions"])
	existingFunctions := value.AsMap(existingData["functions"])
	if existingFunctions == nil {
		existingFunctions = make(map[string]any)
	}
	for name, fn := range srcFunctions {
		existingFunctions[name] = fn
	}

	// Merge defaults (existing preserved, missing keys filled in)
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

	// Build merged config
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

	// Load source config
	srcData, err := loadYamlFile(srcPath)
	if err != nil {
		return err
	}

	// Load existing config (if any)
	existingData, _ := loadYamlFile(destPath)

	// Build route index from existing routes
	existingRoutes := value.AsSlice(existingData["routes"])
	routeIndex := make(map[string]int) // key -> index
	for i, route := range existingRoutes {
		key := routeKey(value.AsMap(route))
		if key != "" {
			routeIndex[key] = i
		}
	}

	// Merge new routes (last-write-wins)
	srcRoutes := value.AsSlice(srcData["routes"])
	for _, route := range srcRoutes {
		routeMap := value.AsMap(route)
		key := routeKey(routeMap)
		if key == "" {
			continue
		}
		if idx, ok := routeIndex[key]; ok {
			// Update existing route
			existingRoutes[idx] = route
		} else {
			// Add new route
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

	// Load source config
	srcData, err := loadYamlFile(srcPath)
	if err != nil {
		return err
	}

	// Load existing config (if any)
	existingData, _ := loadYamlFile(destPath)

	// Merge resources
	srcResources := value.AsMap(srcData["resources"])
	if srcResources == nil {
		srcResources = make(map[string]any)
	}
	existingResources := value.AsMap(existingData["resources"])
	if existingResources == nil {
		existingResources = make(map[string]any)
	}

	// Merge DynamoDB tables
	srcDynamo := value.AsSlice(srcResources["dynamodb"])
	existingDynamo := value.AsSlice(existingResources["dynamodb"])
	mergedDynamo := mergeResourceList(existingDynamo, srcDynamo, "TableName")
	if len(mergedDynamo) > 0 {
		existingResources["dynamodb"] = mergedDynamo
	}

	// Merge S3 buckets
	srcS3 := value.AsSlice(srcResources["s3"])
	existingS3 := value.AsSlice(existingResources["s3"])
	mergedS3 := mergeResourceList(existingS3, srcS3, "BucketName")
	if len(mergedS3) > 0 {
		existingResources["s3"] = mergedS3
	}

	// Merge Layers
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

func mergeImageImportManifest(srcDir, destDir string) error {
	srcPath := filepath.Join(srcDir, "image-import.json")
	destPath := filepath.Join(destDir, "image-import.json")

	src, srcExists, err := loadImageImportManifest(srcPath)
	if err != nil {
		return err
	}
	if !srcExists {
		return nil
	}

	existing, _, err := loadImageImportManifest(destPath)
	if err != nil {
		return err
	}
	merged := imageImportManifest{
		Version:    firstNonEmpty(src.Version, existing.Version, "1"),
		PushTarget: firstNonEmpty(src.PushTarget, existing.PushTarget),
		Images:     []imageImportEntry{},
	}

	index := map[string]imageImportEntry{}
	for _, entry := range existing.Images {
		key := imageImportKey(entry)
		if key == "" {
			continue
		}
		index[key] = entry
	}
	for _, entry := range src.Images {
		key := imageImportKey(entry)
		if key == "" {
			continue
		}
		index[key] = entry
	}

	keys := make([]string, 0, len(index))
	for key := range index {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		merged.Images = append(merged.Images, index[key])
	}
	return atomicWriteJSON(destPath, merged)
}

func acquireDeployLock(lockFile *os.File, timeout time.Duration) error {
	if lockFile == nil {
		return fmt.Errorf("lock file is nil")
	}
	deadline := time.Now().Add(timeout)
	for {
		err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return nil
		}
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout after %s", timeout)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		return err
	}
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

// loadYamlFile loads a YAML file into a map.
func loadYamlFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func loadImageImportManifest(path string) (imageImportManifest, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return imageImportManifest{}, false, nil
		}
		return imageImportManifest{}, false, err
	}
	var result imageImportManifest
	if err := json.Unmarshal(data, &result); err != nil {
		return imageImportManifest{}, false, err
	}
	if result.Images == nil {
		result.Images = []imageImportEntry{}
	}
	return result, true, nil
}

// atomicWriteYaml writes data to a YAML file atomically using tmp + rename.
func atomicWriteYaml(path string, data map[string]any) error {
	content, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0o600); err != nil {
		return err
	}

	// Sync to disk
	f, err := os.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	_ = f.Close()

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func atomicWriteJSON(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, content, 0o600); err != nil {
		return err
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	_ = f.Close()

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	return nil
}

func imageImportKey(entry imageImportEntry) string {
	if name := strings.TrimSpace(entry.FunctionName); name != "" {
		return name
	}
	source := strings.TrimSpace(entry.ImageSource)
	ref := strings.TrimSpace(entry.ImageRef)
	if source == "" && ref == "" {
		return ""
	}
	return source + "|" + ref
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
