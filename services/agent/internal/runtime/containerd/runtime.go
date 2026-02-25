// Where: services/agent/internal/runtime/containerd/runtime.go
// What: Containerd runtime implementation for the Go agent.
// Why: Manage container lifecycle and CNI wiring for lambda workers.
package containerd

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/go-cni"
	"github.com/poruru-code/esb/services/agent/internal/config"
	"github.com/poruru-code/esb/services/agent/internal/identity"
)

const (
	runtimeFirecracker = "aws.firecracker"
)

type Runtime struct {
	client        Client
	cni           cni.CNI
	cniMu         sync.Mutex // serialize CNI operations to avoid bridge races
	namespace     string
	env           string
	brandSlug     string
	cniNetwork    string
	resolvConf    string
	accessTracker sync.Map // map[containerID]time.Time - tracks last access time
}

// NewRuntime creates a new containerd runtime with CNI networking.
func NewRuntime(client Client, cniPlugin cni.CNI, namespace, env, brandSlug string) *Runtime {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		panic("containerd runtime namespace is required")
	}
	brand := identity.SanitizeBrandSlug(brandSlug)
	if brand == "" {
		panic("containerd runtime brand slug is required")
	}
	return &Runtime{
		client:     client,
		cni:        cniPlugin,
		namespace:  ns,
		env:        env,
		brandSlug:  brand,
		cniNetwork: brand + "-net",
		resolvConf: fmt.Sprintf("/run/containerd/%s/resolv.conf", ns),
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
	if value := strings.TrimSpace(os.Getenv("CNI_SUBNET")); value != "" {
		if gw, err := subnetGateway(value); err == nil {
			return gw
		}
	}
	return config.DefaultCNIDNSServer
}

func subnetGateway(subnet string) (string, error) {
	_, cidr, err := net.ParseCIDR(strings.TrimSpace(subnet))
	if err != nil {
		return "", err
	}
	ones, bits := cidr.Mask.Size()
	if bits != 32 || ones >= 31 {
		return "", fmt.Errorf("usable IPv4 CIDR required")
	}
	network := cidr.IP.Mask(cidr.Mask).To4()
	if network == nil {
		return "", fmt.Errorf("IPv4 CIDR required")
	}
	value := binary.BigEndian.Uint32(network) + 1
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, value)
	return ip.String(), nil
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
	return r.cniNetwork
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
