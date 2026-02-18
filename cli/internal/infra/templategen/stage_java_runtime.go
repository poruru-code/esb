// Where: cli/internal/infra/templategen/stage_java_runtime.go
// What: Java runtime artifact staging and build helpers.
// Why: Isolate Java-specific runtime preparation from generic staging logic.
package templategen

import (
	"fmt"
	"path/filepath"
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
	return "", fmt.Errorf("java wrapper jar not found in runtime-hooks/java/wrapper")
}

func resolveJavaWrapperSource(ctx stageContext) string {
	hooksDir, err := resolveJavaHooksDir(ctx)
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(hooksDir, "wrapper", javaWrapperFileName),
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
	return "", fmt.Errorf("java agent jar not found in runtime-hooks/java/agent")
}

func resolveJavaAgentSource(ctx stageContext) string {
	hooksDir, err := resolveJavaHooksDir(ctx)
	if err != nil {
		return ""
	}
	candidates := []string{
		filepath.Join(hooksDir, "agent", javaAgentFileName),
	}
	for _, candidate := range candidates {
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

func resolveJavaHooksDir(ctx stageContext) (string, error) {
	rel := filepath.Join("runtime-hooks", "java")
	candidates := []string{
		filepath.Clean(filepath.Join(ctx.ProjectRoot, rel)),
		filepath.Clean(filepath.Join(ctx.BaseDir, rel)),
	}
	for _, candidate := range candidates {
		if dirExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("java runtime hooks directory not found")
}

func javaRuntimeMavenBuildLine() string {
	return fmt.Sprintf(
		"mvn -s %s -q -Dmaven.repo.local=%s -Dmaven.artifact.threads=1 -DskipTests "+
			"-pl ../wrapper,../agent -am package",
		containerM2SettingsPath,
		containerM2RepoPath,
	)
}
