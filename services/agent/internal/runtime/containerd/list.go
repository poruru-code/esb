// Where: services/agent/internal/runtime/containerd/list.go
// What: Container listing and CNI IP lookup helpers.
// Why: Decouple state inspection paths from runtime lifecycle methods.
package containerd

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/namespaces"
	"github.com/poruru/edge-serverless-box/services/agent/internal/runtime"
)

// List returns the state of all managed containers.
// Used by Janitor to identify idle or orphan containers.
func (r *Runtime) List(ctx context.Context) ([]runtime.ContainerState, error) {
	ctx = namespaces.WithNamespace(ctx, r.namespace)

	containers, err := r.client.Containers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var states []runtime.ContainerState
	for _, c := range containers {
		containerID := c.ID()

		managed, err := r.isManagedContainer(ctx, c)
		if err != nil {
			log.Printf("Warning: failed to inspect container %s during list: %v", containerID, err)
			continue
		}
		if !managed {
			continue
		}

		info, infoErr := c.Info(ctx)
		createdAt := time.Time{}
		functionName := ""
		ownerID := ""
		if infoErr == nil {
			createdAt = info.CreatedAt
			functionName = info.Labels[runtime.LabelFunctionName]
			ownerID = info.Labels[runtime.LabelOwner]
		} else {
			labels, err := c.Labels(ctx)
			if err == nil {
				functionName = labels[runtime.LabelFunctionName]
				ownerID = labels[runtime.LabelOwner]
			}
		}
		if functionName == "" {
			log.Printf("Warning: missing function label for container %s", containerID)
			continue
		}

		if createdAt.IsZero() {
			createdAt = time.Now()
		}

		status := runtime.StatusUnknown
		task, err := c.Task(ctx, nil)
		if err == nil {
			s, err := task.Status(ctx)
			if err == nil {
				switch s.Status {
				case containerd.Running:
					status = runtime.StatusRunning
				case containerd.Paused:
					status = runtime.StatusPaused
				case containerd.Stopped:
					status = runtime.StatusStopped
				default:
					status = runtime.StatusUnknown
				}
			}
		} else {
			status = runtime.StatusStopped
		}

		lastUsedAt := createdAt
		if val, ok := r.accessTracker.Load(containerID); ok {
			lastUsedAt = val.(time.Time)
		}

		ipAddress := ""
		if ip, err := r.resolveContainerIP(containerID); err == nil {
			ipAddress = ip
		}

		states = append(states, runtime.ContainerState{
			ID:            containerID,
			FunctionName:  functionName,
			Status:        status,
			LastUsedAt:    lastUsedAt,
			ContainerName: containerID,
			CreatedAt:     createdAt,
			IPAddress:     ipAddress,
			Port:          8080,
			OwnerID:       ownerID,
		})
	}

	return states, nil
}

func (r *Runtime) resolveContainerIP(containerID string) (string, error) {
	netDir := resolveCNINetDir()
	networkName := r.resolveCNINetworkName()
	path := filepath.Join(netDir, networkName, containerID)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	ip, err := parseCNIIPAddress(string(data))
	if err != nil {
		return "", err
	}
	return ip, nil
}

func parseCNIIPAddress(value string) (string, error) {
	scanner := bufio.NewScanner(strings.NewReader(value))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		ip := net.ParseIP(fields[0])
		if ip == nil || ip.To4() == nil {
			return "", fmt.Errorf("invalid IP address %q", fields[0])
		}
		return ip.String(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no IP address found")
}
