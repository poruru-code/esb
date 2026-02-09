// Where: cli/internal/infra/build/stage.go
// What: File staging helpers for generator output.
// Why: Keep GenerateFiles readable and testable.
package build

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/poruru/edge-serverless-box/cli/internal/domain/manifest"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/runtime"
	"github.com/poruru/edge-serverless-box/cli/internal/domain/template"
)

type stageContext struct {
	BaseDir           string
	OutputDir         string
	FunctionsDir      string
	ProjectRoot       string
	SitecustomizePath string
	LayerCacheDir     string
	DryRun            bool
	Verbose           bool
}

type stagedFunction struct {
	Function         template.FunctionSpec
	FunctionDir      string
	SitecustomizeRef string
}

const hostM2SettingsPath = "/tmp/host-m2-settings.xml"

// stageFunction prepares the function source, layers, and sitecustomize file
// under the output directory so downstream steps can render Dockerfiles.
func stageFunction(fn template.FunctionSpec, ctx stageContext) (stagedFunction, error) {
	if fn.Name == "" {
		return stagedFunction{}, fmt.Errorf("function name is required")
	}

	profile, err := runtime.Resolve(fn.Runtime)
	if err != nil {
		return stagedFunction{}, err
	}

	functionDir := filepath.Join(ctx.FunctionsDir, fn.Name)
	if !ctx.DryRun {
		if err := ensureDir(functionDir); err != nil {
			return stagedFunction{}, err
		}
	}

	sourcePath := resolveResourcePath(ctx.BaseDir, fn.CodeURI)
	stagingSrc := filepath.Join(functionDir, "src")
	if !ctx.DryRun {
		switch {
		case dirExists(sourcePath):
			if err := copyDir(sourcePath, stagingSrc); err != nil {
				return stagedFunction{}, err
			}
		case fileExists(sourcePath):
			if err := ensureDir(stagingSrc); err != nil {
				return stagedFunction{}, err
			}
			targetDir := stagingSrc
			if subDir := profile.CodeUriTargetDir(sourcePath); subDir != "" {
				targetDir = filepath.Join(stagingSrc, subDir)
				if err := ensureDir(targetDir); err != nil {
					return stagedFunction{}, err
				}
			}
			target := filepath.Join(targetDir, filepath.Base(sourcePath))
			if err := copyFile(sourcePath, target); err != nil {
				return stagedFunction{}, err
			}
		}
	}

	fn.CodeURI = ensureSlash(path.Join("functions", fn.Name, "src"))
	fn.HasRequirements = fileExists(filepath.Join(stagingSrc, "requirements.txt"))

	stagedLayers, err := stageLayers(fn.Layers, ctx, fn.Name, functionDir, profile)
	if err != nil {
		return stagedFunction{}, err
	}
	fn.Layers = stagedLayers

	siteRef := path.Join("functions", fn.Name, "sitecustomize.py")
	siteRef = filepath.ToSlash(siteRef)
	if !ctx.DryRun {
		if profile.UsesSitecustomize {
			siteSrc := resolveSitecustomizeSource(ctx)
			if siteSrc != "" {
				if info, err := os.Stat(siteSrc); err == nil {
					if err := linkOrCopyFile(siteSrc, filepath.Join(functionDir, "sitecustomize.py"), info.Mode()); err != nil {
						return stagedFunction{}, err
					}
				}
			}
		} else {
			sitePath := filepath.Join(functionDir, "sitecustomize.py")
			if fileExists(sitePath) {
				if err := os.Remove(sitePath); err != nil {
					return stagedFunction{}, err
				}
			}
		}
	}

	if !ctx.DryRun && profile.UsesJavaWrapper {
		wrapperSrc, err := ensureJavaWrapperSource(ctx)
		if err != nil {
			return stagedFunction{}, err
		}
		if info, err := os.Stat(wrapperSrc); err == nil {
			target := filepath.Join(functionDir, javaWrapperFileName)
			if err := linkOrCopyFile(wrapperSrc, target, info.Mode()); err != nil {
				return stagedFunction{}, err
			}
		} else {
			return stagedFunction{}, err
		}
	}
	if !ctx.DryRun && profile.UsesJavaAgent {
		agentSrc, err := ensureJavaAgentSource(ctx)
		if err != nil {
			return stagedFunction{}, err
		}
		if info, err := os.Stat(agentSrc); err == nil {
			target := filepath.Join(functionDir, javaAgentFileName)
			if err := linkOrCopyFile(agentSrc, target, info.Mode()); err != nil {
				return stagedFunction{}, err
			}
		} else {
			return stagedFunction{}, err
		}
	}

	return stagedFunction{
		Function:         fn,
		FunctionDir:      functionDir,
		SitecustomizeRef: siteRef,
	}, nil
}

