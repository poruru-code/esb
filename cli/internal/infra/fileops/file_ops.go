// Where: cli/internal/infra/fileops/file_ops.go
// What: Shared filesystem operations for build and template generation.
// Why: Keep behavior consistent and avoid duplicated I/O helper implementations.
package fileops

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxZipEntryBytes int64 = 200 << 20 // 200 MiB safety cap for zip extraction.

func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func RemoveDir(path string) error {
	if path == "" {
		return nil
	}
	if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func WriteFile(path, content string) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func WriteConfigFile(path, content string) error {
	if err := EnsureDir(filepath.Dir(path)); err != nil {
		return err
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func CopyDir(src, dst string) error {
	if err := EnsureDir(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return EnsureDir(target)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFileWithMode(path, target, info.Mode())
	})
}

func CopyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return copyFileWithMode(src, dst, info.Mode())
}

func copyFileWithMode(src, dst string, mode fs.FileMode) error {
	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

func LinkOrCopyFile(src, dst string, mode fs.FileMode) error {
	if err := EnsureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	if err := removePathIfExists(dst); err != nil {
		return err
	}
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyFileWithMode(src, dst, mode)
}

func CopyDirLinkOrCopy(src, dst string) error {
	if err := EnsureDir(dst); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return EnsureDir(target)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return LinkOrCopyFile(path, target, info.Mode())
	})
}

func ExtractZipLayer(src, cacheDir string) (string, error) {
	if strings.TrimSpace(cacheDir) == "" {
		return "", fmt.Errorf("layer cache dir is required")
	}
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}

	base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	identifier := fmt.Sprintf("%s_%d_%d", base, info.ModTime().Unix(), info.Size())
	dest := filepath.Join(cacheDir, identifier)
	if DirExists(dest) {
		return dest, nil
	}

	tmp := dest + ".tmp"
	if err := RemoveDir(tmp); err != nil {
		return "", err
	}
	if err := EnsureDir(tmp); err != nil {
		return "", err
	}
	if err := unzipFile(src, tmp); err != nil {
		_ = RemoveDir(tmp)
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func unzipFile(src, dst string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()

	for _, file := range reader.File {
		//nolint:gosec // Path traversal is checked below with cleaned prefix validation.
		targetPath := filepath.Join(dst, file.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("zip path escapes target: %s", file.Name)
		}

		if file.FileInfo().IsDir() {
			if err := EnsureDir(targetPath); err != nil {
				return err
			}
			continue
		}

		if err := EnsureDir(filepath.Dir(targetPath)); err != nil {
			return err
		}

		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(targetPath)
		if err != nil {
			in.Close()
			return err
		}
		if file.UncompressedSize64 > 0 && file.UncompressedSize64 > uint64(maxZipEntryBytes) {
			in.Close()
			out.Close()
			return fmt.Errorf("zip entry too large: %s", file.Name)
		}
		limited := io.LimitReader(in, maxZipEntryBytes+1)
		written, err := io.Copy(out, limited)
		if err != nil {
			in.Close()
			out.Close()
			return err
		}
		if written > maxZipEntryBytes {
			in.Close()
			out.Close()
			return fmt.Errorf("zip entry too large: %s", file.Name)
		}
		in.Close()
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

func removePathIfExists(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func FileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func FileOrDirExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
