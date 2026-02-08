// Where: services/agent/internal/runtime/containerd/runtime.go
// What: Containerd runtime implementation for the Go agent.
// Why: Manage container lifecycle and CNI wiring for lambda workers.
package containerd

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/go-cni"
	"github.com/poruru/edge-serverless-box/meta"
	"github.com/poruru/edge-serverless-box/services/agent/internal/config"
)

const (
	runtimeFirecracker  = "aws.firecracker"
	resolvConfMountPath = "/run/containerd/esb/resolv.conf"
)

type Runtime struct {
	client        Client
	cni           cni.CNI
	cniMu         sync.Mutex // serialize CNI operations to avoid bridge races
	namespace     string
	env           string
	accessTracker sync.Map // map[containerID]time.Time - tracks last access time
}

// NewRuntime creates a new containerd runtime with CNI networking.
func NewRuntime(client Client, cniPlugin cni.CNI, namespace, env string) *Runtime {
	return &Runtime{
		client:    client,
		cni:       cniPlugin,
		namespace: namespace,
		env:       env,
	}
}

func resolveSnapshotter() string {
	if value := strings.TrimSpace(os.Getenv("CONTAINERD_SNAPSHOTTER")); value != "" {
		return value
	}
	runtimeName := strings.TrimSpace(os.Getenv("CONTAINERD_RUNTIME"))
	if runtimeName == runtimeFirecracker {
		return config.DefaultSnapshotterDevmapper
	}
	return config.DefaultSnapshotterOverlay
}

func resolveCNIDNSServer() string {
	if value := strings.TrimSpace(os.Getenv("CNI_DNS_SERVER")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("CNI_GW_IP")); value != "" {
		return value
	}
	return config.DefaultCNIDNSServer
}

func resolveCNINetDir() string {
	if value := strings.TrimSpace(os.Getenv("CNI_NET_DIR")); value != "" {
		return value
	}
	return "/var/lib/cni/networks" // Standard CNI path, no constant for this as it's not esb-specific
}

func (r *Runtime) resolveCNINetworkName() string {
	if r.cni != nil {
		cfg := r.cni.GetConfig()
		if cfg != nil {
			for _, network := range cfg.Networks {
				if network != nil && network.Config != nil && network.Config.Name != "" {
					return network.Config.Name
				}
			}
		}
	}
	return meta.RuntimeCNIName
}

func (r *Runtime) Destroy(ctx context.Context, id string) error {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	container, err := r.client.LoadContainer(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to load container %s: %w", id, err)
	}

	task, err := container.Task(ctx, nil)
	if err == nil {
		if r.cni != nil {
			netnsPath := fmt.Sprintf("/proc/%d/ns/net", task.Pid())
			if err := r.removeCNI(ctx, id, netnsPath); err != nil {
				log.Printf("WARNING: failed to remove CNI network for %s: %v", id, err)
			}
		}
		_, _ = task.Delete(ctx, containerd.WithProcessKill)
	}

	if err := container.Delete(ctx, containerd.WithSnapshotCleanup); err != nil {
		return fmt.Errorf("failed to delete container %s: %w", id, err)
	}

	r.accessTracker.Delete(id)
	return nil
}

func (r *Runtime) Suspend(ctx context.Context, id string) error {
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

	r.accessTracker.Store(id, time.Now())
	return nil
}

func (r *Runtime) Touch(id string) {
	r.accessTracker.Store(id, time.Now())
}

func (r *Runtime) Close() error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}
