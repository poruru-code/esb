package deployops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/poruru-code/esb/pkg/artifactcore"
)

type prepareImagesInput struct {
	ArtifactPath string
	NoCache      bool
	Runner       CommandRunner
	Runtime      *RuntimeObservation
	EnsureBase   bool
}

type CommandRunner interface {
	Run(cmd []string) error
}

type defaultCommandRunner struct{}

func (defaultCommandRunner) Run(cmd []string) error {
	if len(cmd) == 0 {
		return fmt.Errorf("command is empty")
	}
	command := exec.Command(cmd[0], cmd[1:]...)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

var defaultCommandRunnerFactory = func() CommandRunner {
	return defaultCommandRunner{}
}

var dockerImageExistsFunc = dockerImageExists

type imageBuildTarget struct {
	functionName string
	imageRef     string
	dockerfile   string
}

type prepareImagesResult struct {
	publishedFunctionImages map[string]struct{}
}

func prepareImages(req prepareImagesInput) error {
	_, err := prepareImagesWithResult(req)
	return err
}

func prepareImagesWithResult(req prepareImagesInput) (prepareImagesResult, error) {
	manifestPath := strings.TrimSpace(req.ArtifactPath)
	if manifestPath == "" {
		return prepareImagesResult{}, artifactcore.ErrArtifactPathRequired
	}
	runtime := normalizeRuntimeObservation(req.Runtime)
	manifest, err := artifactcore.ReadArtifactManifest(manifestPath)
	if err != nil {
		return prepareImagesResult{}, err
	}
	runner := req.Runner
	if runner == nil {
		runner = defaultCommandRunnerFactory()
	}
	ensureBase := req.EnsureBase
	builtFunctionImages := make(map[string]struct{})
	builtBaseImages := make(map[string]struct{})
	publishedFunctionImages := make(map[string]struct{})
	resolvedMavenShimImages := make(map[string]string)
	hasFunctionBuildTargets := false

	for i := range manifest.Artifacts {
		artifactRoot, err := manifest.ResolveArtifactRoot(manifestPath, i)
		if err != nil {
			return prepareImagesResult{}, err
		}
		runtimeConfigDir, err := manifest.ResolveRuntimeConfigDir(manifestPath, i)
		if err != nil {
			return prepareImagesResult{}, err
		}
		functionsPath := filepath.Join(runtimeConfigDir, "functions.yml")
		functionsPayload, ok, err := loadYAML(functionsPath)
		if err != nil {
			return prepareImagesResult{}, fmt.Errorf("load functions config: %w", err)
		}
		if !ok {
			return prepareImagesResult{}, fmt.Errorf("functions config not found: %s", functionsPath)
		}
		functionsRaw, ok := functionsPayload["functions"].(map[string]any)
		if !ok {
			return prepareImagesResult{}, fmt.Errorf("functions must be map in %s", functionsPath)
		}

		buildTargets := collectImageBuildTargets(artifactRoot, functionsRaw, builtFunctionImages)
		if len(buildTargets) == 0 {
			continue
		}
		hasFunctionBuildTargets = true

		functionNames := make([]string, 0, len(buildTargets))
		for _, target := range buildTargets {
			functionNames = append(functionNames, target.functionName)
		}

		if err := withFunctionBuildWorkspace(artifactRoot, functionNames, func(contextRoot string) error {
			for _, target := range buildTargets {
				if err := buildAndPushFunctionImage(
					target.imageRef,
					target.functionName,
					contextRoot,
					req.NoCache,
					runtime,
					runner,
					ensureBase,
					builtBaseImages,
					resolvedMavenShimImages,
				); err != nil {
					return err
				}
				builtFunctionImages[target.imageRef] = struct{}{}
				publishedFunctionImages[target.imageRef] = struct{}{}
			}
			return nil
		}); err != nil {
			return prepareImagesResult{}, err
		}
	}
	if ensureBase && !hasFunctionBuildTargets {
		defaultBaseRef, err := resolveDefaultLambdaBaseRef(runtime)
		if err != nil {
			return prepareImagesResult{}, err
		}
		if err := ensureLambdaBaseImage(defaultBaseRef, req.NoCache, runner, builtBaseImages); err != nil {
			return prepareImagesResult{}, err
		}
	}
	return prepareImagesResult{publishedFunctionImages: publishedFunctionImages}, nil
}

func collectImageBuildTargets(
	artifactRoot string,
	functionsRaw map[string]any,
	builtFunctionImages map[string]struct{},
) []imageBuildTarget {
	names := make([]string, 0, len(functionsRaw))
	for name := range functionsRaw {
		names = append(names, name)
	}
	sort.Strings(names)
	targets := make([]imageBuildTarget, 0, len(names))
	for _, functionName := range names {
		payload, ok := functionsRaw[functionName].(map[string]any)
		if !ok {
			continue
		}
		rawImageRef, ok := payload["image"]
		if !ok {
			continue
		}
		imageRef := strings.TrimSpace(fmt.Sprintf("%v", rawImageRef))
		if imageRef == "" {
			continue
		}
		normalizedImageRef, _ := normalizeFunctionImageRefForRuntime(imageRef)
		if normalizedImageRef == "" {
			continue
		}
		if _, ok := builtFunctionImages[normalizedImageRef]; ok {
			continue
		}
		dockerfile := filepath.Join(artifactRoot, "functions", functionName, "Dockerfile")
		if _, err := os.Stat(dockerfile); err != nil {
			continue
		}
		targets = append(targets, imageBuildTarget{
			functionName: functionName,
			imageRef:     normalizedImageRef,
			dockerfile:   dockerfile,
		})
	}
	return targets
}

func buildAndPushFunctionImage(
	imageRef, functionName, contextRoot string,
	noCache bool,
	runtime *RuntimeObservation,
	runner CommandRunner,
	ensureBase bool,
	builtBaseImages map[string]struct{},
	resolvedMavenShimImages map[string]string,
) error {
	dockerfile := filepath.Join(contextRoot, "functions", functionName, "Dockerfile")
	pushRef := resolvePushReference(imageRef)
	resolvedDockerfile, cleanup, err := resolveFunctionBuildDockerfile(
		dockerfile,
		runtime,
		noCache,
		runner,
		resolvedMavenShimImages,
	)
	if err != nil {
		return err
	}
	defer cleanup()
	if ensureBase {
		if err := ensureLambdaBaseImageFromDockerfile(
			resolvedDockerfile,
			noCache,
			runner,
			builtBaseImages,
		); err != nil {
			return err
		}
	}

	buildCmd := buildxBuildCommand(imageRef, resolvedDockerfile, contextRoot, noCache)
	if err := runner.Run(buildCmd); err != nil {
		return fmt.Errorf("build function image %s: %w", imageRef, err)
	}
	if pushRef != imageRef {
		if err := runner.Run([]string{"docker", "tag", imageRef, pushRef}); err != nil {
			return fmt.Errorf("tag function image %s -> %s: %w", imageRef, pushRef, err)
		}
	}
	if err := runner.Run([]string{"docker", "push", pushRef}); err != nil {
		return fmt.Errorf("push function image %s: %w", pushRef, err)
	}
	return nil
}
