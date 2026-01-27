package generator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/poruru/edge-serverless-box/meta"
)

type manifestImageMeta struct {
	id       string
	platform string
}

type manifestRunner struct {
	images map[string]manifestImageMeta
}

func (r *manifestRunner) Run(_ context.Context, _, _ string, _ ...string) error {
	return nil
}

func (r *manifestRunner) RunQuiet(_ context.Context, _, _ string, _ ...string) error {
	return nil
}

func (r *manifestRunner) RunOutput(_ context.Context, _, name string, args ...string) ([]byte, error) {
	if name == "git" {
		if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "HEAD" {
			return []byte("deadbeef\n"), nil
		}
		if len(args) >= 2 && args[0] == "status" && args[1] == "--porcelain" {
			return []byte(""), nil
		}
	}
	if name == "docker" && len(args) >= 3 && args[0] == "image" {
		switch args[1] {
		case "ls":
			tag := args[len(args)-1]
			if _, ok := r.images[tag]; ok {
				return []byte("present\n"), nil
			}
			return []byte(""), nil
		case "inspect":
			if len(args) < 5 {
				return []byte(""), nil
			}
			format := args[3]
			tag := args[4]
			meta, ok := r.images[tag]
			if !ok {
				return []byte(""), nil
			}
			switch format {
			case "{{.Id}}":
				return []byte(meta.id + "\n"), nil
			case "{{.Os}}/{{.Architecture}}":
				return []byte(meta.platform + "\n"), nil
			}
		}
	}
	return []byte(""), nil
}

