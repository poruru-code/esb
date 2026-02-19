package artifactcore

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrRuntimeBaseDockerfileMissing = errors.New("runtime base dockerfile missing")
	ErrSecretEnvFileRequired        = errors.New("secret env file required")
)

type MissingSecretKeysError struct {
	Keys []string
}

func (e MissingSecretKeysError) Error() string {
	if len(e.Keys) == 0 {
		return "missing required secret env keys"
	}
	keys := append([]string(nil), e.Keys...)
	sort.Strings(keys)
	return fmt.Sprintf("missing required secret env keys: %s", strings.Join(keys, ", "))
}

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
