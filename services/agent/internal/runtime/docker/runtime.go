package docker

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/poruru/edge-serverless-box/services/agent/internal/config"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Client defines the subset of Docker API used by Agent.
type Client interface {
	ContainerList(ctx context.Context, options container.ListOptions) ([]container.Summary, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	NetworkConnect(ctx context.Context, networkID, containerID string, config *network.EndpointSettings) error
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ImagePull(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error)
}

type Runtime struct {
	client        Client
	networkID     string
	env           string
	brandSlug     string
	accessTracker sync.Map // map[containerID]time.Time - tracks last access time
}

// NewRuntime creates a new Docker runtime.
func NewRuntime(client Client, networkID, env, brandSlug string) *Runtime {
	brand := strings.TrimSpace(brandSlug)
	if brand == "" {
		brand = "esb"
	}
	return &Runtime{
		client:    client,
		networkID: networkID,
		env:       env,
		brandSlug: brand,
	}
}

func (r *Runtime) Ensure(ctx context.Context, req runtime.EnsureRequest) (*runtime.WorkerInfo, error) {
	// Phase 4-1: Factory behavior. Always create a new container.
	// Pool management is handled by the Gateway.
	// Mutex removed to allow parallel provisioning.

	ownerID := strings.TrimSpace(req.OwnerID)
	if ownerID == "" {
		return nil, fmt.Errorf("owner_id is required")
	}

	imageName := req.Image
	if imageName == "" {
		// Phase 5 Step 0: Support container registry
		registry := os.Getenv("CONTAINER_REGISTRY")
		if registry == "" {
			registry = config.DefaultContainerRegistry
		}
		baseImage, err := runtime.ResolveFunctionImageName(req.FunctionName)
		if err != nil {
			return nil, err
		}
		tag := runtime.ResolveFunctionImageTag()
		if registry != "" {
			imageName = fmt.Sprintf("%s/%s:%s", registry, baseImage, tag)
		} else {
			imageName = fmt.Sprintf("%s:%s", baseImage, tag)
		}
	}

	// Phase 7: Use new container name format: {brand}-{env}-{func}-{uuid}
	u := uuid.New()
	id := hex.EncodeToString(u[:4])
	containerName := fmt.Sprintf("%s-%s-%s-%s", r.brandSlug, r.env, req.FunctionName, id)

	// Phase 5 Step 0: Pull image from registry if set
	registry := os.Getenv("CONTAINER_REGISTRY")
	if registry == "" {
		registry = config.DefaultContainerRegistry
	}
	if registry != "" {
		fmt.Printf("[Agent] Pulling image %s...\n", imageName)
		pullReader, err := r.client.ImagePull(ctx, imageName, image.PullOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to pull image %s: %w", imageName, err)
		}
		defer pullReader.Close()

		// Wait for pull to complete
		_, _ = io.Copy(io.Discard, pullReader)
	} else {
		fmt.Printf("[Agent] Skipping pull for local image %s (no registry configured)\n", imageName)
	}

	envList := make([]string, 0, len(req.Env))
	for k, v := range req.Env {
		envList = append(envList, fmt.Sprintf("%s=%s", k, v))
	}

	config := &container.Config{
		Image: imageName,
		Env:   envList,
		Labels: map[string]string{
			runtime.LabelFunctionName: req.FunctionName,
			runtime.LabelCreatedBy:    runtime.ValueCreatedByAgent,
			runtime.LabelEsbEnv:       r.env,
			runtime.LabelFunctionKind: runtime.ValueFunctionKind,
			runtime.LabelOwner:        ownerID,
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

	// Record initial access time
	r.accessTracker.Store(containerID, time.Now())

	// Retry IP resolution with exponential backoff
	var ip string
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		info, err := r.client.ContainerInspect(ctx, containerID)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect container: %w", err)
		}

		if info.NetworkSettings != nil && info.NetworkSettings.Networks != nil {
			if netData, ok := info.NetworkSettings.Networks[r.networkID]; ok && netData.IPAddress != "" {
				ip = netData.IPAddress
				break
			}
			for _, netData := range info.NetworkSettings.Networks {
				if netData.IPAddress != "" {
					ip = netData.IPAddress
					break
				}
			}
		}

		if ip != "" {
			break
		}

		if i < maxRetries-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(100*(1<<i)) * time.Millisecond): // 100ms, 200ms, 400ms, 800ms, 1.6s
			}
		}
	}

	if ip == "" {
		return nil, fmt.Errorf("container %s started but IP address not available after %d retries", containerID, maxRetries)
	}

	return &runtime.WorkerInfo{
		ID:        containerID,
		IPAddress: ip,
		Port:      8080,
		OwnerID:   ownerID,
	}, nil
}

