// Where: cli/internal/infra/build/git_context.go
// What: Git context resolution helpers for traceability build contexts.
// Why: Support worktree and standard clones without relying on .git in build context.
package build

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type gitContext struct {
	GitDir    string
	GitCommon string
}

type gitRunner interface {
	RunOutput(ctx context.Context, dir, name string, args ...string) ([]byte, error)
}

func resolveGitContext(ctx context.Context, runner gitRunner, repoRoot string) (gitContext, error) {
	if runner == nil {
		return gitContext{}, fmt.Errorf("git runner is required")
	}
	root := filepath.Clean(strings.TrimSpace(repoRoot))
	if root == "" {
		return gitContext{}, fmt.Errorf("repo root is required")
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return gitContext{}, fmt.Errorf("repo root resolve failed: %w", err)
	}
	top, err := runGit(ctx, runner, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return gitContext{}, err
	}
	topResolved, err := filepath.EvalSymlinks(top)
	if err != nil {
		return gitContext{}, fmt.Errorf("git top resolve failed: %w", err)
	}
	if filepath.Clean(topResolved) != filepath.Clean(rootResolved) {
		return gitContext{}, fmt.Errorf("repo root mismatch: %s", top)
	}
	gitDirRaw, err := runGit(ctx, runner, root, "rev-parse", "--git-dir")
	if err != nil {
		return gitContext{}, err
	}
	gitCommonRaw, err := runGit(ctx, runner, root, "rev-parse", "--git-common-dir")
	if err != nil {
		return gitContext{}, err
	}
	gitDir, gitDirIsFile, err := resolveGitDir(root, gitDirRaw)
	if err != nil {
		return gitContext{}, err
	}
	gitCommon := resolveGitCommon(root, gitDir, gitDirIsFile, gitCommonRaw)
	if _, err := os.Stat(filepath.Join(gitDir, "HEAD")); err != nil {
		return gitContext{}, fmt.Errorf("gitdir missing HEAD: %w", err)
	}
	if _, err := os.Stat(filepath.Join(gitCommon, "objects")); err != nil {
		return gitContext{}, fmt.Errorf("git common dir missing objects: %w", err)
	}
	return gitContext{GitDir: gitDir, GitCommon: gitCommon}, nil
}

func runGit(ctx context.Context, runner gitRunner, root string, args ...string) (string, error) {
	out, err := runner.RunOutput(ctx, root, "git", args...)
	if err != nil {
		msg := strings.TrimSpace(string(out))
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, msg)
	}
	val := strings.TrimSpace(string(out))
	if val == "" {
		return "", fmt.Errorf("git %s returned empty output", strings.Join(args, " "))
	}
	return val, nil
}

func resolveGitDir(root, gitDirRaw string) (string, bool, error) {
	gitDirPath := resolveAbs(root, gitDirRaw)
	info, err := os.Stat(gitDirPath)
	if err != nil {
		return "", false, fmt.Errorf("gitdir not found: %w", err)
	}
	if info.IsDir() {
		return gitDirPath, false, nil
	}
	content, err := os.ReadFile(gitDirPath)
	if err != nil {
		return "", false, fmt.Errorf("gitdir read failed: %w", err)
	}
	line := strings.TrimSpace(string(content))
	if !strings.HasPrefix(line, "gitdir: ") {
		return "", false, fmt.Errorf("gitdir file format invalid")
	}
	target := strings.TrimSpace(strings.TrimPrefix(line, "gitdir: "))
	if target == "" {
		return "", false, fmt.Errorf("gitdir file is empty")
	}
	return resolveAbs(filepath.Dir(gitDirPath), target), true, nil
}

func resolveGitCommon(root, gitDir string, gitDirIsFile bool, gitCommonRaw string) string {
	candidates := []string{root}
	if gitDirIsFile {
		candidates = append(candidates, gitDir)
	} else if gitDir != "" {
		candidates = append(candidates, gitDir)
	}
	for _, base := range candidates {
		candidate := resolveAbs(base, gitCommonRaw)
		if _, err := os.Stat(filepath.Join(candidate, "objects")); err == nil {
			return candidate
		}
	}
	return resolveAbs(root, gitCommonRaw)
}

func resolveAbs(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(base, path))
}
