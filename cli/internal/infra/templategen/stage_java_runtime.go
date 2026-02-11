// Where: cli/internal/infra/templategen/stage_java_runtime.go
// What: Java runtime artifact staging and build helpers.
// Why: Isolate Java-specific runtime preparation from generic staging logic.
package templategen

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/poruru/edge-serverless-box/meta"
)

const (
	containerM2SettingsPath = "/tmp/m2/settings.xml"
	containerM2RepoPath     = "/tmp/m2/repository"
	javaRuntimeBuildImage   = "public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7"
)

const (
	javaWrapperFileName = "lambda-java-wrapper.jar"
	javaAgentFileName   = "lambda-java-agent.jar"
)

func ensureJavaWrapperSource(ctx stageContext) (string, error) {
	if src := resolveJavaWrapperSource(ctx); src != "" {
		return src, nil
	}
	return "", fmt.Errorf("java wrapper jar not found after build")
}

func resolveJavaWrapperSource(ctx stageContext) string {
	runtimeDir, err := resolveJavaRuntimeDir(ctx)
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(runtimeDir, "extensions", "wrapper", javaWrapperFileName),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func ensureJavaAgentSource(ctx stageContext) (string, error) {
	if src := resolveJavaAgentSource(ctx); src != "" {
		return src, nil
	}
	return "", fmt.Errorf("java agent jar not found after build")
}

func resolveJavaAgentSource(ctx stageContext) string {
	runtimeDir, err := resolveJavaRuntimeDir(ctx)
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(runtimeDir, "extensions", "agent", javaAgentFileName),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func resolveJavaRuntimeDir(ctx stageContext) (string, error) {
	rel := filepath.Join("runtime", "java")
	candidates := []string{
		filepath.Clean(filepath.Join(ctx.ProjectRoot, rel)),
		filepath.Clean(filepath.Join(ctx.BaseDir, rel)),
	}
	for _, candidate := range candidates {
		if dirExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("java runtime directory not found")
}

func ensureJavaM2RepositoryCacheDir(ctx stageContext) (string, error) {
	if strings.TrimSpace(ctx.ProjectRoot) == "" {
		return "", fmt.Errorf("project root is required for java m2 cache")
	}
	cacheDir := filepath.Join(ctx.ProjectRoot, meta.HomeDir, "cache", "m2", "repository")
	if err := ensureDir(cacheDir); err != nil {
		return "", fmt.Errorf("prepare java m2 cache dir: %w", err)
	}
	return cacheDir, nil
}

func javaRuntimeMavenBuildLine() string {
	return fmt.Sprintf(
		"mvn -s %s -q -Dmaven.repo.local=%s -Dmaven.artifact.threads=1 -DskipTests "+
			"-pl ../extensions/wrapper,../extensions/agent -am package",
		containerM2SettingsPath,
		containerM2RepoPath,
	)
}

func buildJavaRuntimeJars(ctx stageContext) error {
	runtimeDir, err := resolveJavaRuntimeDir(ctx)
	if err != nil {
		return err
	}
	ctx.verbosef("  Building Java runtime jars in %s\n", runtimeDir)

	buildDir := filepath.Join(runtimeDir, "build")
	if !dirExists(buildDir) {
		return fmt.Errorf("java runtime build directory not found: %s", buildDir)
	}
	m2RepoCacheDir, err := ensureJavaM2RepositoryCacheDir(ctx)
	if err != nil {
		return err
	}

	args := []string{
		"run",
		"--rm",
	}
	if uid, gid := os.Getuid(), os.Getgid(); uid >= 0 && gid >= 0 {
		args = append(args, "--user", fmt.Sprintf("%d:%d", uid, gid))
	}
	settingsPath, err := writeTempMavenSettingsFile()
	if err != nil {
		return fmt.Errorf("invalid proxy configuration for java runtime build: %w", err)
	}
	defer func() {
		_ = os.Remove(settingsPath)
	}()
	args = append(args,
		"-v", fmt.Sprintf("%s:/src:ro", runtimeDir),
		"-v", fmt.Sprintf("%s:/out", runtimeDir),
		"-v", fmt.Sprintf("%s:%s:ro", settingsPath, containerM2SettingsPath),
		"-v", fmt.Sprintf("%s:%s", m2RepoCacheDir, containerM2RepoPath),
	)
	args = append(args, "-e", "MAVEN_CONFIG=/tmp/m2", "-e", "HOME=/tmp")
	args = appendJavaBuildEnvArgs(args)
	script := strings.Join([]string{
		"set -euo pipefail",
		"mkdir -p /tmp/work /out/extensions/wrapper /out/extensions/agent",
		"cp -a /src/. /tmp/work",
		"cd /tmp/work/build",
		javaRuntimeMavenBuildLine(),
		"cp ../extensions/wrapper/target/lambda-java-wrapper.jar /out/extensions/wrapper/lambda-java-wrapper.jar",
		"cp ../extensions/agent/target/lambda-java-agent.jar /out/extensions/agent/lambda-java-agent.jar",
	}, "\n")
	args = append(args,
		javaRuntimeBuildImage,
		"bash", "-lc", script,
	)

	cmd := exec.Command("docker", args...)
	if ctx.Verbose {
		cmd.Stdout = resolveGenerateOutput(ctx.Out)
		cmd.Stderr = resolveGenerateErrOutput(ctx.Out)
		return cmd.Run()
	}

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("docker not found; install docker to build the Java runtime jars")
		}
		return fmt.Errorf("java runtime build failed: %w\n%s", err, output.String())
	}
	return nil
}
