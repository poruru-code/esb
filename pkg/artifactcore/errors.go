package artifactcore

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrArtifactPathRequired = errors.New("artifact path is required")
	ErrOutputDirRequired    = errors.New("output dir is required")
)

type MissingReferencedPathError struct {
	Path string
}

func (e MissingReferencedPathError) Error() string {
	path := strings.TrimSpace(e.Path)
	if path == "" {
		return "referenced path not found"
	}
	return fmt.Sprintf("referenced path not found: %s", path)
}
