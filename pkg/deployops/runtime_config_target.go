package deployops

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const runtimeConfigMountDestination = "/app/runtime-config"

var errRuntimeConfigMountNotFound = errors.New("runtime-config mount not found")

type RuntimeConfigTarget struct {
	BindPath   string
	VolumeName string
}

func (t RuntimeConfigTarget) normalized() RuntimeConfigTarget {
	return RuntimeConfigTarget{
		BindPath:   strings.TrimSpace(t.BindPath),
		VolumeName: strings.TrimSpace(t.VolumeName),
	}
}

func (t RuntimeConfigTarget) isEmpty() bool {
	normalized := t.normalized()
	return normalized.BindPath == "" && normalized.VolumeName == ""
}

type RuntimeConfigResolver interface {
	ResolveRuntimeConfigTarget() (RuntimeConfigTarget, error)
}

type dockerCommandFunc func(args ...string) ([]byte, error)

type dockerRuntimeConfigResolver struct {
	projectName string
	runDocker   dockerCommandFunc
}

type dockerInspectContainer struct {
	Mounts []dockerInspectMount `json:"Mounts"`
}

type dockerInspectMount struct {
	Destination string `json:"Destination"`
	Source      string `json:"Source"`
	Type        string `json:"Type"`
	Name        string `json:"Name"`
}

func newDockerRuntimeConfigResolver() RuntimeConfigResolver {
	return dockerRuntimeConfigResolver{
		projectName: strings.TrimSpace(os.Getenv("PROJECT_NAME")),
		runDocker:   runDockerCommand,
	}
}

func (r dockerRuntimeConfigResolver) ResolveRuntimeConfigTarget() (RuntimeConfigTarget, error) {
	projectName := strings.TrimSpace(r.projectName)
	if projectName != "" {
		return r.resolveWithProject(projectName)
	}
	return r.resolveWithoutProject()
}

func (r dockerRuntimeConfigResolver) resolveWithProject(projectName string) (RuntimeConfigTarget, error) {
	services := []string{"gateway", "provisioner"}
	for _, service := range services {
		containerIDs, err := r.listComposeContainers(
			fmt.Sprintf("label=com.docker.compose.project=%s", projectName),
			fmt.Sprintf("label=com.docker.compose.service=%s", service),
		)
		if err != nil {
			return RuntimeConfigTarget{}, err
		}
		target, err := r.resolveFromContainers(containerIDs)
		if err == nil {
			return target, nil
		}
		if errors.Is(err, errRuntimeConfigMountNotFound) {
			continue
		}
		return RuntimeConfigTarget{}, err
	}
	return RuntimeConfigTarget{}, fmt.Errorf(
		"runtime-config mount was not found for compose project %q; ensure stack is running",
		projectName,
	)
}

func (r dockerRuntimeConfigResolver) resolveWithoutProject() (RuntimeConfigTarget, error) {
	services := []string{"gateway", "provisioner"}
	for _, service := range services {
		containerIDs, err := r.listComposeContainers(
			fmt.Sprintf("label=com.docker.compose.service=%s", service),
		)
		if err != nil {
			return RuntimeConfigTarget{}, err
		}
		if len(containerIDs) == 0 {
			continue
		}
		if len(containerIDs) > 1 {
			return RuntimeConfigTarget{}, fmt.Errorf(
				"multiple running %s containers detected; set PROJECT_NAME to disambiguate",
				service,
			)
		}
		return r.resolveFromContainers(containerIDs)
	}
	return RuntimeConfigTarget{}, fmt.Errorf("no running gateway/provisioner compose container found")
}

func (r dockerRuntimeConfigResolver) resolveFromContainers(containerIDs []string) (RuntimeConfigTarget, error) {
	if len(containerIDs) == 0 {
		return RuntimeConfigTarget{}, errRuntimeConfigMountNotFound
	}
	for _, containerID := range containerIDs {
		target, err := r.inspectRuntimeConfigTarget(containerID)
		if err == nil {
			return target, nil
		}
		if errors.Is(err, errRuntimeConfigMountNotFound) {
			continue
		}
		return RuntimeConfigTarget{}, err
	}
	return RuntimeConfigTarget{}, errRuntimeConfigMountNotFound
}

func (r dockerRuntimeConfigResolver) listComposeContainers(filters ...string) ([]string, error) {
	args := []string{"ps", "-q"}
	for _, filter := range filters {
		normalized := strings.TrimSpace(filter)
		if normalized == "" {
			continue
		}
		args = append(args, "--filter", normalized)
	}
	output, err := r.docker(args...)
	if err != nil {
		return nil, fmt.Errorf("list compose containers: %w", err)
	}
	return parseContainerIDs(output), nil
}

func (r dockerRuntimeConfigResolver) inspectRuntimeConfigTarget(containerID string) (RuntimeConfigTarget, error) {
	normalizedID := strings.TrimSpace(containerID)
	if normalizedID == "" {
		return RuntimeConfigTarget{}, errRuntimeConfigMountNotFound
	}
	output, err := r.docker("inspect", normalizedID)
	if err != nil {
		return RuntimeConfigTarget{}, fmt.Errorf("inspect container %s: %w", normalizedID, err)
	}
	var payload []dockerInspectContainer
	if err := json.Unmarshal(output, &payload); err != nil {
		return RuntimeConfigTarget{}, fmt.Errorf("decode docker inspect payload for %s: %w", normalizedID, err)
	}
	if len(payload) == 0 {
		return RuntimeConfigTarget{}, fmt.Errorf("docker inspect returned no containers for %s", normalizedID)
	}
	for _, mount := range payload[0].Mounts {
		if strings.TrimSpace(mount.Destination) != runtimeConfigMountDestination {
			continue
		}
		mountType := strings.ToLower(strings.TrimSpace(mount.Type))
		if mountType == "volume" {
			name := strings.TrimSpace(mount.Name)
			if name != "" {
				return RuntimeConfigTarget{VolumeName: name}, nil
			}
		}
		source := strings.TrimSpace(mount.Source)
		if source == "" {
			return RuntimeConfigTarget{}, fmt.Errorf(
				"runtime-config mount source is empty for container %s",
				normalizedID,
			)
		}
		return RuntimeConfigTarget{BindPath: source}, nil
	}
	return RuntimeConfigTarget{}, errRuntimeConfigMountNotFound
}

func (r dockerRuntimeConfigResolver) docker(args ...string) ([]byte, error) {
	runDocker := r.runDocker
	if runDocker == nil {
		runDocker = runDockerCommand
	}
	return runDocker(args...)
}

func runDockerCommand(args ...string) ([]byte, error) {
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput == "" {
			return nil, fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
		}
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, trimmedOutput)
	}
	return output, nil
}

func parseContainerIDs(output []byte) []string {
	lines := strings.Split(string(output), "\n")
	ids := make([]string, 0, len(lines))
	for _, line := range lines {
		containerID := strings.TrimSpace(line)
		if containerID == "" {
			continue
		}
		ids = append(ids, containerID)
	}
	return ids
}
