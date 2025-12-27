package docker

import (
	"context"
	"fmt"
	"io"
	"time"

	"sync"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
)

// DockerClient defines the subset of Docker API used by Agent.
type DockerClient interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]types.Container, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error
	ContainerInspect(ctx context.Context, containerID string) (types.ContainerJSON, error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error)
}

type Runtime struct {
	client    DockerClient
	networkID string
	mu        sync.Mutex
}

func NewRuntime(client DockerClient, networkID string) *Runtime {
	return &Runtime{
		client:    client,
		networkID: networkID,
	}
}

func (r *Runtime) Ensure(ctx context.Context, req runtime.EnsureRequest) (*runtime.WorkerInfo, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Phase 4-1: Factory behavior. Always create a new container.
	// Pool management is handled by the Gateway.

	image := req.Image
	if image == "" {
		image = fmt.Sprintf("%s:latest", req.FunctionName)
	}

	containerName := fmt.Sprintf("lambda-%s-%d", req.FunctionName, time.Now().UnixNano())

	envList := make([]string, 0, len(req.Env))
	for k, v := range req.Env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	config := &container.Config{
		Image: image,
		Env:   envList,
		Labels: map[string]string{
			"esb_function": req.FunctionName,
			"created_by":   "esb-agent",
		},
		ExposedPorts: nat.PortSet{
			"8080/tcp": struct{}{},
		},
	}

	hostConfig := &container.HostConfig{
		RestartPolicy: container.RestartPolicy{Name: "no"},
	}

	networkingConfig := &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{
			r.networkID: {},
		},
	}

	fmt.Printf("[Agent] Creating new container %s for %s\n", containerName, req.FunctionName)
	resp, err := r.client.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}
	containerID := resp.ID

	if err := r.client.ContainerStart(ctx, containerID, container.StartOptions{}); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

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

	if ip == "" && info.NetworkSettings != nil {
		for _, netData := range info.NetworkSettings.Networks {
			if netData.IPAddress != "" {
				ip = netData.IPAddress
				break
			}
		}
	}

	return &runtime.WorkerInfo{
		ID:        containerID,
		IPAddress: ip,
		Port:      8080,
	}, nil
}

func (r *Runtime) Destroy(ctx context.Context, id string) error {
	return r.client.ContainerRemove(ctx, id, container.RemoveOptions{Force: true})
}

func (r *Runtime) Pause(ctx context.Context, id string) error {
	// Docker 自身の Pause 機能を呼ぶことも可能だが、Phase 2 の主目的は containerd。
	// Docker 版では簡略化するか、未実装でも良いが、インターフェース互換のために空実装またはエラーを返す。
	return fmt.Errorf("pause not implemented for docker runtime")
}

func (r *Runtime) Resume(ctx context.Context, id string) error {
	return fmt.Errorf("resume not implemented for docker runtime")
}

func (r *Runtime) Close() error {
	return nil
}

// GC - Docker runtime doesn't require GC as containers are managed by Docker daemon.
// This is a stub for interface compatibility.
func (r *Runtime) GC(ctx context.Context) error {
	// No-op for Docker runtime
	return nil
}

// List returns the state of all managed containers.
func (r *Runtime) List(ctx context.Context) ([]runtime.ContainerState, error) {
	filter := filters.NewArgs()
	filter.Add("label", "created_by=esb-agent")

	containers, err := r.client.ContainerList(ctx, container.ListOptions{
		Filters: filter,
		All:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	states := make([]runtime.ContainerState, 0, len(containers))
	for _, c := range containers {
		funcName := c.Labels["esb_function"]
		if funcName == "" {
			continue
		}

		// Docker API doesn't provide precise last_used_at.
		// For Phase 1/3, we use Created time as a base.
		createdTime := time.Unix(c.Created, 0)

		// Docker names start with /
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
			if name[0] == '/' {
				name = name[1:]
			}
		}

		states = append(states, runtime.ContainerState{
			ID:            c.ID,
			FunctionName:  funcName,
			Status:        c.State, // "running", "exited", etc.
			LastUsedAt:    createdTime,
			ContainerName: name,
		})
	}
	return states, nil
}
