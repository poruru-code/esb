// Where: cli/internal/generator/build_fingerprint.go
// What: Build fingerprint helpers for image change detection.
// Why: Tie cache keys to project/env and generated outputs.
package generator

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru/edge-serverless-box/cli/internal/staging"
)

func buildImageFingerprint(outputDir, composeProject, env, baseImageID string, functions []FunctionSpec) (string, error) {
	outputHash, err := outputFingerprint(outputDir, functions)
	if err != nil {
		return "", err
	}
	stageKey := staging.CacheKey(composeProject, env)
	seed := fmt.Sprintf("%s:%s:%s", stageKey, strings.TrimSpace(baseImageID), outputHash)
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:]), nil
}

func outputFingerprint(outputDir string, functions []FunctionSpec) (string, error) {
	if strings.TrimSpace(outputDir) == "" {
		return "", fmt.Errorf("output dir is required")
	}
	configDir := filepath.Join(outputDir, "config")
	paths := []string{
		filepath.Join(configDir, "functions.yml"),
		filepath.Join(configDir, "routing.yml"),
	}
	for _, path := range paths {
		if !fileExists(path) {
			return "", fmt.Errorf("config not found: %s", path)
		}
	}
	for _, fn := range functions {
		if strings.TrimSpace(fn.Name) == "" {
			continue
		}
		paths = append(paths, filepath.Join(outputDir, "functions", fn.Name))
	}
	return hashPaths(outputDir, paths)
}

func hashPaths(baseDir string, paths []string) (string, error) {
	hasher := sha256.New()
	files := make([]string, 0)
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			err = filepath.WalkDir(path, func(entryPath string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() {
					return nil
				}
				files = append(files, entryPath)
				return nil
			})
			if err != nil {
				return "", err
			}
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	for _, path := range files {
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			rel = path
		}
		_, _ = hasher.Write([]byte(filepath.ToSlash(rel)))
		_, _ = hasher.Write([]byte{0})
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(hasher, file); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
		_, _ = hasher.Write([]byte{0})
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
