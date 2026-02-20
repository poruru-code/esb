package deployops

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/poruru/edge-serverless-box/pkg/artifactcore"
	"github.com/poruru/edge-serverless-box/pkg/runtimeimage"
)

func probeRuntimeObservation(manifest artifactcore.ArtifactManifest) (*artifactcore.RuntimeObservation, []string, error) {
	project := strings.TrimSpace(manifest.Project)
	if project == "" {
		return nil, nil, fmt.Errorf("project is empty in artifact manifest")
	}
	format := `{{.Label "com.docker.compose.service"}} {{.Image}}`
	cmd := exec.Command(
		"docker",
		"ps",
		"--filter", "label=com.docker.compose.project="+project,
		"--format", format,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, nil, fmt.Errorf("run docker ps: %w: %s", err, strings.TrimSpace(string(out)))
	}

	serviceImages := parseServiceImages(string(out))
	if len(serviceImages) == 0 {
		return nil, []string{fmt.Sprintf("runtime compatibility probe found no running compose services for project %q", project)}, nil
	}

	imageRef, _ := runtimeimage.PreferredServiceImage(serviceImages)

	warnings := make([]string, 0)
	mode := runtimeimage.InferModeFromServiceImages(serviceImages)
	if mode == "" {
		warnings = append(warnings, "runtime compatibility probe could not infer runtime mode from running images")
	}
	esbVersion := runtimeimage.ParseTag(imageRef)
	if esbVersion == "" {
		warnings = append(warnings, fmt.Sprintf("runtime compatibility probe could not infer esb version tag from image %q", imageRef))
	}
	observation := &artifactcore.RuntimeObservation{
		Mode:       mode,
		ESBVersion: esbVersion,
		Source:     fmt.Sprintf("docker ps (project=%s)", project),
	}
	return observation, warnings, nil
}

func parseServiceImages(raw string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		service, image, found := strings.Cut(line, " ")
		if !found {
			continue
		}
		service = strings.TrimSpace(service)
		image = strings.TrimSpace(image)
		if service == "" || image == "" {
			continue
		}
		out[service] = image
	}
	return out
}
