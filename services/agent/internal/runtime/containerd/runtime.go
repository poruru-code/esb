package containerd

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/go-cni"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
)

type Runtime struct {
	client        ContainerdClient
	cni           cni.CNI
	portAllocator *PortAllocator
	namespace     string
	accessTracker sync.Map // map[containerID]time.Time - tracks last access time
}

func NewRuntime(client ContainerdClient, cniBackend cni.CNI, portAllocator *PortAllocator, namespace string) *Runtime {
	return &Runtime{
		client:        client,
		cni:           cniBackend,
		portAllocator: portAllocator,
		namespace:     namespace,
	}
}

func (r *Runtime) Ensure(ctx context.Context, req runtime.EnsureRequest) (*runtime.WorkerInfo, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	// Phase 4-1: Factory behavior. Always create a new container.
	image := req.Image
	if image == "" {
		image = fmt.Sprintf("%s:latest", req.FunctionName)
	}

	containerID := fmt.Sprintf("%s%s-%d", runtime.ContainerNamePrefix, req.FunctionName, time.Now().UnixNano())

	// 1. Ensure image (only for Cold Start)
	imgObj, err := r.ensureImage(ctx, image)
	if err != nil {
		return nil, err
	}

	// 2. Create Container
	container, err := r.client.NewContainer(ctx, containerID,
		containerd.WithNewSpec(),
		containerd.WithNewSnapshot(containerID, imgObj),
		containerd.WithContainerLabels(map[string]string{
			runtime.LabelFunctionName: req.FunctionName,
			runtime.LabelCreatedBy:    runtime.ValueCreatedByAgent,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// 3. Create and Start Task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	if err := task.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start task: %w", err)
	}

	// 4. Setup Network
	ip, port, err := r.setupNetwork(ctx, container, task)
	if err != nil {
		// Rollback task and container with detached context
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cleanupCtx = namespaces.WithNamespace(cleanupCtx, r.namespace)
		defer cancel()

		// Best effort cleanup
		_, _ = task.Delete(cleanupCtx, containerd.WithProcessKill)
		_ = container.Delete(cleanupCtx, containerd.WithSnapshotCleanup)

		return nil, fmt.Errorf("failed to setup network: %w", err)
	}

	// Record access time for Janitor
	r.accessTracker.Store(containerID, time.Now())

	return &runtime.WorkerInfo{
		ID:        containerID,
		IPAddress: ip,
		Port:      port,
	}, nil
}

func (r *Runtime) Destroy(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: %w", id, err)
	}

	// Delete task if exists
	task, err := container.Task(ctx, nil)
	if err == nil {
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
	}

	// Delete container
	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("failed to delete container %s: %w", id, err)
	}

	// Remove from accessTracker
	r.accessTracker.Delete(id)

	return nil
}

func (r *Runtime) Pause(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: %w", id, err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task for container %s: %w", id, err)
	}

	if err := task.Pause(ctx); err != nil {
		return fmt.Errorf("failed to pause task for container %s: %w", id, err)
	}

	return nil
}

func (r *Runtime) Resume(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: %w", id, err)
	}

	task, err := container.Task(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to get task for container %s: %w", id, err)
	}

	if err := task.Resume(ctx); err != nil {
		return fmt.Errorf("failed to resume task for container %s: %w", id, err)
	}

	// Record access time for Janitor
	r.accessTracker.Store(id, time.Now())

	return nil
}

func (r *Runtime) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

// List returns the state of all managed containers.
// Used by Janitor to identify idle or orphan containers.
func (r *Runtime) List(ctx context.Context) ([]runtime.ContainerState, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	// Get all containers managed by ESB
	containers, err := r.client.Containers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var states []runtime.ContainerState
	for _, c := range containers {
		containerID := c.ID()

		// Skip containers not managed by our runtime (check for lambda- prefix)
		if !strings.HasPrefix(containerID, runtime.ContainerNamePrefix) {
			continue
		}

		// Get function name from labels
		labels, err := c.Labels(ctx)
		functionName := ""
		if err == nil {
			functionName = labels[runtime.LabelFunctionName]
		}

		// Get task status
		status := "UNKNOWN"
		task, err := c.Task(ctx, nil)
		if err == nil {
			s, err := task.Status(ctx)
			if err == nil {
				switch s.Status {
				case containerd.Running:
					status = "RUNNING"
				case containerd.Paused:
					status = "PAUSED"
				case containerd.Stopped:
					status = "STOPPED"
				default:
					status = "UNKNOWN"
				}
			}
		} else {
			status = "STOPPED" // No task means container is stopped
		}

		// Get last access time from tracker
		lastUsedAt := time.Time{}
		if val, ok := r.accessTracker.Load(containerID); ok {
			lastUsedAt = val.(time.Time)
		}

		states = append(states, runtime.ContainerState{
			ID:           containerID,
			FunctionName: functionName,
			Status:       status,
			LastUsedAt:   lastUsedAt,
		})
	}

	return states, nil
}
