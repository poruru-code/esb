// Where: cli/internal/infra/templategen/file_ops.go
// What: Template generator adapters for shared filesystem operations.
// Why: Preserve local helper call sites while centralizing implementation in infra/fileops.
package templategen

import (
	"io/fs"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/fileops"
)

func ensureDir(path string) error {
	return fileops.EnsureDir(path)
}

func removeDir(path string) error {
	return fileops.RemoveDir(path)
}

func writeFile(path, content string) error {
	return fileops.WriteFile(path, content)
}

func writeConfigFile(path, content string) error {
	return fileops.WriteConfigFile(path, content)
}

func copyDir(src, dst string) error {
	return fileops.CopyDir(src, dst)
}

func copyFile(src, dst string) error {
	return fileops.CopyFile(src, dst)
}

func linkOrCopyFile(src, dst string, mode fs.FileMode) error {
	return fileops.LinkOrCopyFile(src, dst, mode)
}

func copyDirLinkOrCopy(src, dst string) error {
	return fileops.CopyDirLinkOrCopy(src, dst)
}

func extractZipLayer(src, cacheDir string) (string, error) {
	return fileops.ExtractZipLayer(src, cacheDir)
}

func fileExists(path string) bool {
	return fileops.FileExists(path)
}

func dirExists(path string) bool {
	return fileops.DirExists(path)
}

func fileOrDirExists(path string) bool {
	return fileops.FileOrDirExists(path)
}
