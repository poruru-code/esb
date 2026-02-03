// Where: cli/internal/generator/bundle_manifest.go
// What: DinD bundle manifest writer for deterministic image bundling.
// Why: Ensure bundler only packages template-derived images with traceability metadata.
package generator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/poruru/edge-serverless-box/cli/internal/infra/compose"
	"github.com/poruru/edge-serverless-box/meta"
)

const bundleManifestSchemaVersion = "1.0"

type bundleManifest struct {
	SchemaVersion string                `json:"schema_version"`
	GeneratedAt   string                `json:"generated_at"`
	Template      bundleTemplate        `json:"template"`
	Build         bundleBuild           `json:"build"`
	Images        []bundleManifestImage `json:"images"`
}

type bundleTemplate struct {
	Path       string            `json:"path"`
	Sha256     string            `json:"sha256"`
	Parameters map[string]string `json:"parameters"`
}

type bundleBuild struct {
	Project     string         `json:"project"`
	Env         string         `json:"env"`
	Mode        string         `json:"mode"`
	ImagePrefix string         `json:"image_prefix"`
	ImageTag    string         `json:"image_tag"`
	Git         bundleBuildGit `json:"git"`
}

type bundleBuildGit struct {
	Commit string `json:"commit"`
	Dirty  bool   `json:"dirty"`
}

type bundleManifestImage struct {
	Name     string            `json:"name"`
	Digest   string            `json:"digest"`
	Kind     string            `json:"kind"`
	Source   string            `json:"source"`
	Labels   map[string]string `json:"labels,omitempty"`
	Platform string            `json:"platform"`
}

type bundleManifestInput struct {
	RepoRoot        string
	OutputDir       string
	TemplatePath    string
	Parameters      map[string]any
	Project         string
	Env             string
	Mode            string
	ImageTag        string
	Registry        string
	ServiceRegistry string
	Functions       []FunctionSpec
	Runner          compose.CommandRunner
}

func writeBundleManifest(ctx context.Context, input bundleManifestInput) (string, error) {
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

	manifest := bundleManifest{
		SchemaVersion: bundleManifestSchemaVersion,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Template: bundleTemplate{
			Path:       templatePath,
			Sha256:     templateHash,
			Parameters: parameters,
		},
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

func collectBundleImages(ctx context.Context, input bundleManifestInput) ([]bundleManifestImage, error) {
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

	// Base images
	if err := add(lambdaBaseImageTag(functionRegistry, input.ImageTag), "base", "generated"); err != nil {
		return nil, err
	}
	if err := add(fmt.Sprintf("%s-os-base:latest", meta.ImagePrefix), "base", "internal"); err != nil {
		return nil, err
	}
	if err := add(fmt.Sprintf("%s-python-base:latest", meta.ImagePrefix), "base", "internal"); err != nil {
		return nil, err
	}

	// Control plane images
	for _, name := range controlPlaneImages(mode, serviceRegistry, input.ImageTag) {
		if err := add(name, "service", "internal"); err != nil {
			return nil, err
		}
	}

	// Function images
	for _, fn := range input.Functions {
		imageTag := fmt.Sprintf("%s-%s:%s", meta.ImagePrefix, fn.ImageName, input.ImageTag)
		imageTag = joinRegistry(functionRegistry, imageTag)
		if err := add(imageTag, "function", "template"); err != nil {
			return nil, err
		}
	}

	// External images
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

func resolveManifestTemplatePath(repoRoot, templatePath string) string {
	root := strings.TrimSpace(repoRoot)
	path := strings.TrimSpace(templatePath)
	if root == "" || path == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return filepath.ToSlash(rel)
}

func stringifyParameters(values map[string]any) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(values))
	for _, key := range keys {
		val := values[key]
		out[key] = fmt.Sprint(val)
	}
	return out
}

func resolveGitMetadata(ctx context.Context, runner compose.CommandRunner, repoRoot string) (string, bool, error) {
	commit, err := runGit(ctx, runner, repoRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", false, err
	}
	out, err := runner.RunOutput(ctx, repoRoot, "git", "status", "--porcelain")
	if err != nil {
		return "", false, fmt.Errorf("git status failed: %w", err)
	}
	dirty := strings.TrimSpace(string(out)) != ""
	return commit, dirty, nil
}

func hashFileSha256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read template: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
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
