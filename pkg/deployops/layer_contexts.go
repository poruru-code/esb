package deployops

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	layerCacheSchemaVersion       = "v1"
	defaultBrandSlug              = "esb"
	maxLayerExtractBytes    int64 = 1 << 30 // 1 GiB
)

func prepareFunctionLayerBuildContexts(
	repoRoot, contextRoot, functionName string,
) (map[string]string, error) {
	dockerfile := filepath.Join(contextRoot, "functions", functionName, "Dockerfile")
	dockerfileData, err := os.ReadFile(dockerfile)
	if err != nil {
		return nil, fmt.Errorf("read function dockerfile %s: %w", dockerfile, err)
	}
	aliases := parseLayerContextAliases(string(dockerfileData))
	if len(aliases) == 0 {
		return nil, nil
	}
	stageAliases := parseDockerfileStageAliases(string(dockerfileData))

	layersDir := filepath.Join(contextRoot, "functions", functionName, "layers")
	zipByName, err := discoverLayerZipFiles(layersDir)
	if err != nil {
		return nil, err
	}

	cacheRoot := filepath.Join(repoRoot, resolveBrandHomeDir(repoRoot), "cache", "layers")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create layer cache root %s: %w", cacheRoot, err)
	}
	nestPython := isPythonLayerLayoutRequired(string(dockerfileData))
	contexts := make(map[string]string, len(aliases))

	for _, alias := range aliases {
		targetName, _ := layerAliasTargetName(alias)
		zipPath, ok := zipByName[targetName]
		if !ok {
			if _, hasStage := stageAliases[alias]; hasStage {
				continue
			}
			return nil, fmt.Errorf("layer archive for alias %q not found in %s", alias, layersDir)
		}
		extracted, err := prepareLayerArchiveCache(cacheRoot, zipPath, nestPython)
		if err != nil {
			return nil, fmt.Errorf("prepare layer cache for alias %q: %w", alias, err)
		}
		contexts[alias] = extracted
	}
	if len(contexts) == 0 {
		return nil, nil
	}
	return contexts, nil
}

func parseLayerContextAliases(dockerfile string) []string {
	seen := map[string]struct{}{}
	aliases := make([]string, 0)
	for _, line := range dockerfileLogicalLines(dockerfile) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 4 || !strings.EqualFold(fields[0], "COPY") {
			continue
		}
		fromAlias := ""
		positional := make([]string, 0, 2)
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "--from=") {
				fromAlias = strings.TrimSpace(strings.TrimPrefix(field, "--from="))
				continue
			}
			if strings.HasPrefix(field, "--") {
				continue
			}
			positional = append(positional, field)
		}
		if fromAlias == "" || len(positional) < 2 {
			continue
		}
		src := strings.TrimSpace(positional[0])
		dst := strings.TrimSpace(positional[1])
		if src != "/" {
			continue
		}
		if dst != "/opt" && dst != "/opt/" {
			continue
		}
		if _, ok := layerAliasTargetName(fromAlias); !ok {
			continue
		}
		if _, ok := seen[fromAlias]; ok {
			continue
		}
		seen[fromAlias] = struct{}{}
		aliases = append(aliases, fromAlias)
	}
	sort.Strings(aliases)
	return aliases
}

func parseDockerfileStageAliases(dockerfile string) map[string]struct{} {
	aliases := map[string]struct{}{}
	for _, line := range dockerfileLogicalLines(dockerfile) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "FROM") {
			continue
		}
		for i := 1; i < len(fields)-1; i++ {
			if !strings.EqualFold(fields[i], "AS") {
				continue
			}
			alias := strings.TrimSpace(fields[i+1])
			if alias == "" {
				continue
			}
			aliases[alias] = struct{}{}
			break
		}
	}
	return aliases
}

func discoverLayerZipFiles(layersDir string) (map[string]string, error) {
	entries, err := os.ReadDir(layersDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read layer directory %s: %w", layersDir, err)
	}
	zipByName := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if !strings.HasSuffix(strings.ToLower(name), ".zip") {
			continue
		}
		targetName := strings.TrimSuffix(name, filepath.Ext(name))
		if targetName == "" {
			continue
		}
		zipByName[targetName] = filepath.Join(layersDir, name)
	}
	return zipByName, nil
}

