// Where: cli/internal/infra/build/file_ops.go
// What: Build package adapters for shared filesystem operations.
// Why: Preserve local helper call sites while centralizing implementation in infra/fileops.
package build

import (
	"github.com/poruru/edge-serverless-box/cli/internal/infra/fileops"
)

func ensureDir(path string) error {
	return fileops.EnsureDir(path)
}

func removeDir(path string) error {
	return fileops.RemoveDir(path)
}

func copyDir(src, dst string) error {
	return fileops.CopyDir(src, dst)
}

func copyFile(src, dst string) error {
	return fileops.CopyFile(src, dst)
}

func fileExists(path string) bool {
	return fileops.FileExists(path)
}