// stageLayers stages each referenced layer inside the function directory,
// applying smart nesting for Python runtimes and sanitizing names.
func stageLayers(layers []manifest.LayerSpec, ctx stageContext, functionName, functionDir string, profile runtime.Profile) ([]manifest.LayerSpec, error) {
	if len(layers) == 0 {
		return nil, nil
	}

	staged := make([]manifest.LayerSpec, 0, len(layers))
	layersDir := filepath.Join(functionDir, "layers")
	for _, layer := range layers {
		source := resolveResourcePath(ctx.BaseDir, layer.ContentURI)
		if !fileOrDirExists(source) {
			continue
		}

		targetName := layerTargetName(layer, source)
		if targetName == "" {
			continue
		}

		if ctx.Verbose {
			fmt.Printf("  Staging layer: %s -> %s\n", layer.Name, targetName)
		}

		layerRef := filepath.ToSlash(filepath.Join("functions", functionName, "layers", targetName))
		if !ctx.DryRun {
			targetDir := filepath.Join(layersDir, targetName)
			if err := removeDir(targetDir); err != nil {
				return nil, err
			}

			var finalSrc string
			switch {
			case fileExists(source) && strings.HasSuffix(strings.ToLower(source), ".zip"):
				extracted, err := extractZipLayer(source, ctx.LayerCacheDir)
				if err != nil {
					return nil, err
				}
				finalSrc = extracted
			case dirExists(source):
				finalSrc = source
			default:
				continue
			}

			finalDest := targetDir
			if shouldNestPython(profile.NestPythonLayers, finalSrc) {
				finalDest = filepath.Join(targetDir, "python")
			}

			if err := copyDirLinkOrCopy(finalSrc, finalDest); err != nil {
				return nil, err
			}
		}

		layer.ContentURI = layerRef
		staged = append(staged, layer)
	}

	return staged, nil
}

func resolveResourcePath(baseDir, raw string) string {
	trimmed := strings.TrimLeft(raw, "/\\")
	if trimmed == "" {
		trimmed = raw
	}
	return filepath.Clean(filepath.Join(baseDir, trimmed))
}

func resolveSitecustomizeSource(ctx stageContext) string {
	source := ctx.SitecustomizePath
	if strings.TrimSpace(source) == "" {
		source = template.DefaultSitecustomizeSource
	}

	if filepath.IsAbs(source) {
		if fileExists(source) {
			return source
		}
		return ""
	}

	candidate := filepath.Clean(filepath.Join(ctx.BaseDir, source))
	if fileExists(candidate) {
		return candidate
	}

	candidate = filepath.Clean(filepath.Join(ctx.ProjectRoot, source))
	if fileExists(candidate) {
		return candidate
	}
	return ""
}

const (
	javaWrapperFileName = "lambda-java-wrapper.jar"
	javaAgentFileName   = "lambda-java-agent.jar"
)

func ensureJavaWrapperSource(ctx stageContext) (string, error) {
	if src := resolveJavaWrapperSource(ctx); src != "" {
		return src, nil
	}
	if err := buildJavaRuntimeJars(ctx); err != nil {
		return "", err
	}
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
		filepath.Join(runtimeDir, "extensions", "wrapper", "target", javaWrapperFileName),
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
	if err := buildJavaRuntimeJars(ctx); err != nil {
		return "", err
	}
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
		filepath.Join(runtimeDir, "extensions", "agent", "target", javaAgentFileName),
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

func isWritableDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	if os.Geteuid() == 0 {
		return true
	}
	mode := info.Mode().Perm()
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int(stat.Uid) == os.Geteuid() {
			return mode&0o200 != 0
		}
		if int(stat.Gid) == os.Getegid() {
			return mode&0o020 != 0
		}
	}
	return mode&0o002 != 0
}

func isReadableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = file.Close()
	return true
}

func appendM2MountArgs(args []string, homeDir string) []string {
	if homeDir == "" {
		return args
	}
	m2Dir := filepath.Join(homeDir, ".m2")
	if isWritableDir(m2Dir) {
		return append(args, "-v", fmt.Sprintf("%s:/tmp/m2", m2Dir))
	}
	settingsPath := filepath.Join(m2Dir, "settings.xml")
	if isReadableFile(settingsPath) {
		return append(
			args,
			"-v",
			fmt.Sprintf("%s:%s:ro", settingsPath, hostM2SettingsPath),
		)
	}
	return args
}

