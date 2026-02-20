// Where: cli/internal/infra/templategen/bundle_manifest.go
// What: Bundle manifest write flow and image collection logic.
// Why: Keep orchestration readable while delegating schema and utility helpers.
package templategen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/poruru-code/esb/cli/internal/infra/compose"
	"github.com/poruru-code/esb/cli/internal/meta"
)

// WriteBundleManifest writes bundle/manifest.json for deterministic image bundling.
func WriteBundleManifest(ctx context.Context, input BundleManifestInput) (string, error) {
	if strings.TrimSpace(input.OutputDir) == "" {
		return "", fmt.Errorf("bundle manifest output dir is required")
	}
	if strings.TrimSpace(input.TemplatePath) == "" {
		return "", fmt.Errorf("bundle manifest template path is required")
	}
	if input.Runner == nil {
		return "", fmt.Errorf("bundle manifest runner is required")
	}

	templatePath := resolveManifestTemplatePath(input.RepoRoot, input.TemplatePath)
	templateHash, err := hashFileSha256(input.TemplatePath)
	if err != nil {
		return "", err
	}
	parameters := stringifyParameters(input.Parameters)
	commit, dirty, err := resolveGitMetadata(ctx, input.Runner, input.RepoRoot)
	if err != nil {
		return "", err
	}

	images, err := collectBundleImages(ctx, input)
	if err != nil {
		return "", err
	}

	template := bundleTemplate{
		Path:       templatePath,
		Sha256:     templateHash,
		Parameters: parameters,
	}
	manifest := bundleManifest{
		SchemaVersion: bundleManifestSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Template:      template,
		Templates:     []bundleTemplate{template},
		Build: bundleBuild{
			Project:     input.Project,
			Env:         input.Env,
			Mode:        input.Mode,
			ImagePrefix: meta.ImagePrefix,
			ImageTag:    input.ImageTag,
			Git: bundleBuildGit{
				Commit: commit,
				Dirty:  dirty,
			},
		},
		Images: images,
	}

	bundleDir := filepath.Join(input.OutputDir, "bundle")
	if err := ensureDir(bundleDir); err != nil {
		return "", err
	}
	path := filepath.Join(bundleDir, "manifest.json")
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal bundle manifest: %w", err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return "", fmt.Errorf("write bundle manifest: %w", err)
	}
	return path, nil
}

func collectBundleImages(ctx context.Context, input BundleManifestInput) ([]bundleManifestImage, error) {
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = compose.ModeDocker
	}

	serviceRegistry := strings.TrimSpace(input.ServiceRegistry)
	functionRegistry := strings.TrimSpace(input.Registry)

	images := make([]bundleManifestImage, 0)
	add := func(name, kind, source string) error {
		if strings.TrimSpace(name) == "" {
			return nil
		}
		digest := dockerImageID(ctx, input.Runner, input.RepoRoot, name)
		if digest == "" {
			return fmt.Errorf("bundle manifest: image not found: %s", name)
		}
		platform := dockerImagePlatform(ctx, input.Runner, input.RepoRoot, name)
		if platform == "" {
			return fmt.Errorf("bundle manifest: platform not found for %s", name)
		}
		images = append(images, bundleManifestImage{
			Name:     name,
			Digest:   digest,
			Kind:     kind,
			Source:   source,
			Platform: platform,
		})
		return nil
	}

	if err := add(lambdaBaseImageTag(functionRegistry, input.ImageTag), "base", "generated"); err != nil {
		return nil, err
	}
	if err := add(fmt.Sprintf("%s-os-base:latest", meta.ImagePrefix), "base", "internal"); err != nil {
		return nil, err
	}
	if err := add(fmt.Sprintf("%s-python-base:latest", meta.ImagePrefix), "base", "internal"); err != nil {
		return nil, err
	}

	for _, name := range controlPlaneImages(mode, serviceRegistry, input.ImageTag) {
		if err := add(name, "service", "internal"); err != nil {
			return nil, err
		}
	}

	for _, fn := range input.Functions {
		imageTag := strings.TrimSpace(fn.ImageRef)
		if strings.TrimSpace(fn.ImageName) != "" {
			imageTag = fmt.Sprintf("%s-%s:%s", meta.ImagePrefix, fn.ImageName, input.ImageTag)
			imageTag = joinRegistry(functionRegistry, imageTag)
		}
		if imageTag == "" {
			return nil, fmt.Errorf("bundle manifest: image name is required for function %s", fn.Name)
		}
		if err := add(imageTag, "function", "template"); err != nil {
			return nil, err
		}
	}

	for _, name := range externalImages(mode) {
		if err := ensureDockerImage(ctx, input.Runner, input.RepoRoot, name); err != nil {
			return nil, err
		}
		if err := add(name, "external", "external"); err != nil {
			return nil, err
		}
	}

	return images, nil
}

func controlPlaneImages(mode, registry, tag string) []string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = "latest"
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	images := []string{}
	switch mode {
	case compose.ModeContainerd:
		images = append(images,
			fmt.Sprintf("%s-gateway-containerd:%s", meta.ImagePrefix, tag),
			fmt.Sprintf("%s-agent-containerd:%s", meta.ImagePrefix, tag),
			fmt.Sprintf("%s-runtime-node-containerd:%s", meta.ImagePrefix, tag),
		)
	default:
		images = append(images,
			fmt.Sprintf("%s-gateway-docker:%s", meta.ImagePrefix, tag),
			fmt.Sprintf("%s-agent-docker:%s", meta.ImagePrefix, tag),
		)
	}
	images = append(images, fmt.Sprintf("%s-provisioner:%s", meta.ImagePrefix, tag))

	out := make([]string, 0, len(images))
	for _, name := range images {
		out = append(out, joinRegistry(registry, name))
	}
	sort.Strings(out)
	return out
}

func externalImages(mode string) []string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	external := []string{
		"alpine:latest",
		"rustfs/rustfs:latest",
		"scylladb/scylla:latest",
		"victoriametrics/victoria-logs:latest",
	}
	if mode == compose.ModeContainerd {
		external = append(external, "coredns/coredns:1.11.1", "registry:2")
	}
	sort.Strings(external)
	return external
}

func dockerImagePlatform(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
) string {
	if runner == nil || imageTag == "" {
		return ""
	}
	if !dockerImageExists(ctx, runner, contextDir, imageTag) {
		return ""
	}
	out, err := runner.RunOutput(ctx, contextDir, "docker", "image", "inspect", "--format", "{{.Os}}/{{.Architecture}}", imageTag)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func ensureDockerImage(
	ctx context.Context,
	runner compose.CommandRunner,
	contextDir string,
	imageTag string,
) error {
	if dockerImageExists(ctx, runner, contextDir, imageTag) {
		return nil
	}
	if err := runner.Run(ctx, contextDir, "docker", "pull", imageTag); err != nil {
		return fmt.Errorf("pull image %s: %w", imageTag, err)
	}
	return nil
}
