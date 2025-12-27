package containerd

import (
	"context"
	"fmt"

	"github.com/containerd/containerd"
	"github.com/containerd/go-cni"
)

// setupNetwork sets up CNI network for the container.
// It allocates a host port, configures CNI, and returns the assigned IP and host port.
func (r *Runtime) setupNetwork(ctx context.Context, container containerd.Container, task containerd.Task) (string, int, error) {
	id := container.ID()
	pid := task.Pid()
	netNSPath := fmt.Sprintf("/proc/%d/ns/net", pid)

	// 1. Allocate Host Port
	hostPort, err := r.portAllocator.Allocate()
	if err != nil {
		return "", 0, fmt.Errorf("failed to allocate host port: %w", err)
	}

	// 2. Setup CNI
	portMap := []cni.PortMapping{{
		HostPort:      int32(hostPort),
		ContainerPort: 8080,
		Protocol:      "tcp",
	}}

	result, err := r.cni.Setup(ctx, id, netNSPath, cni.WithCapabilityPortMap(portMap))
	if err != nil {
		r.portAllocator.Release(hostPort)
		return "", 0, fmt.Errorf("failed to setup CNI network: %w", err)
	}

	// 3. Extract IP
	var ip string
	// Check standard CNI result structure
	// go-cni Result.Interfaces["eth0"].IPConfigs[0].IP
	if len(result.Interfaces) > 0 {
		for _, iface := range result.Interfaces {
			if len(iface.IPConfigs) > 0 {
				ip = iface.IPConfigs[0].IP.String()
				break
			}
		}
	}

	if ip == "" {
		// Rollback on failure to find IP
		_ = r.cni.Remove(ctx, id, netNSPath, cni.WithCapabilityPortMap(portMap))
		r.portAllocator.Release(hostPort)
		return "", 0, fmt.Errorf("failed to get IP address from CNI result")
	}

	return ip, hostPort, nil
}

func (r *Runtime) teardownNetwork(ctx context.Context, container containerd.Container) error {
	return nil
}
