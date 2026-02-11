// Where: cli/internal/infra/build/bake_args.go
// What: Bake allow-argument and secret path helpers.
// Why: Keep command permission argument construction cohesive and testable.
package build

import (
	"path/filepath"
	"sort"
	"strings"
)

func bakeAllowArgs(targets []bakeTarget) []string {
	readPaths := make(map[string]struct{})
	for _, target := range targets {
		addBakeReadPath(readPaths, target.Context)
		for _, contextPath := range target.Contexts {
			addBakeReadPath(readPaths, contextPath)
		}
		for _, secret := range target.Secrets {
			path := parseBakeSecretPath(secret)
			if path == "" {
				continue
			}
			readPaths[path] = struct{}{}
		}
	}
	if len(readPaths) == 0 {
		return nil
	}
	args := make([]string, 0, len(readPaths))
	ordered := make([]string, 0, len(readPaths))
	for path := range readPaths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		args = append(args, "--allow=fs.read="+path)
	}
	return args
}

func addBakeReadPath(readPaths map[string]struct{}, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	if strings.HasPrefix(value, "target:") {
		return
	}
	if strings.Contains(value, "://") {
		return
	}
	if !filepath.IsAbs(value) {
		return
	}
	readPaths[value] = struct{}{}
}

func parseBakeSecretPath(spec string) string {
	return parseBakeKeyValuePath(spec, "src")
}

func parseBakeKeyValuePath(spec, key string) string {
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parsedKey, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(parsedKey) != key {
			continue
		}
		return strings.TrimSpace(value)
	}
	return ""
}
