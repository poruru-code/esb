// Where: cli/internal/infra/templategen/stage_java_runtime.go
// What: Java runtime artifact staging and build helpers.
// Why: Isolate Java-specific runtime preparation from generic staging logic.
package templategen

import (
	"fmt"
	"path/filepath"
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
