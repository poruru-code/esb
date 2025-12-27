package runtime

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// DockerClient defines the subset of Docker API used by Agent.
// This interface allows mocking for testing.
type DockerClient interface {
	ContainerList(ctx context.Context, options types.ContainerListOptions) ([]types.Container, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options types.ContainerStartOptions) error
	// NetworkConnect is crucial for ensuring Lambda containers can talk to Gateway
	NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerRemove(ctx context.Context, containerID string, options types.ContainerRemoveOptions) error
	ImagePull(ctx context.Context, ref string, options types.ImagePullOptions) (io.ReadCloser, error)
}

type DockerRuntime struct {
	client    DockerClient
	networkID string
}

func NewDockerRuntime(client DockerClient, networkID string) *DockerRuntime {
	return &DockerRuntime{
		client:    client,
		networkID: networkID,
	}
}

// WorkerInfo represents a running container instance
type WorkerInfo struct {
	ID        string
	Name      string
	IPAddress string
	Port      int32
}

func (r *DockerRuntime) EnsureContainer(ctx context.Context, functionName string, image string, env map[string]string) (*WorkerInfo, error) {
	// 1. Check if container exists
	filter := filters.NewArgs()
	filter.Add("label", fmt.Sprintf("esb_function=%s", functionName))

	containers, err := r.client.ContainerList(ctx, types.ContainerListOptions{
		Filters: filter,
		All:     true, // Include stopped containers to restart them if needed
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var containerID string
	var containerName string

	if len(containers) > 0 {
		// Found existing container
		c := containers[0]
		containerID = c.ID
		containerName = c.Names[0] // Usually starts with /
		if len(containerName) > 1 && containerName[0] == '/' {
			containerName = containerName[1:]
		}

		if c.State != "running" {
			// Restart
			if err := r.client.ContainerStart(ctx, containerID, types.ContainerStartOptions{}); err != nil {
				return nil, fmt.Errorf("failed to start existing container: %w", err)
			}
		}
	} else {
		// Create new container
		if image == "" {
			image = fmt.Sprintf("%s:latest", functionName)
		}

		containerName = fmt.Sprintf("lambda-%s-%d", functionName, time.Now().UnixNano())

		// Prepare Env
		envList := make([]string, 0, len(env))
		for k, v := range env {
			envList = append(envList, fmt.Sprintf("%s=%s", k, v))
		}

		config := &container.Config{
			Image: image,
			Env:   envList,
			Labels: map[string]string{
				"esb_function": functionName,
				"created_by":   "esb-agent",
			},
			ExposedPorts: nat.PortSet{
				"8080/tcp": struct{}{},
			},
		}

		hostConfig := &container.HostConfig{
			RestartPolicy: container.RestartPolicy{Name: "no"},
			// PortBindings can be added if we want to expose to host, but usually internal network is enough
		}

		// Important: Connect to the specified network
		networkingConfig := &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				r.networkID: {},
			},
		}

		resp, err := r.client.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, containerName)
		if err != nil {
			return nil, fmt.Errorf("failed to create container: %w", err)
		}
		containerID = resp.ID

		if err := r.client.ContainerStart(ctx, containerID, types.ContainerStartOptions{}); err != nil {
			return nil, fmt.Errorf("failed to start container: %w", err)
		}
	}

	// Inspect to get IP address
	info, err := r.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	ip := ""
	if info.NetworkSettings != nil && info.NetworkSettings.Networks != nil {
		if netData, ok := info.NetworkSettings.Networks[r.networkID]; ok {
			ip = netData.IPAddress
		}
	}

	// Fallback if IP is empty (rare case or host networking)
	if ip == "" {
		// Try to find any IP
		for _, netData := range info.NetworkSettings.Networks {
			if netData.IPAddress != "" {
				ip = netData.IPAddress
				break
			}
		}
	}

	return &WorkerInfo{
		ID:        containerID,
		Name:      containerName, // or info.Name
		IPAddress: ip,
		Port:      8080,
	}, nil
}

func (r *DockerRuntime) DestroyContainer(ctx context.Context, containerID string) error {
	return r.client.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true})
}
