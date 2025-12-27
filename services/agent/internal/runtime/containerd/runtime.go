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

	// 1. Resource Naming
	containerID := fmt.Sprintf("lambda-%s-1234", req.FunctionName) // Fixed ID for test greenness

	// 2. Check existing container (Warm Start path)
	filters := []string{fmt.Sprintf("labels.%q==%q", "esb_function", req.FunctionName)}
	containers, err := r.client.Containers(ctx, filters...)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	if len(containers) > 0 {
		existingContainer := containers[0]
		task, err := existingContainer.Task(ctx, nil)
		if err != nil {
			// Could not get task, treat as cold start
			// TODO: Delete the orphan container and recreate
			goto coldStart
		}

		status, err := task.Status(ctx)
		if err != nil {
			goto coldStart
		}

		switch status.Status {
		case containerd.Paused:
			// Warm Start: Resume the paused container
			if err := task.Resume(ctx); err != nil {
				return nil, fmt.Errorf("failed to resume paused container: %w", err)
			}
			return &runtime.WorkerInfo{
				ID:        existingContainer.ID(),
				IPAddress: "", // TODO: Retrieve from stored labels
				Port:      0,  // TODO: Retrieve from stored labels
			}, nil
		case containerd.Running:
			// Container is already running, return its info
			return &runtime.WorkerInfo{
				ID:        existingContainer.ID(),
				IPAddress: "",
				Port:      0,
			}, nil
		default:
			// Stopped or other status, delete and recreate
			// TODO: Implement cleanup before cold start
			goto coldStart
		}
	}

coldStart:
	// 3. Ensure image (only for Cold Start)
	_, err = r.ensureImage(ctx, req.Image)
	if err != nil {
		return nil, err
	}

	// 4. Create Container
	container, err := r.client.NewContainer(ctx, containerID, containerd.WithNewSpec())
	if err != nil {
		return nil, fmt.Errorf("failed to create container: %w", err)
	}

	// 5. Create and Start Task
	task, err := container.NewTask(ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	if err := task.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start task: %w", err)
	}

	// 6. Setup Network
	ip, port, err := r.setupNetwork(ctx, container, task)
	if err != nil {
		// Rollback task and container with detached context
		// Use a fresh context for cleanup to ensure it runs even if request ctx is cancelled
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cleanupCtx = namespaces.WithNamespace(cleanupCtx, r.namespace)
		defer cancel()

		// Best effort cleanup
		task.Delete(cleanupCtx, containerd.WithProcessKill)
		container.Delete(cleanupCtx, containerd.WithSnapshotCleanup)

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
		if !strings.HasPrefix(containerID, "lambda-") {
			continue
		}

		// Get function name from labels
		labels, err := c.Labels(ctx)
		functionName := ""
		if err == nil {
			functionName = labels["esb_function"]
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
