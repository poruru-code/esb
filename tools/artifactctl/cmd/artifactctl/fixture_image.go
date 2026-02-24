package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/poruru-code/esb/pkg/artifactcore"
	proxybuildargs "github.com/poruru-code/esb/pkg/proxy/buildargs"
)

const (
	fixtureImageEnsureSchemaVersion = 1
	defaultFixtureImageRoot         = "e2e/fixtures/images/lambda"
	javaFixtureName                 = "esb-e2e-image-java"
	javaFixtureMavenBaseImage       = "public.ecr.aws/sam/build-java21@sha256:5f78d6d9124e54e5a7a9941ef179d74d88b7a5b117526ea8574137e5403b51b7"
)

var (
	localFixtureSubdirs = map[string]string{
		"esb-e2e-image-python": "python",
		"esb-e2e-image-java":   "java",
	}
	fixtureDockerfileFromPattern = regexp.MustCompile(`(?i)^FROM(?:\s+--platform=[^\s]+)?\s+([^\s]+)`) //nolint:lll
	javaFixtureMavenArgPattern   = regexp.MustCompile(`(?im)^\s*ARG\s+MAVEN_IMAGE(?:\s*=.*)?\s*$`)
	javaFixtureFromPattern       = regexp.MustCompile(`(?im)^\s*FROM\s+\$\{?MAVEN_IMAGE\}?\s+AS\s+builder\s*$`)
)

type fixtureCommandRunner interface {
	Run(cmd []string) error
}

type FixtureImageEnsureInput struct {
	ArtifactPath    string
	NoCache         bool
	FixtureRoot     string
	Runner          fixtureCommandRunner
	EnsureMavenShim func(MavenShimEnsureInput) (MavenShimEnsureResult, error)
	LogWriter       io.Writer
}

type FixtureImageEnsureResult struct {
	SchemaVersion  int      `json:"schema_version"`
	PreparedImages []string `json:"prepared_images"`
}

func executeFixtureImageEnsure(input FixtureImageEnsureInput) (FixtureImageEnsureResult, error) {
	return executeFixtureImageEnsureWithLogWriter(input, os.Stderr)
}

func executeFixtureImageEnsureWithLogWriter(
	input FixtureImageEnsureInput,
	logWriter io.Writer,
) (FixtureImageEnsureResult, error) {
	manifestPath := strings.TrimSpace(input.ArtifactPath)
	if manifestPath == "" {
		return FixtureImageEnsureResult{}, fmt.Errorf("artifact manifest path is empty")
	}
	manifest, err := artifactcore.ReadArtifactManifest(manifestPath)
	if err != nil {
		return FixtureImageEnsureResult{}, fmt.Errorf("read artifact manifest: %w", err)
	}
	sources, err := collectLocalFixtureImageSources(manifest, manifestPath)
	if err != nil {
		return FixtureImageEnsureResult{}, err
	}
	if len(sources) == 0 {
		return FixtureImageEnsureResult{
			SchemaVersion:  fixtureImageEnsureSchemaVersion,
			PreparedImages: []string{},
		}, nil
	}

	runner := input.Runner
	if runner == nil {
		writer := logWriter
		if input.LogWriter != nil {
			writer = input.LogWriter
		}
		runner = stderrCommandRunner{writer: writer}
	}
	ensureMavenShim := input.EnsureMavenShim
	if ensureMavenShim == nil {
		writer := logWriter
		if input.LogWriter != nil {
			writer = input.LogWriter
		}
		ensureMavenShim = func(in MavenShimEnsureInput) (MavenShimEnsureResult, error) {
			return executeMavenShimEnsureWithLogWriter(in, writer)
		}
	}

	fixtureRoot := strings.TrimSpace(input.FixtureRoot)
	if fixtureRoot == "" {
		fixtureRoot = defaultFixtureImageRoot
	}
	fixtureRootAbs, err := filepath.Abs(fixtureRoot)
	if err != nil {
		return FixtureImageEnsureResult{}, fmt.Errorf("resolve fixture root: %w", err)
	}

	preparedImages := make([]string, 0, len(sources))
	resolvedShimByRegistry := map[string]string{}
	for _, source := range sources {
		fixtureName := fixtureRepoName(source)
		subdir, ok := localFixtureSubdirs[fixtureName]
		if !ok {
			return FixtureImageEnsureResult{}, fmt.Errorf("unknown local fixture image source: %s", source)
		}
		fixtureDir := filepath.Join(fixtureRootAbs, subdir)
		if info, statErr := os.Stat(fixtureDir); statErr != nil || !info.IsDir() {
			if statErr != nil {
				return FixtureImageEnsureResult{}, fmt.Errorf("local fixture image source not found: %s", fixtureDir)
			}
			return FixtureImageEnsureResult{}, fmt.Errorf("local fixture image source is not a directory: %s", fixtureDir)
		}

		buildArgs := map[string]string{}
		if fixtureName == javaFixtureName {
			if err := assertJavaFixtureUsesMavenShimContract(filepath.Join(fixtureDir, "Dockerfile")); err != nil {
				return FixtureImageEnsureResult{}, err
			}
			registryHost := imageRegistryHost(source)
			cacheKey := javaFixtureMavenBaseImage + "|" + registryHost
			shimImage, ok := resolvedShimByRegistry[cacheKey]
			if !ok {
				shimResult, shimErr := ensureMavenShim(MavenShimEnsureInput{
					BaseImage:    javaFixtureMavenBaseImage,
					HostRegistry: registryHost,
					NoCache:      input.NoCache,
				})
				if shimErr != nil {
					return FixtureImageEnsureResult{}, fmt.Errorf("maven shim ensure failed: %w", shimErr)
				}
				shimImage = strings.TrimSpace(shimResult.ShimImage)
				if shimImage == "" {
					return FixtureImageEnsureResult{}, fmt.Errorf("maven shim ensure returned empty shim image")
				}
				resolvedShimByRegistry[cacheKey] = shimImage
			}
			buildArgs["MAVEN_IMAGE"] = shimImage
		}

		buildCmd := buildxBuildCommandForFixture(source, fixtureDir, input.NoCache, buildArgs)
		if runErr := runner.Run(buildCmd); runErr != nil {
			return FixtureImageEnsureResult{}, fmt.Errorf("build local fixture image %s: %w", source, runErr)
		}
		if runErr := runner.Run([]string{"docker", "push", source}); runErr != nil {
			return FixtureImageEnsureResult{}, fmt.Errorf("push local fixture image %s: %w", source, runErr)
		}
		preparedImages = append(preparedImages, source)
	}

	return FixtureImageEnsureResult{
		SchemaVersion:  fixtureImageEnsureSchemaVersion,
		PreparedImages: preparedImages,
	}, nil
}

