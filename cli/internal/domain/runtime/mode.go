// Where: cli/internal/domain/runtime/mode.go
// What: Runtime mode normalization and inference helpers.
// Why: Keep deploy mode logic pure and reusable across command/usecase layers.
package runtime

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var errUnsupportedMode = errors.New("unsupported mode")

const (
	ModeDocker     = "docker"
	ModeContainerd = "containerd"
)

// ContainerInfo is the minimal container state needed for mode inference.
type ContainerInfo struct {
	Service string
	State   string
}

// NormalizeMode validates and normalizes the runtime mode string.
func NormalizeMode(mode string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(mode))
	switch trimmed {
	case ModeDocker, ModeContainerd:
		return trimmed, nil
	default:
		return "", fmt.Errorf("%w: %s", errUnsupportedMode, mode)
	}
}

// FallbackMode returns docker when the previous value is empty or invalid.
func FallbackMode(previous string) string {
	trimmed := strings.TrimSpace(previous)
	if trimmed == "" {
		return ModeDocker
	}
	mode, err := NormalizeMode(trimmed)
	if err != nil {
		return ModeDocker
	}
	return mode
}

// InferModeFromComposeFiles infers mode from compose file names.
func InferModeFromComposeFiles(files []string) string {
	for _, file := range files {
		base := strings.ToLower(filepath.Base(file))
		if strings.Contains(base, ModeContainerd) {
			return ModeContainerd
		}
	}
	for _, file := range files {
		base := strings.ToLower(filepath.Base(file))
		if strings.Contains(base, ModeDocker) {
			return ModeDocker
		}
	}
	return ""
}

// InferModeFromContainers infers mode from container service names.
// It prioritizes runtime-node (containerd) over agent (docker).
func InferModeFromContainers(containers []ContainerInfo, runningOnly bool) string {
	hasRuntimeNode := false
	hasAgent := false
	for _, ctr := range containers {
		if runningOnly && !strings.EqualFold(ctr.State, "running") {
			continue
		}
		service := strings.TrimSpace(ctr.Service)
		switch service {
		case "runtime-node":
			hasRuntimeNode = true
		case "agent":
			hasAgent = true
		}
	}
	if hasRuntimeNode {
		return ModeContainerd
	}
	if hasAgent {
		return ModeDocker
	}
	return ""
}