func firstConfiguredEnv(keys ...string) (string, bool) {
	for _, key := range keys {
		value, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		return trimmed, true
	}
	return "", false
}

func appendJavaBuildEnvArgs(args []string) []string {
	pairs := []struct {
		upper string
		lower string
	}{
		{upper: "HTTP_PROXY", lower: "http_proxy"},
		{upper: "HTTPS_PROXY", lower: "https_proxy"},
		{upper: "NO_PROXY", lower: "no_proxy"},
	}
	for _, pair := range pairs {
		value, ok := firstConfiguredEnv(pair.upper, pair.lower)
		if !ok {
			continue
		}
		args = append(args, "-e", pair.upper+"="+value, "-e", pair.lower+"="+value)
	}
	for _, key := range []string{"MAVEN_OPTS", "JAVA_TOOL_OPTIONS"} {
		value, ok := firstConfiguredEnv(key)
		if !ok {
			continue
		}
		args = append(args, "-e", key+"="+value)
	}
	return args
}

func buildJavaRuntimeJars(ctx stageContext) error {
	runtimeDir, err := resolveJavaRuntimeDir(ctx)
	if err != nil {
		return err
	}
	if ctx.Verbose {
		fmt.Printf("  Building Java runtime jars in %s\n", runtimeDir)
	}

	buildDir := filepath.Join(runtimeDir, "build")
	if !dirExists(buildDir) {
		return fmt.Errorf("java runtime build directory not found: %s", buildDir)
	}

	homeDir, _ := os.UserHomeDir()
	args := []string{
		"run",
		"--rm",
	}
	if uid, gid := os.Getuid(), os.Getgid(); uid >= 0 && gid >= 0 {
		args = append(args, "--user", fmt.Sprintf("%d:%d", uid, gid))
	}
	args = append(args,
		"-v", fmt.Sprintf("%s:/src:ro", runtimeDir),
		"-v", fmt.Sprintf("%s:/out", runtimeDir),
	)
	args = appendM2MountArgs(args, homeDir)
	args = append(args, "-e", "MAVEN_CONFIG=/tmp/m2", "-e", "HOME=/tmp")
	args = appendJavaBuildEnvArgs(args)
	script := strings.Join([]string{
		"set -euo pipefail",
		"mkdir -p /tmp/work /tmp/m2 /out/extensions/wrapper /out/extensions/agent",
		fmt.Sprintf(
			"if [ -f %s ]; then cp %s /tmp/m2/settings.xml; fi",
			hostM2SettingsPath,
			hostM2SettingsPath,
		),
		"cp -a /src/. /tmp/work",
		"cd /tmp/work/build",
		"mvn -q -DskipTests -pl ../extensions/wrapper,../extensions/agent -am package",
		"cp ../extensions/wrapper/target/lambda-java-wrapper.jar /out/extensions/wrapper/lambda-java-wrapper.jar",
		"cp ../extensions/agent/target/lambda-java-agent.jar /out/extensions/agent/lambda-java-agent.jar",
	}, "\n")
	args = append(args,
		"maven:3.9.6-eclipse-temurin-21",
		"bash", "-lc", script,
	)

	cmd := exec.Command("docker", args...)
	if ctx.Verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
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

// layerTargetName derives a filesystem-safe directory name for a layer.
func layerTargetName(layer manifest.LayerSpec, source string) string {
	if sanitized := sanitizeLayerName(layer.Name); sanitized != "" {
		return sanitized
	}
	base := filepath.Base(source)
	if strings.HasSuffix(strings.ToLower(base), ".zip") {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if sanitized := sanitizeLayerName(base); sanitized != "" {
		return sanitized
	}
	return "layer"
}

// sanitizeLayerName keeps only alphanumeric, dot, underscore, and dash characters.
func sanitizeLayerName(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// shouldNestPython returns true when a Python layer lacks an explicit python/
// layout and therefore must be nested to satisfy the runtime expectation.
func shouldNestPython(nest bool, sourceDir string) bool {
	if !nest {
		return false
	}
	if sourceDir == "" {
		return false
	}
	return !containsPythonLayout(sourceDir)
}

// containsPythonLayout checks for python/ or site-packages/ at the root level.
func containsPythonLayout(dir string) bool {
	return dirExists(filepath.Join(dir, "python")) || dirExists(filepath.Join(dir, "site-packages"))
}

func ensureSlash(value string) string {
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}