func layerAliasTargetName(alias string) (string, bool) {
	const prefix = "layer_"
	if !strings.HasPrefix(alias, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(alias, prefix)
	sep := strings.IndexByte(rest, '_')
	if sep <= 0 {
		return "", false
	}
	indexPart := rest[:sep]
	if !isDigits(indexPart) {
		return "", false
	}
	targetName := strings.TrimSpace(rest[sep+1:])
	if targetName == "" {
		return "", false
	}
	return targetName, true
}

func isDigits(value string) bool {
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return value != ""
}

func isPythonLayerLayoutRequired(dockerfile string) bool {
	for _, line := range dockerfileLogicalLines(dockerfile) {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 || !strings.EqualFold(fields[0], "ENV") {
			continue
		}
		assignments := fields[1:]
		for i, assignment := range assignments {
			if strings.Contains(assignment, "=") {
				key, value, _ := strings.Cut(assignment, "=")
				if strings.EqualFold(strings.TrimSpace(key), "PYTHONPATH") &&
					strings.Contains(value, "/opt/python") {
					return true
				}
				continue
			}
			if strings.EqualFold(strings.TrimSpace(assignment), "PYTHONPATH") &&
				i+1 < len(assignments) &&
				strings.Contains(assignments[i+1], "/opt/python") {
				return true
			}
		}
	}
	return false
}

func dockerfileLogicalLines(dockerfile string) []string {
	rawLines := strings.Split(dockerfile, "\n")
	lines := make([]string, 0, len(rawLines))
	var current strings.Builder
	for _, raw := range rawLines {
		trimmedRight := strings.TrimRight(raw, " \t\r")
		continued := strings.HasSuffix(trimmedRight, "\\")
		part := strings.TrimSuffix(trimmedRight, "\\")
		part = strings.TrimRight(part, " \t")
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
		current.WriteString(part)
		if continued {
			continue
		}
		lines = append(lines, current.String())
		current.Reset()
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func resolveBrandHomeDir(repoRoot string) string {
	slug := sanitizeBrandSlug(readBrandingSlugFile(filepath.Join(repoRoot, ".branding.env")))
	if slug == "" {
		slug = sanitizeBrandSlug(strings.TrimSpace(os.Getenv("BRANDING_SLUG")))
	}
	if slug == "" {
		slug = defaultBrandSlug
	}
	return "." + slug
}

func sanitizeBrandSlug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	result := strings.TrimSpace(b.String())
	if result == "" {
		return ""
	}
	return result
}

func readBrandingSlugFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "export ")
		if !strings.HasPrefix(trimmed, "BRANDING_SLUG=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "BRANDING_SLUG="))
		value = strings.Trim(value, "\"'")
		if value != "" {
			return value
		}
	}
	return ""
}

func prepareLayerArchiveCache(cacheRoot, archivePath string, nestPython bool) (string, error) {
	archiveDigest, err := hashFileSHA256(archivePath)
	if err != nil {
		return "", err
	}
	mode := "plain"
	prefix := ""
	if nestPython {
		hasLayout, err := zipHasPythonLayout(archivePath)
		if err != nil {
			return "", err
		}
		if hasLayout {
			mode = "python-layout"
		} else {
			mode = "python-prefixed"
			prefix = "python"
		}
	}
	cacheKey := layerCacheKey(archiveDigest, mode)
	dest := filepath.Join(cacheRoot, cacheKey)
	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}

	tmpDir := fmt.Sprintf("%s.tmp-%d", dest, os.Getpid())
	_ = os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", err
	}
	if err := extractZipToDir(archivePath, tmpDir, prefix); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", err
	}
	if err := os.Rename(tmpDir, dest); err != nil {
		_ = os.RemoveAll(tmpDir)
		if _, statErr := os.Stat(dest); statErr == nil {
			return dest, nil
		}
		return "", err
	}
	return dest, nil
}

