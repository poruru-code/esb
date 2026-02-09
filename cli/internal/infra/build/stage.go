// Where: cli/internal/infra/build/stage.go
// What: File staging helpers for generator output.
// Why: Keep GenerateFiles readable and testable.
package build

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
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

const (
	containerM2SettingsPath = "/tmp/m2/settings.xml"
	javaRuntimeBuildImage   = "public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7"
)

type mavenProxyEndpoint struct {
	Host     string
	Port     int
	Username string
	Password string
}

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
	for _, key := range []string{
		"HTTP_PROXY",
		"http_proxy",
		"HTTPS_PROXY",
		"https_proxy",
		"NO_PROXY",
		"no_proxy",
		"MAVEN_OPTS",
		"JAVA_TOOL_OPTIONS",
	} {
		args = append(args, "-e", key+"=")
	}
	return args
}

func validateMavenProxyURL(envLabel string, rawURL string) error {
	value := strings.TrimSpace(rawURL)
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", envLabel, err)
	}
	if parsed.Scheme == "" || parsed.Hostname() == "" {
		return fmt.Errorf("%s must include scheme and host: %s", envLabel, rawURL)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%s must use http or https scheme: %s", envLabel, rawURL)
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf(
			"%s must not include path/query/fragment: %s",
			envLabel,
			rawURL,
		)
	}
	if portText := parsed.Port(); portText != "" {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("%s has invalid port: %s", envLabel, rawURL)
		}
	}
	return nil
}

func parseMavenProxyEndpoint(envLabel string, rawURL string) (mavenProxyEndpoint, error) {
	if err := validateMavenProxyURL(envLabel, rawURL); err != nil {
		return mavenProxyEndpoint{}, err
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return mavenProxyEndpoint{}, fmt.Errorf("%s is invalid: %w", envLabel, err)
	}
	port := 0
	if parsed.Port() != "" {
		port, err = strconv.Atoi(parsed.Port())
		if err != nil || port < 1 || port > 65535 {
			return mavenProxyEndpoint{}, fmt.Errorf("%s has invalid port: %s", envLabel, rawURL)
		}
	} else {
		if strings.EqualFold(parsed.Scheme, "https") {
			port = 443
		} else {
			port = 80
		}
	}

	username := ""
	password := ""
	if parsed.User != nil {
		username = parsed.User.Username()
		if decoded, decodeErr := url.PathUnescape(username); decodeErr == nil {
			username = decoded
		}
		if rawPassword, ok := parsed.User.Password(); ok {
			password = rawPassword
			if decoded, decodeErr := url.PathUnescape(password); decodeErr == nil {
				password = decoded
			}
		}
	}

	return mavenProxyEndpoint{
		Host:     parsed.Hostname(),
		Port:     port,
		Username: username,
		Password: password,
	}, nil
}

func normalizeNoProxyToken(token string) string {
	normalized := strings.TrimSpace(token)
	if normalized == "" {
		return ""
	}

	if strings.HasPrefix(normalized, "[") {
		if closingIndex := strings.Index(normalized, "]"); closingIndex > 1 {
			normalized = strings.TrimSpace(normalized[1:closingIndex])
		}
	} else if strings.Count(normalized, ":") == 1 {
		host, port, ok := strings.Cut(normalized, ":")
		if ok {
			if _, err := strconv.Atoi(port); err == nil {
				normalized = strings.TrimSpace(host)
			}
		}
	}

	if strings.HasPrefix(normalized, ".") && !strings.HasPrefix(normalized, "*.") {
		normalized = "*" + normalized
	}
	return normalized
}

func buildMavenNonProxyHosts(raw string) string {
	seen := make(map[string]struct{})
	values := make([]string, 0)
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	}) {
		normalized := normalizeNoProxyToken(token)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		values = append(values, normalized)
	}
	return strings.Join(values, "|")
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

func renderMavenProxyBlock(
	proxyID string,
	protocol string,
	endpoint mavenProxyEndpoint,
	nonProxyHosts string,
) []string {
	lines := []string{
		"    <proxy>",
		fmt.Sprintf("      <id>%s</id>", xmlEscape(proxyID)),
		"      <active>true</active>",
		fmt.Sprintf("      <protocol>%s</protocol>", xmlEscape(protocol)),
		fmt.Sprintf("      <host>%s</host>", xmlEscape(endpoint.Host)),
		fmt.Sprintf("      <port>%d</port>", endpoint.Port),
	}
	if endpoint.Username != "" {
		lines = append(lines, fmt.Sprintf("      <username>%s</username>", xmlEscape(endpoint.Username)))
	}
	if endpoint.Password != "" {
		lines = append(lines, fmt.Sprintf("      <password>%s</password>", xmlEscape(endpoint.Password)))
	}
	if nonProxyHosts != "" {
		lines = append(
			lines,
			fmt.Sprintf("      <nonProxyHosts>%s</nonProxyHosts>", xmlEscape(nonProxyHosts)),
		)
	}
	lines = append(lines, "    </proxy>")
	return lines
}

func renderMavenSettingsXML() (string, error) {
	httpRaw, hasHTTP := firstConfiguredEnv("HTTP_PROXY", "http_proxy")
	httpsRaw, hasHTTPS := firstConfiguredEnv("HTTPS_PROXY", "https_proxy")

	var (
		httpEndpoint  *mavenProxyEndpoint
		httpsEndpoint *mavenProxyEndpoint
	)
	if hasHTTP {
		parsed, err := parseMavenProxyEndpoint("HTTP_PROXY/http_proxy", httpRaw)
		if err != nil {
			return "", err
		}
		httpEndpoint = &parsed
	}
	if hasHTTPS {
		parsed, err := parseMavenProxyEndpoint("HTTPS_PROXY/https_proxy", httpsRaw)
		if err != nil {
			return "", err
		}
		httpsEndpoint = &parsed
	}
	if httpsEndpoint == nil && httpEndpoint != nil {
		fallback := *httpEndpoint
		httpsEndpoint = &fallback
	}

	nonProxyHosts := ""
	if rawNoProxy, ok := firstConfiguredEnv("NO_PROXY", "no_proxy"); ok {
		nonProxyHosts = buildMavenNonProxyHosts(rawNoProxy)
	}

	lines := []string{
		"<settings>",
		"  <proxies>",
	}
	if httpEndpoint != nil {
		lines = append(lines, renderMavenProxyBlock("http-proxy", "http", *httpEndpoint, nonProxyHosts)...)
	}
	if httpsEndpoint != nil {
		lines = append(
			lines,
			renderMavenProxyBlock("https-proxy", "https", *httpsEndpoint, nonProxyHosts)...,
		)
	}
	lines = append(
		lines,
		"  </proxies>",
		"</settings>",
	)
	return strings.Join(lines, "\n") + "\n", nil
}

func writeTempMavenSettingsFile() (string, error) {
	settings, err := renderMavenSettingsXML()
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "esb-m2-settings-*.xml")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if _, err := file.WriteString(settings); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func javaRuntimeMavenBuildLine() string {
	return fmt.Sprintf(
		"mvn -s %s -q -Dmaven.artifact.threads=1 -DskipTests "+
			"-pl ../extensions/wrapper,../extensions/agent -am package",
		containerM2SettingsPath,
	)
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