func buildxBuildCommandForFixture(
	tag string,
	contextDir string,
	noCache bool,
	buildArgs map[string]string,
) []string {
	cmd := []string{
		"docker",
		"buildx",
		"build",
		"--platform",
		"linux/amd64",
		"--load",
	}
	if noCache {
		cmd = append(cmd, "--no-cache")
	}
	cmd = proxybuildargs.AppendDockerBuildArgsFromOS(cmd)
	if len(buildArgs) > 0 {
		keys := make([]string, 0, len(buildArgs))
		for key := range buildArgs {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := strings.TrimSpace(buildArgs[key])
			if value == "" {
				continue
			}
			cmd = append(cmd, "--build-arg", key+"="+value)
		}
	}
	cmd = append(cmd, "--tag", tag, contextDir)
	return cmd
}

func collectLocalFixtureImageSources(
	manifest artifactcore.ArtifactManifest,
	manifestPath string,
) ([]string, error) {
	sources := make(map[string]struct{})
	for i := range manifest.Artifacts {
		artifactRoot, err := manifest.ResolveArtifactRoot(manifestPath, i)
		if err != nil {
			return nil, err
		}
		walkErr := filepath.WalkDir(artifactRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if d.Name() != "Dockerfile" {
				return nil
			}
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, line := range strings.Split(string(content), "\n") {
				trimmed := strings.TrimSpace(line)
				match := fixtureDockerfileFromPattern.FindStringSubmatch(trimmed)
				if match == nil {
					continue
				}
				source := strings.TrimSpace(match[1])
				if isLocalFixtureImageSource(source) {
					sources[source] = struct{}{}
				}
			}
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("scan artifact root %s: %w", artifactRoot, walkErr)
		}
	}
	result := make([]string, 0, len(sources))
	for source := range sources {
		result = append(result, source)
	}
	sort.Strings(result)
	return result, nil
}

func isLocalFixtureImageSource(source string) bool {
	if strings.TrimSpace(source) == "" {
		return false
	}
	_, ok := localFixtureSubdirs[fixtureRepoName(source)]
	return ok
}

func fixtureRepoName(source string) string {
	withoutDigest := strings.SplitN(source, "@", 2)[0]
	lastSegment := withoutDigest[strings.LastIndex(withoutDigest, "/")+1:]
	return strings.SplitN(lastSegment, ":", 2)[0]
}

func imageRegistryHost(imageRef string) string {
	withoutDigest := strings.SplitN(strings.TrimSpace(imageRef), "@", 2)[0]
	if !strings.Contains(withoutDigest, "/") {
		return ""
	}
	candidate := strings.SplitN(withoutDigest, "/", 2)[0]
	if candidate == "" {
		return ""
	}
	if candidate == "localhost" || strings.Contains(candidate, ".") || strings.Contains(candidate, ":") {
		return candidate
	}
	return ""
}

func assertJavaFixtureUsesMavenShimContract(dockerfile string) error {
	content, err := os.ReadFile(dockerfile)
	if err != nil {
		return fmt.Errorf("java fixture Dockerfile not found: %s", dockerfile)
	}
	text := string(content)
	if javaFixtureMavenArgPattern.FindStringIndex(text) == nil {
		return fmt.Errorf(
			"java fixture Dockerfile must define `ARG MAVEN_IMAGE` so E2E can inject maven-shim: %s",
			dockerfile,
		)
	}
	if javaFixtureFromPattern.FindStringIndex(text) == nil {
		return fmt.Errorf(
			"java fixture Dockerfile must use `FROM ${MAVEN_IMAGE} AS builder` for proxy-safe Maven resolution: %s",
			dockerfile,
		)
	}
	return nil
}
