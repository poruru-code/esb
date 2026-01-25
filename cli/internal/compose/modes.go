package compose

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	ModeDocker     = "docker"
	ModeContainerd = "containerd"
)

// resolveMode normalizes the mode string.
func resolveMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return ModeDocker
	}
	return mode
}

// ResolveComposeFiles returns the list of compose files for a given mode and target.
// This logic was previously in up.go but is required for build.
func ResolveComposeFiles(rootDir, mode, _ string) ([]string, error) {
	var files []string

	switch mode {
	case ModeDocker:
		files = append(files, filepath.Join(rootDir, "docker-compose.docker.yml"))
	case ModeContainerd:
		files = append(files, filepath.Join(rootDir, "docker-compose.containerd.yml"))
	default:
		return nil, fmt.Errorf("unsupported mode: %s", mode)
	}

	return files, nil
}
