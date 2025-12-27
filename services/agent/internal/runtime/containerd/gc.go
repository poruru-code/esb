package containerd

import (
	"context"
	"fmt"
	"log"
	"syscall"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
)

// GC removes all containers and tasks in the runtime's namespace.
func (r *Runtime) GC(ctx context.Context) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	containers, err := r.client.Containers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers for GC: %w", err)
	}

	for _, c := range containers {
		task, err := c.Task(ctx, nil)
		if err == nil {
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
			log.Printf("Warning: failed to delete container %s during GC: %v", c.ID(), err)
		}
	}

	return nil
}