func (r *Runtime) Destroy(ctx context.Context, id string) error {
	if err := r.client.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	r.accessTracker.Delete(id)
	return nil
}

func (r *Runtime) Touch(id string) {
	r.accessTracker.Store(id, time.Now())
}

func (r *Runtime) Suspend(_ context.Context, _ string) error {
	// We could call Docker's Pause, but Phase 2's main focus is containerd.
	// For Docker, keep it simplified or unimplemented, but return a stub or error for compatibility.
	return status.Error(codes.Unimplemented, "pause not implemented for docker runtime")
}

func (r *Runtime) Resume(_ context.Context, _ string) error {
	return status.Error(codes.Unimplemented, "resume not implemented for docker runtime")
}

func (r *Runtime) Close() error {
	return nil
}

// GC - Docker runtime doesn't require GC as containers are managed by Docker daemon.
// This is a stub for interface compatibility.
func (r *Runtime) GC(_ context.Context) error {
	// No-op for Docker runtime
	return nil
}

// List returns the state of all managed containers.
func (r *Runtime) List(ctx context.Context) ([]runtime.ContainerState, error) {
	filter := filters.NewArgs()
	filter.Add("label", fmt.Sprintf("%s=%s", runtime.LabelCreatedBy, runtime.ValueCreatedByAgent))
	// Phase 7: Filter by environment label
	filter.Add("label", fmt.Sprintf("%s=%s", runtime.LabelEsbEnv, r.env))
	filter.Add("label", fmt.Sprintf("%s=%s", runtime.LabelFunctionKind, runtime.ValueFunctionKind))

	containers, err := r.client.ContainerList(ctx, container.ListOptions{
		Filters: filter,
		All:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	states := make([]runtime.ContainerState, 0, len(containers))
	for _, c := range containers {
		funcName := c.Labels[runtime.LabelFunctionName]
		if funcName == "" {
			continue
		}
		ownerID := c.Labels[runtime.LabelOwner]

		// For Phase 1/3, we use Created time as a base, but check accessTracker.
		createdTime := time.Unix(c.Created, 0)
		lastUsedAt := createdTime
		if val, ok := r.accessTracker.Load(c.ID); ok {
			lastUsedAt = val.(time.Time)
		}

		// Docker names start with /
		name := ""
		if len(c.Names) > 0 {
			name = c.Names[0]
			if name[0] == '/' {
				name = name[1:]
			}
		}

		ipAddress, _ := r.resolveContainerIP(ctx, c.ID)

		states = append(states, runtime.ContainerState{
			ID:            c.ID,
			FunctionName:  funcName,
			Status:        normalizeDockerStatus(c.State),
			LastUsedAt:    lastUsedAt,
			ContainerName: name,
			CreatedAt:     createdTime, // Container creation time from Docker API
			IPAddress:     ipAddress,
			Port:          8080,
			OwnerID:       ownerID,
		})
	}
	return states, nil
}

func (r *Runtime) resolveContainerIP(ctx context.Context, containerID string) (string, error) {
	info, err := r.client.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
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
	return ip, nil
}

func (r *Runtime) Metrics(_ context.Context, _ string) (*runtime.ContainerMetrics, error) {
	return nil, fmt.Errorf("metrics not implemented for docker runtime")
}

func normalizeDockerStatus(state string) string {
	switch state {
	case "running":
		return runtime.StatusRunning
	case "paused":
		return runtime.StatusPaused
	case "exited", "dead":
		return runtime.StatusStopped
	default:
		return runtime.StatusUnknown
	}
}
