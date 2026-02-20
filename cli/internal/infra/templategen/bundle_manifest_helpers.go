// Where: cli/internal/infra/templategen/bundle_manifest_helpers.go
// What: Shared helper utilities for bundle manifest generation.
// Why: Isolate metadata/path/hash helpers from manifest orchestration.
package templategen

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru-code/esb/cli/internal/infra/compose"
)

func resolveManifestTemplatePath(repoRoot, templatePath string) string {
	root := strings.TrimSpace(repoRoot)
	path := strings.TrimSpace(templatePath)
	if root == "" || path == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return filepath.ToSlash(rel)
}

func stringifyParameters(values map[string]any) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(values))
	for _, key := range keys {
		val := values[key]
		out[key] = fmt.Sprint(val)
	}
	return out
}

func resolveGitMetadata(ctx context.Context, runner compose.CommandRunner, repoRoot string) (string, bool, error) {
	commit, err := runGit(ctx, runner, repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", false, err
	}
	out, err := runner.RunOutput(ctx, repoRoot, "git", "status", "--porcelain")
	if err != nil {
		return "", false, fmt.Errorf("git status failed: %w", err)
	}
	dirty := strings.TrimSpace(string(out)) != ""
	return commit, dirty, nil
}

func hashFileSha256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read template: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