func TestWriteBundleManifest(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	functions := []FunctionSpec{
		{Name: "Hello", ImageName: "lambda-hello"},
	}

	imageTag := "latest"
	functionImage := strings.Join([]string{meta.ImagePrefix + "-lambda-hello", imageTag}, ":")

	images := map[string]manifestImageMeta{
		lambdaBaseImageTag("", imageTag):            {id: "sha256:1111111111111111111111111111111111111111111111111111111111111111", platform: "linux/amd64"},
		meta.ImagePrefix + "-os-base:latest":        {id: "sha256:2222222222222222222222222222222222222222222222222222222222222222", platform: "linux/amd64"},
		meta.ImagePrefix + "-python-base:latest":    {id: "sha256:3333333333333333333333333333333333333333333333333333333333333333", platform: "linux/amd64"},
		meta.ImagePrefix + "-gateway-docker:latest": {id: "sha256:4444444444444444444444444444444444444444444444444444444444444444", platform: "linux/amd64"},
		meta.ImagePrefix + "-agent-docker:latest":   {id: "sha256:5555555555555555555555555555555555555555555555555555555555555555", platform: "linux/amd64"},
		meta.ImagePrefix + "-provisioner:latest":    {id: "sha256:6666666666666666666666666666666666666666666666666666666666666666", platform: "linux/amd64"},
		functionImage:                               {id: "sha256:7777777777777777777777777777777777777777777777777777777777777777", platform: "linux/amd64"},
		"alpine:latest":                             {id: "sha256:8888888888888888888888888888888888888888888888888888888888888888", platform: "linux/amd64"},
		"rustfs/rustfs:latest":                      {id: "sha256:9999999999999999999999999999999999999999999999999999999999999999", platform: "linux/amd64"},
		"scylladb/scylla:latest":                    {id: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", platform: "linux/amd64"},
		"victoriametrics/victoria-logs:latest":      {id: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", platform: "linux/amd64"},
	}

	runner := &manifestRunner{images: images}
	outputDir := filepath.Join(tmpDir, ".esb", "default")
	if err := ensureDir(outputDir); err != nil {
		t.Fatalf("ensure output dir: %v", err)
	}

	path, err := writeBundleManifest(context.Background(), bundleManifestInput{
		RepoRoot:     tmpDir,
		OutputDir:    outputDir,
		TemplatePath: templatePath,
		Parameters:   map[string]any{"ParamA": "value"},
		Project:      "esb-default",
		Env:          "default",
		Mode:         "docker",
		ImageTag:     imageTag,
		Registry:     "",
		Functions:    functions,
		Runner:       runner,
	})
	if err != nil {
		t.Fatalf("write bundle manifest: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest bundleManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if manifest.SchemaVersion != bundleManifestSchemaVersion {
		t.Fatalf("unexpected schema version: %s", manifest.SchemaVersion)
	}
	if manifest.Template.Path != "template.yaml" {
		t.Fatalf("unexpected template path: %s", manifest.Template.Path)
	}
	if !reflect.DeepEqual(manifest.Template.Parameters, map[string]string{"ParamA": "value"}) {
		t.Fatalf("unexpected parameters: %#v", manifest.Template.Parameters)
	}

	expectedImages := []string{
		lambdaBaseImageTag("", imageTag),
		meta.ImagePrefix + "-os-base:latest",
		meta.ImagePrefix + "-python-base:latest",
		meta.ImagePrefix + "-gateway-docker:latest",
		meta.ImagePrefix + "-agent-docker:latest",
		meta.ImagePrefix + "-provisioner:latest",
		functionImage,
		"alpine:latest",
		"rustfs/rustfs:latest",
		"scylladb/scylla:latest",
		"victoriametrics/victoria-logs:latest",
	}
	gotImages := make([]string, 0, len(manifest.Images))
	for _, img := range manifest.Images {
		gotImages = append(gotImages, img.Name)
	}
	for _, name := range expectedImages {
		if !contains(gotImages, name) {
			t.Fatalf("expected image %s in manifest", name)
		}
	}
}

func TestWriteBundleManifestContainerdIncludesRuntimeImages(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	functions := []FunctionSpec{
		{Name: "Hello", ImageName: "lambda-hello"},
	}

	imageTag := "latest"
	functionRegistry := "localhost:5010/"
	serviceRegistry := "localhost:5010/"
	functionImage := functionRegistry + meta.ImagePrefix + "-lambda-hello:" + imageTag

	images := map[string]manifestImageMeta{
		lambdaBaseImageTag(functionRegistry, imageTag):                    {id: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", platform: "linux/amd64"},
		meta.ImagePrefix + "-os-base:latest":                              {id: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", platform: "linux/amd64"},
		meta.ImagePrefix + "-python-base:latest":                          {id: "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", platform: "linux/amd64"},
		serviceRegistry + meta.ImagePrefix + "-gateway-containerd:latest": {id: "sha256:dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", platform: "linux/amd64"},
		serviceRegistry + meta.ImagePrefix + "-agent-containerd:latest":   {id: "sha256:eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", platform: "linux/amd64"},
		serviceRegistry + meta.ImagePrefix + "-runtime-node-containerd:latest": {
			id:       "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
			platform: "linux/amd64",
		},
		serviceRegistry + meta.ImagePrefix + "-provisioner:latest": {id: "sha256:1111111111111111111111111111111111111111111111111111111111111111", platform: "linux/amd64"},
		functionImage:                          {id: "sha256:2222222222222222222222222222222222222222222222222222222222222222", platform: "linux/amd64"},
		"alpine:latest":                        {id: "sha256:3333333333333333333333333333333333333333333333333333333333333333", platform: "linux/amd64"},
		"rustfs/rustfs:latest":                 {id: "sha256:4444444444444444444444444444444444444444444444444444444444444444", platform: "linux/amd64"},
		"scylladb/scylla:latest":               {id: "sha256:5555555555555555555555555555555555555555555555555555555555555555", platform: "linux/amd64"},
		"victoriametrics/victoria-logs:latest": {id: "sha256:6666666666666666666666666666666666666666666666666666666666666666", platform: "linux/amd64"},
		"coredns/coredns:1.11.1":               {id: "sha256:7777777777777777777777777777777777777777777777777777777777777777", platform: "linux/amd64"},
		"registry:2":                           {id: "sha256:8888888888888888888888888888888888888888888888888888888888888888", platform: "linux/amd64"},
	}

	runner := &manifestRunner{images: images}
	outputDir := filepath.Join(tmpDir, ".esb", "default")
	if err := ensureDir(outputDir); err != nil {
		t.Fatalf("ensure output dir: %v", err)
	}

	_, err := writeBundleManifest(context.Background(), bundleManifestInput{
		RepoRoot:        tmpDir,
		OutputDir:       outputDir,
		TemplatePath:    templatePath,
		Parameters:      map[string]any{},
		Project:         "esb-default",
		Env:             "default",
		Mode:            "containerd",
		ImageTag:        imageTag,
		Registry:        functionRegistry,
		ServiceRegistry: serviceRegistry,
		Functions:       functions,
		Runner:          runner,
	})
	if err != nil {
		t.Fatalf("write bundle manifest: %v", err)
	}
}

func TestWriteBundleManifestFailsWhenImageMissing(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "template.yaml")
	if err := os.WriteFile(templatePath, []byte("Resources: {}"), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	runner := &manifestRunner{images: map[string]manifestImageMeta{}}
	outputDir := filepath.Join(tmpDir, ".esb", "default")
	if err := ensureDir(outputDir); err != nil {
		t.Fatalf("ensure output dir: %v", err)
	}

	_, err := writeBundleManifest(context.Background(), bundleManifestInput{
		RepoRoot:     tmpDir,
		OutputDir:    outputDir,
		TemplatePath: templatePath,
		Parameters:   map[string]any{},
		Project:      "esb-default",
		Env:          "default",
		Mode:         "docker",
		ImageTag:     "latest",
		Registry:     "",
		Functions:    []FunctionSpec{},
		Runner:       runner,
	})
	if err == nil {
		t.Fatalf("expected error when images are missing")
	}
	if !strings.Contains(err.Error(), "image not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func contains(values []string, needle string) bool {
	for _, val := range values {
		if val == needle {
			return true
		}
	}
	return false
}
