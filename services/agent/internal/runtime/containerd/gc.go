package containerd

import (
	"context"
	"fmt"
	"log"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/poruru-code/esb/services/agent/internal/runtime"
)

// GC removes all containers and tasks in the runtime's namespace.
func (r *Runtime) GC(ctx context.Context) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	containers, err := r.client.Containers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers for GC: %w", err)
	}

	for _, c := range containers {
		containerID := c.ID()
		managed, err := r.isManagedContainer(ctx, c)
		if err != nil {
			log.Printf("Warning: failed to inspect container %s during GC: %v", containerID, err)
			continue
		}
		if !managed {
			continue
		}

		task, err := c.Task(ctx, nil)
		if err == nil {
			if r.cni != nil {
				pid := task.Pid()
				if pid > 0 {
					netnsPath := fmt.Sprintf("/proc/%d/ns/net", pid)
					if err := r.removeCNI(ctx, containerID, netnsPath); err != nil {
						log.Printf("Warning: failed to remove CNI network for %s: %v", containerID, err)
					}
				}
			}

			// Kill and delete task
			status, err := task.Status(ctx)
			if err == nil && (status.Status == containerd.Running || status.Status == containerd.Paused) {
				_ = task.Kill(ctx, syscall.SIGKILL)
				exitStatus, _ := task.Wait(ctx)
				if exitStatus != nil {
					<-exitStatus
				}
			}
			_, _ = task.Delete(ctx)
		}

		if err := c.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
			log.Printf("Warning: failed to delete container %s during GC: %v", containerID, err)
		}
	}

	return nil
}

func (r *Runtime) isManagedContainer(ctx context.Context, c containerd.Container) (bool, error) {
	labels, err := r.getContainerLabels(ctx, c)
	if err != nil {
		return false, err
	}
	if labels[runtime.LabelCreatedBy] != runtime.ValueCreatedByAgent {
		return false, nil
	}
	if labels[runtime.LabelEsbEnv] != r.env {
		return false, nil
	}
	if labels[runtime.LabelFunctionKind] != runtime.ValueFunctionKind {
		return false, nil
	}
	return true, nil
}

func (r *Runtime) getContainerLabels(
	ctx context.Context,
	c containerd.Container,
) (map[string]string, error) {
	info, err := c.Info(ctx)
	if err == nil && len(info.Labels) > 0 {
		return info.Labels, nil
	}
	labels, err := c.Labels(ctx)
	if err != nil {
		return nil, err
	}
	return labels, nil
}
