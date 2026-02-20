// Where: cli/internal/infra/build/file_ops.go
// What: Build package adapters for shared filesystem operations.
// Why: Preserve local helper call sites while centralizing implementation in infra/fileops.
package build

import (
	"github.com/poruru-code/esb/cli/internal/infra/fileops"
)

func ensureDir(path string) error {
	return fileops.EnsureDir(path)
}

func fileExists(path string) bool {
	return fileops.FileExists(path)
}
