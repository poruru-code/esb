// Where: cli/internal/infra/build/merge_config_image_import.go
// What: image-import manifest merge logic.
// Why: Keep JSON manifest behavior separate from YAML merge concerns.
package build

import (
	"path/filepath"
	"sort"
	"strings"
)

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