func layerCacheKey(archiveDigest, mode string) string {
	seed := strings.Join([]string{layerCacheSchemaVersion, archiveDigest, mode}, ":")
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:8])
}

func hashFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func zipHasPythonLayout(path string) (bool, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return false, err
	}
	defer reader.Close()
	for _, file := range reader.File {
		normalized := strings.TrimPrefix(filepath.ToSlash(file.Name), "/")
		if normalized == "" {
			continue
		}
		head, _, _ := strings.Cut(normalized, "/")
		switch strings.ToLower(strings.TrimSpace(head)) {
		case "python", "site-packages":
			return true, nil
		}
	}
	return false, nil
}

func extractZipToDir(src, dst, prefix string) error {
	return extractZipToDirWithLimit(src, dst, prefix, maxLayerExtractBytes)
}

func extractZipToDirWithLimit(src, dst, prefix string, maxExtractBytes int64) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	base := filepath.Clean(dst)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}

	if maxExtractBytes <= 0 {
		return fmt.Errorf("zip extraction limit must be positive")
	}

	var extractedTotal int64
	for _, file := range reader.File {
		name := filepath.Clean(file.Name)
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(filepath.Separator)) || name == ".." {
			return fmt.Errorf("invalid zip entry: %s", file.Name)
		}
		if prefix != "" {
			name = filepath.Join(prefix, name)
		}
		target := filepath.Join(base, name)
		if !strings.HasPrefix(target, base+string(filepath.Separator)) && target != base {
			return fmt.Errorf("zip entry escapes destination: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		remaining := maxExtractBytes - extractedTotal
		if remaining <= 0 {
			return fmt.Errorf("zip extraction exceeds limit: %d bytes", maxExtractBytes)
		}
		if file.UncompressedSize64 > uint64(remaining) {
			return fmt.Errorf("zip extraction exceeds limit: %d bytes", maxExtractBytes)
		}
		srcFile, err := file.Open()
		if err != nil {
			return err
		}
		dstFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode())
		if err != nil {
			_ = srcFile.Close()
			return err
		}
		written, copyErr := io.Copy(dstFile, io.LimitReader(srcFile, remaining))
		if copyErr == nil && written == remaining {
			probe := make([]byte, 1)
			probeBytes, probeErr := srcFile.Read(probe)
			if probeBytes > 0 {
				_ = os.Remove(target)
				copyErr = fmt.Errorf("zip extraction exceeds limit: %d bytes", maxExtractBytes)
			} else if probeErr != nil && probeErr != io.EOF {
				copyErr = probeErr
			}
		}
		closeErr := srcFile.Close()
		if flushErr := dstFile.Close(); copyErr == nil {
			copyErr = flushErr
		}
		if copyErr == nil && closeErr != nil {
			copyErr = closeErr
		}
		if copyErr != nil {
			return copyErr
		}
		extractedTotal += written
		if extractedTotal > maxExtractBytes {
			return fmt.Errorf("zip extraction exceeds limit: %d bytes", maxExtractBytes)
		}
	}
	return nil
}

func resolveRepoRoot(manifestPath, artifactRoot string) string {
	candidates := []string{artifactRoot, filepath.Dir(manifestPath)}
	for _, candidate := range candidates {
		if root, ok := findAncestorWithFile(candidate, ".branding.env"); ok {
			return root
		}
	}
	for _, candidate := range candidates {
		if root, ok := findAncestorWithDir(candidate, ".git"); ok {
			return root
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return filepath.Dir(manifestPath)
}

func findAncestorWithFile(start, name string) (string, bool) {
	current := filepath.Clean(start)
	for {
		info, err := os.Stat(filepath.Join(current, name))
		if err == nil && !info.IsDir() {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func findAncestorWithDir(start, name string) (string, bool) {
	current := filepath.Clean(start)
	for {
		info, err := os.Stat(filepath.Join(current, name))
		if err == nil && info.IsDir() {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}
